package role

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropic-cav/cav-npc/internal/client"
	"github.com/anthropic-cav/cav-npc/internal/signal"
)

// CustomRoleConfig defines a custom role from TOML configuration.
type CustomRoleConfig struct {
	SignalFilter       client.Filter `json:"signal_filter"`
	SystemPrompt      string        `json:"system_prompt_template"`
	OutputSignalTypes []string      `json:"output_signal_types"`
	BatchModeEnabled  bool          `json:"batch_mode"`
	BatchInterval     time.Duration `json:"batch_interval"`
}

// LoadCustom creates a Role from a custom configuration map.
// Expected keys: signal_filter, system_prompt_template, output_signal_types, batch_mode, batch_interval
func LoadCustom(name string, params map[string]any) (Role, error) {
	cfg := CustomRoleConfig{}

	// Parse signal_filter
	if filterRaw, ok := params["signal_filter"].(map[string]any); ok {
		if types, ok := filterRaw["types"].([]any); ok {
			for _, t := range types {
				if s, ok := t.(string); ok {
					cfg.SignalFilter.Types = append(cfg.SignalFilter.Types, s)
				}
			}
		}
		if tags, ok := filterRaw["tags"].([]any); ok {
			for _, t := range tags {
				if s, ok := t.(string); ok {
					cfg.SignalFilter.Tags = append(cfg.SignalFilter.Tags, s)
				}
			}
		}
	}

	// Parse system_prompt_template
	if tmpl, ok := params["system_prompt_template"].(string); ok {
		cfg.SystemPrompt = tmpl
	} else {
		return nil, fmt.Errorf("custom role %q: system_prompt_template is required", name)
	}

	// Parse output_signal_types
	if types, ok := params["output_signal_types"].([]any); ok {
		for _, t := range types {
			if s, ok := t.(string); ok {
				cfg.OutputSignalTypes = append(cfg.OutputSignalTypes, s)
			}
		}
	}

	// Parse batch_mode
	if bm, ok := params["batch_mode"].(bool); ok {
		cfg.BatchModeEnabled = bm
	}

	// Parse batch_interval (as string duration)
	if bi, ok := params["batch_interval"].(string); ok {
		d, err := time.ParseDuration(bi)
		if err == nil {
			cfg.BatchInterval = d
		}
	}
	if cfg.BatchModeEnabled && cfg.BatchInterval == 0 {
		cfg.BatchInterval = 1 * time.Hour // default
	}

	return &customRole{name: name, cfg: cfg}, nil
}

// customRole implements Role using configuration-driven behavior.
type customRole struct {
	name string
	cfg  CustomRoleConfig
}

func (c *customRole) Name() string { return c.name }

func (c *customRole) Filter() client.Filter { return c.cfg.SignalFilter }

func (c *customRole) BatchMode() (bool, time.Duration) {
	return c.cfg.BatchModeEnabled, c.cfg.BatchInterval
}

func (c *customRole) Capable(sig *signal.EntropicSignal) bool {
	// Custom roles match based on their filter tags
	if len(c.cfg.SignalFilter.Tags) == 0 {
		return true
	}
	for _, tag := range sig.Tags {
		for _, ft := range c.cfg.SignalFilter.Tags {
			if tag == ft {
				return true
			}
		}
	}
	return false
}

func (c *customRole) BuildPrompt(ctx PromptContext) (system, user string) {
	sys, err := RenderPrompt(c.cfg.SystemPrompt, ctx)
	if err != nil {
		sys = c.cfg.SystemPrompt
	}
	system = SecurityPrefix + sys

	if ctx.Signal != nil {
		b, _ := json.MarshalIndent(ctx.Signal, "", "  ")
		user = string(b)
	} else if len(ctx.Batch) > 0 {
		b, _ := json.MarshalIndent(ctx.Batch, "", "  ")
		user = string(b)
	}
	return
}

func (c *customRole) ParseOutput(raw string, _ PromptContext) ([]*signal.OutSignal, error) {
	out, err := signal.ParseLLMOutput(raw)
	if err != nil {
		return nil, err
	}
	// Force output type to first configured type if specified
	if len(c.cfg.OutputSignalTypes) > 0 {
		out.Type = signal.SignalType(c.cfg.OutputSignalTypes[0])
	}
	return []*signal.OutSignal{out}, nil
}
