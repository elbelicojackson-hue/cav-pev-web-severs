package builtin

import (
	"encoding/json"
	"time"

	"github.com/anthropic-cav/cav-npc/internal/client"
	"github.com/anthropic-cav/cav-npc/internal/role"
	"github.com/anthropic-cav/cav-npc/internal/signal"
)

func init() {
	role.Register("format_validator", func(_ map[string]any) (role.Role, error) {
		return &formatValidator{}, nil
	})
}

const validatorSystemPrompt = `你是 CAV 网络的格式验证器 (Format Validator)。你的职责是检查网络中信号的结构合规性。

当你发现结构性问题时,产出一个 type="challenge" 的 EntropicSignal,引用违规信号并说明具体问题。

检查项:
- posterior_shift 是否包含所有必需字段 (subject, relation, object, prior_confidence, posterior_confidence, delta_bits, direction)
- confidence 值是否在 [0,1] 范围内
- delta_bits 是否非负
- grounding 是否包含 type, source, evidence
- falsifiability 是否非空
- uncertainty.known_failure_modes 是否至少有一项

如果信号完全合规,不要产出任何输出(返回空 JSON 对象 {})。

信号来源: {{.signal_from}}
信号类型: {{.signal_type}}

请以单个 JSON 对象回复。如果发现问题,返回完整的 challenge EntropicSignal;如果合规,返回 {}。`

type formatValidator struct{}

func (f *formatValidator) Name() string { return "format_validator" }

func (f *formatValidator) Filter() client.Filter {
	return client.Filter{} // subscribe to all
}

func (f *formatValidator) BatchMode() (bool, time.Duration) {
	return false, 0
}

func (f *formatValidator) Capable(sig *signal.EntropicSignal) bool {
	// Validator handles format-checking tasks
	for _, tag := range sig.Tags {
		if tag == "format_check" || tag == "validation" {
			return true
		}
	}
	return false
}

func (f *formatValidator) BuildPrompt(ctx role.PromptContext) (system, user string) {
	sys, err := role.RenderPrompt(validatorSystemPrompt, ctx)
	if err != nil {
		sys = validatorSystemPrompt
	}
	system = role.SecurityPrefix + sys

	user = "验证以下信号的结构合规性:\n\n" + formatSignalForPrompt(ctx.Signal)

	// Also run local validation and include results
	if ctx.Signal != nil {
		localErr := signal.Validate(ctx.Signal)
		if localErr != nil {
			user += "\n\n本地校验结果: 不合规 — " + localErr.Error()
		} else {
			user += "\n\n本地校验结果: 合规"
		}
	}

	return
}

func (f *formatValidator) ParseOutput(raw string, ctx role.PromptContext) ([]*signal.OutSignal, error) {
	// If the LLM returns empty object {}, no challenge needed
	raw2 := stripCodeFencesSimple(raw)
	var check map[string]any
	if json.Unmarshal([]byte(raw2), &check) == nil {
		if len(check) == 0 {
			return nil, nil // signal is valid, no output
		}
	}

	// Otherwise parse as a challenge signal
	out, err := signal.ParseLLMOutput(raw)
	if err != nil {
		return nil, err
	}
	// Force type to challenge (R13.5)
	out.Type = signal.SignalChallenge
	return []*signal.OutSignal{out}, nil
}

func stripCodeFencesSimple(s string) string {
	// Minimal fence stripping for the empty-object check
	if len(s) > 6 && s[:3] == "```" {
		for i := 3; i < len(s); i++ {
			if s[i] == '\n' {
				s = s[i+1:]
				break
			}
		}
		if len(s) > 3 && s[len(s)-3:] == "```" {
			s = s[:len(s)-3]
		}
	}
	return s
}
