package builtin

import (
	"time"

	"github.com/anthropic-cav/cav-npc/internal/client"
	"github.com/anthropic-cav/cav-npc/internal/role"
	"github.com/anthropic-cav/cav-npc/internal/signal"
)

func init() {
	role.Register("cve_translator", func(_ map[string]any) (role.Role, error) {
		return &cveTranslator{}, nil
	})
}

const cveSystemPrompt = `你是 CAV 网络的 CVE 翻译器 (CVE Translator)。你的职责是将 NVD/KEV 漏洞数据转换为结构化的 EntropicSignal 格式。

输入信号包含原始漏洞信息(CVE ID、CVSS 分数、影响范围等)。你需要:
1. 提取关键信息并构建 posterior_shift (subject=受影响组件, relation=存在漏洞, object=CVE ID)
2. 设置合理的 confidence (基于 CVSS 分数和验证状态)
3. 提供 grounding (引用 NVD/KEV 数据源)
4. 设置 falsifiability (什么情况下该漏洞不适用)
5. 列出 known_failure_modes (误报场景)

输出 type 固定为 "learning"。
Tags 必须包含: "cve", "vulnerability", "praxon-translation"

你的声誉分数: {{.npc_reputation}}

请以单个 JSON 对象回复,包含完整的 EntropicSignal 结构。`

type cveTranslator struct{}

func (c *cveTranslator) Name() string { return "cve_translator" }

func (c *cveTranslator) Filter() client.Filter {
	return client.Filter{
		Tags: []string{"cve", "vulnerability", "nvd", "kev"},
	}
}

func (c *cveTranslator) BatchMode() (bool, time.Duration) {
	return false, 0
}

func (c *cveTranslator) Capable(sig *signal.EntropicSignal) bool {
	for _, tag := range sig.Tags {
		switch tag {
		case "cve", "vulnerability", "nvd", "kev":
			return true
		}
	}
	return false
}

func (c *cveTranslator) BuildPrompt(ctx role.PromptContext) (system, user string) {
	sys, err := role.RenderPrompt(cveSystemPrompt, ctx)
	if err != nil {
		sys = cveSystemPrompt
	}
	system = role.SecurityPrefix + sys
	user = "将以下漏洞信号转换为结构化 EntropicSignal:\n\n" + formatSignalForPrompt(ctx.Signal)
	return
}

func (c *cveTranslator) ParseOutput(raw string, _ role.PromptContext) ([]*signal.OutSignal, error) {
	out, err := signal.ParseLLMOutput(raw)
	if err != nil {
		return nil, err
	}
	// Force correct type and required tags (R5.5)
	out.Type = signal.SignalLearning
	out.Tags = ensureTags(out.Tags, "cve", "vulnerability", "praxon-translation")
	return []*signal.OutSignal{out}, nil
}

// ensureTags adds required tags if not already present.
func ensureTags(tags []string, required ...string) []string {
	existing := make(map[string]bool, len(tags))
	for _, t := range tags {
		existing[t] = true
	}
	for _, r := range required {
		if !existing[r] {
			tags = append(tags, r)
		}
	}
	return tags
}
