// Package builtin contains the 6 built-in NPC roles.
package builtin

import (
	"time"

	"github.com/anthropic-cav/cav-npc/internal/client"
	"github.com/anthropic-cav/cav-npc/internal/role"
	"github.com/anthropic-cav/cav-npc/internal/signal"
)

func init() {
	role.Register("signal_sentinel", func(_ map[string]any) (role.Role, error) {
		return &signalSentinel{}, nil
	})
}

const sentinelSystemPrompt = `你是 CAV 网络的信号哨兵 (Signal Sentinel)。你的职责是监控所有网络信号,识别异常、不一致和潜在问题。

当你发现以下情况时,产出一个结构化的 EntropicSignal:
- 信号之间的逻辑矛盾
- 异常的置信度跳变 (delta_bits 异常高或低)
- 缺乏 grounding 的高置信度声明
- 重复或近似重复的信号 (可能的 Sybil 行为)

输出格式要求:
- 对于异常检测: type="challenge", 说明发现的问题
- 对于有价值的观察: type="learning", 总结你的发现

你的声誉分数: {{.npc_reputation}}
信号来源: {{.signal_from}}
信号类型: {{.signal_type}}

请以单个 JSON 对象回复,包含完整的 EntropicSignal 结构。`

type signalSentinel struct{}

func (s *signalSentinel) Name() string { return "signal_sentinel" }

func (s *signalSentinel) Filter() client.Filter {
	return client.Filter{} // empty = subscribe to all
}

func (s *signalSentinel) BatchMode() (bool, time.Duration) {
	return false, 0
}

func (s *signalSentinel) Capable(sig *signal.EntropicSignal) bool {
	// Sentinel can handle any signal analysis task
	return true
}

func (s *signalSentinel) BuildPrompt(ctx role.PromptContext) (system, user string) {
	sys, err := role.RenderPrompt(sentinelSystemPrompt, ctx)
	if err != nil {
		sys = sentinelSystemPrompt // fallback to raw template
	}
	system = role.SecurityPrefix + sys

	// User message is the signal content
	user = "分析以下信号:\n\n" + formatSignalForPrompt(ctx.Signal)
	return
}

func (s *signalSentinel) ParseOutput(raw string, _ role.PromptContext) ([]*signal.OutSignal, error) {
	out, err := signal.ParseLLMOutput(raw)
	if err != nil {
		return nil, err
	}
	return []*signal.OutSignal{out}, nil
}
