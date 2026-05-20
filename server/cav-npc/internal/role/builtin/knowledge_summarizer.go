package builtin

import (
	"encoding/json"
	"time"

	"github.com/anthropic-cav/cav-npc/internal/client"
	"github.com/anthropic-cav/cav-npc/internal/role"
	"github.com/anthropic-cav/cav-npc/internal/signal"
)

func init() {
	role.Register("knowledge_summarizer", func(_ map[string]any) (role.Role, error) {
		return &knowledgeSummarizer{}, nil
	})
}

const summarizerSystemPrompt = `你是 CAV 网络的知识摘要器 (Knowledge Summarizer)。你的职责是定期产出知识摘要,将一段时间内的信号综合为高层次的认知信号。

你收到的是一批信号(batch mode)。请:
1. 识别主要主题和趋势
2. 综合多个信号的发现
3. 产出 1-3 个摘要信号,每个覆盖一个主题

输出类型固定为 "learning"。
每个输出信号必须:
- 有明确的 subject (主题)
- 引用多个源信号作为 grounding
- 量化信息增益 (delta_bits)
- 列出综合过程中的不确定性

你的声誉分数: {{.npc_reputation}}

请以 JSON 数组回复,每个元素是一个完整的 EntropicSignal 结构。如果只有一个摘要,也用数组包裹。`

type knowledgeSummarizer struct{}

func (k *knowledgeSummarizer) Name() string { return "knowledge_summarizer" }

func (k *knowledgeSummarizer) Filter() client.Filter {
	return client.Filter{} // subscribe to all (batch mode)
}

func (k *knowledgeSummarizer) BatchMode() (bool, time.Duration) {
	return true, 1 * time.Hour // R6.3: batch every hour
}

func (k *knowledgeSummarizer) Capable(sig *signal.EntropicSignal) bool {
	// Summarizer doesn't bid on individual tasks
	return false
}

func (k *knowledgeSummarizer) BuildPrompt(ctx role.PromptContext) (system, user string) {
	sys, err := role.RenderPrompt(summarizerSystemPrompt, ctx)
	if err != nil {
		sys = summarizerSystemPrompt
	}
	system = role.SecurityPrefix + sys
	user = "综合以下信号批次,产出知识摘要:\n\n" + formatBatchForPrompt(ctx.Batch)
	return
}

func (k *knowledgeSummarizer) ParseOutput(raw string, _ role.PromptContext) ([]*signal.OutSignal, error) {
	// Try parsing as array first
	raw = stripCodeFencesSimple(raw)

	// Attempt array parse
	var arr []signal.OutSignal
	if err := parseJSONArray(raw, &arr); err == nil && len(arr) > 0 {
		result := make([]*signal.OutSignal, 0, len(arr))
		for i := range arr {
			arr[i].Type = signal.SignalLearning
			arr[i].Tags = ensureTags(arr[i].Tags, "summary", "knowledge")
			result = append(result, &arr[i])
		}
		return result, nil
	}

	// Fallback: single signal
	out, err := signal.ParseLLMOutput(raw)
	if err != nil {
		return nil, err
	}
	out.Type = signal.SignalLearning
	out.Tags = ensureTags(out.Tags, "summary", "knowledge")
	return []*signal.OutSignal{out}, nil
}

func parseJSONArray(raw string, dst *[]signal.OutSignal) error {
	// Find array boundaries
	start := -1
	for i, c := range raw {
		if c == '[' {
			start = i
			break
		}
	}
	if start < 0 {
		return signal.ErrNoJSON
	}
	end := -1
	for i := len(raw) - 1; i >= start; i-- {
		if raw[i] == ']' {
			end = i
			break
		}
	}
	if end < 0 {
		return signal.ErrNoJSON
	}

	candidate := raw[start : end+1]
	return json.Unmarshal([]byte(candidate), dst)
}
