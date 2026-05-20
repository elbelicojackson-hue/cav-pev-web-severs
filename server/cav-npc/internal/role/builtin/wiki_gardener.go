package builtin

import (
	"time"

	"github.com/anthropic-cav/cav-npc/internal/client"
	"github.com/anthropic-cav/cav-npc/internal/role"
	"github.com/anthropic-cav/cav-npc/internal/signal"
)

func init() {
	role.Register("wiki_gardener", func(_ map[string]any) (role.Role, error) {
		return &wikiGardener{}, nil
	})
}

const wikiSystemPrompt = `你是 CAV 网络的 Wiki 园丁 (Wiki Gardener)。你的职责是维护知识库的一致性,发现矛盾并提出修正。

当你发现以下情况时产出信号:
- 新信号与已有知识矛盾 → type="retraction" (建议撤回旧知识)
- 新信号可以精炼已有知识 → type="refinement" (提出改进)
- 知识条目之间存在不一致 → type="challenge" (标记矛盾)

你的声誉分数: {{.npc_reputation}}
信号来源: {{.signal_from}}

请以单个 JSON 对象回复,包含完整的 EntropicSignal 结构。`

type wikiGardener struct{}

func (w *wikiGardener) Name() string { return "wiki_gardener" }

func (w *wikiGardener) Filter() client.Filter {
	return client.Filter{
		Tags: []string{"wiki", "knowledge"},
	}
}

func (w *wikiGardener) BatchMode() (bool, time.Duration) {
	return false, 0
}

func (w *wikiGardener) Capable(sig *signal.EntropicSignal) bool {
	for _, tag := range sig.Tags {
		if tag == "wiki" || tag == "knowledge" {
			return true
		}
	}
	return false
}

func (w *wikiGardener) BuildPrompt(ctx role.PromptContext) (system, user string) {
	sys, err := role.RenderPrompt(wikiSystemPrompt, ctx)
	if err != nil {
		sys = wikiSystemPrompt
	}
	system = role.SecurityPrefix + sys
	user = "分析以下知识信号,检查一致性:\n\n" + formatSignalForPrompt(ctx.Signal)
	return
}

func (w *wikiGardener) ParseOutput(raw string, _ role.PromptContext) ([]*signal.OutSignal, error) {
	out, err := signal.ParseLLMOutput(raw)
	if err != nil {
		return nil, err
	}
	// Ensure tags include wiki domain
	out.Tags = ensureTags(out.Tags, "wiki", "knowledge")
	return []*signal.OutSignal{out}, nil
}
