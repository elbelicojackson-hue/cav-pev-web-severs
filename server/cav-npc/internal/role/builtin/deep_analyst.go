package builtin

import (
	"time"

	"github.com/anthropic-cav/cav-npc/internal/client"
	"github.com/anthropic-cav/cav-npc/internal/role"
	"github.com/anthropic-cav/cav-npc/internal/signal"
)

func init() {
	role.Register("deep_analyst", func(_ map[string]any) (role.Role, error) {
		return &deepAnalyst{}, nil
	})
}

const analystSystemPrompt = `你是 CAV 网络的深度分析师 (Deep Analyst)。你的职责是处理复杂的多步推理任务,产出高质量的认知信号。

你擅长:
- 因果推理 (识别因果关系 vs 相关性)
- 多源信息综合
- 反事实分析
- 不确定性量化

输出类型: "learning" (新发现) 或 "refinement" (改进已有结论)

要求:
- posterior_shift 必须有明确的因果方向
- grounding 必须引用具体证据
- uncertainty.known_failure_modes 至少列出 2 项
- falsifiability 必须具体可验证

你的声誉分数: {{.npc_reputation}}
信号来源: {{.signal_from}}
信号类型: {{.signal_type}}

请以单个 JSON 对象回复,包含完整的 EntropicSignal 结构。`

type deepAnalyst struct{}

func (d *deepAnalyst) Name() string { return "deep_analyst" }

func (d *deepAnalyst) Filter() client.Filter {
	return client.Filter{
		Tags: []string{"complex", "reasoning", "analysis"},
	}
}

func (d *deepAnalyst) BatchMode() (bool, time.Duration) {
	return false, 0
}

func (d *deepAnalyst) Capable(sig *signal.EntropicSignal) bool {
	for _, tag := range sig.Tags {
		switch tag {
		case "complex", "reasoning", "analysis", "needs_analysis":
			return true
		}
	}
	return false
}

func (d *deepAnalyst) BuildPrompt(ctx role.PromptContext) (system, user string) {
	sys, err := role.RenderPrompt(analystSystemPrompt, ctx)
	if err != nil {
		sys = analystSystemPrompt
	}
	system = role.SecurityPrefix + sys
	user = "对以下信号进行深度分析:\n\n" + formatSignalForPrompt(ctx.Signal)
	return
}

func (d *deepAnalyst) ParseOutput(raw string, _ role.PromptContext) ([]*signal.OutSignal, error) {
	out, err := signal.ParseLLMOutput(raw)
	if err != nil {
		return nil, err
	}
	return []*signal.OutSignal{out}, nil
}
