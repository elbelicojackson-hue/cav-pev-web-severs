// Package config handles TOML configuration loading, environment variable
// resolution, and startup validation for the NPC runtime.
package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config is the top-level configuration for the NPC runtime.
type Config struct {
	Runtime RuntimeConfig `toml:"runtime"`
	NPCs    []NPCConfig   `toml:"npc"`
}

// RuntimeConfig holds process-level settings.
type RuntimeConfig struct {
	GatewayURL string       `toml:"gateway_url"`
	KeysDir    string       `toml:"keys_dir"`
	HealthPort int          `toml:"health_port"`
	LogLevel   string       `toml:"log_level"`
	Budget     BudgetConfig `toml:"budget"`
}

// BudgetConfig defines token budget limits.
type BudgetConfig struct {
	MaxTokensPerHour int64 `toml:"max_tokens_per_hour"`
	MaxTokensPerDay  int64 `toml:"max_tokens_per_day"`
}

// NPCConfig defines a single NPC instance.
type NPCConfig struct {
	Name      string         `toml:"name"`
	Role      string         `toml:"role"`
	GuildTag  string         `toml:"guild_tag"`
	LLM       LLMConfig      `toml:"llm"`
	RateLimit RateLimitConf  `toml:"rate_limit"`
	Custom    map[string]any `toml:"custom"`
}

// LLMConfig specifies the LLM provider for an NPC.
type LLMConfig struct {
	Provider    string  `toml:"provider"`
	Endpoint    string  `toml:"endpoint"`
	APIKeyEnv   string  `toml:"api_key_env"`
	Model       string  `toml:"model"`
	MaxTokens   int     `toml:"max_tokens"`
	Temperature float64 `toml:"temperature"`
}

// RateLimitConf defines per-NPC rate limits.
type RateLimitConf struct {
	LLMPerMinute int `toml:"llm_per_minute"`
	PublishPer5s int `toml:"publish_per_5s"`
}

// Load reads and validates a TOML configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse TOML: %w", err)
	}

	applyDefaults(&cfg)

	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// applyDefaults fills in zero-value fields with sensible defaults.
func applyDefaults(cfg *Config) {
	if cfg.Runtime.LogLevel == "" {
		cfg.Runtime.LogLevel = "info"
	}
	if cfg.Runtime.KeysDir == "" {
		cfg.Runtime.KeysDir = "./npc-keys"
	}
	if cfg.Runtime.HealthPort == 0 {
		cfg.Runtime.HealthPort = 9090
	}

	for i := range cfg.NPCs {
		if cfg.NPCs[i].RateLimit.LLMPerMinute == 0 {
			cfg.NPCs[i].RateLimit.LLMPerMinute = 10
		}
		if cfg.NPCs[i].RateLimit.PublishPer5s == 0 {
			cfg.NPCs[i].RateLimit.PublishPer5s = 1
		}
	}
}

// validNameRe matches allowed NPC names: lowercase alphanumeric + hyphens + underscores, 1-64 chars.
var validNameRe = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// allowedProviders is the set of supported LLM providers.
var allowedProviders = map[string]bool{
	"deepseek":   true,
	"volcengine": true,
	"dashscope":  true,
}

// allowedRoles is the set of built-in roles (custom roles validated separately).
var allowedRoles = map[string]bool{
	"signal_sentinel":      true,
	"cve_translator":       true,
	"wiki_gardener":        true,
	"deep_analyst":         true,
	"format_validator":     true,
	"knowledge_summarizer": true,
}

// Validate checks the configuration for correctness.
func Validate(cfg *Config) error {
	// Runtime
	if cfg.Runtime.GatewayURL == "" {
		return fmt.Errorf("validation: runtime.gateway_url is required")
	}
	u, err := url.Parse(cfg.Runtime.GatewayURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("validation: runtime.gateway_url must be a valid http/https URL, got %q", cfg.Runtime.GatewayURL)
	}
	// Production TLS enforcement (unless dev mode)
	if u.Scheme == "http" && os.Getenv("CAV_NPC_DEV") != "1" {
		return fmt.Errorf("validation: runtime.gateway_url must use https:// in production (set CAV_NPC_DEV=1 for development)")
	}

	if cfg.Runtime.HealthPort <= 0 || cfg.Runtime.HealthPort > 65535 {
		return fmt.Errorf("validation: runtime.health_port must be in (0, 65535], got %d", cfg.Runtime.HealthPort)
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[cfg.Runtime.LogLevel] {
		return fmt.Errorf("validation: runtime.log_level must be one of debug/info/warn/error, got %q", cfg.Runtime.LogLevel)
	}

	// Budget
	if cfg.Runtime.Budget.MaxTokensPerHour <= 0 {
		return fmt.Errorf("validation: runtime.budget.max_tokens_per_hour must be positive")
	}
	if cfg.Runtime.Budget.MaxTokensPerDay <= 0 {
		return fmt.Errorf("validation: runtime.budget.max_tokens_per_day must be positive")
	}
	if cfg.Runtime.Budget.MaxTokensPerHour > cfg.Runtime.Budget.MaxTokensPerDay {
		return fmt.Errorf("validation: runtime.budget.max_tokens_per_hour (%d) must not exceed max_tokens_per_day (%d)",
			cfg.Runtime.Budget.MaxTokensPerHour, cfg.Runtime.Budget.MaxTokensPerDay)
	}

	// NPCs
	if len(cfg.NPCs) == 0 {
		return fmt.Errorf("validation: at least one [[npc]] must be defined")
	}

	names := make(map[string]bool, len(cfg.NPCs))
	for i, npc := range cfg.NPCs {
		prefix := fmt.Sprintf("validation: npc[%d] (%q)", i, npc.Name)

		// Name
		if !validNameRe.MatchString(npc.Name) {
			return fmt.Errorf("%s: name must match [a-z0-9_-]{1,64}", prefix)
		}
		if names[npc.Name] {
			return fmt.Errorf("%s: duplicate npc name", prefix)
		}
		names[npc.Name] = true

		// Role
		if !allowedRoles[npc.Role] {
			// Allow custom roles (they'll be validated at role registration time)
			if npc.Role == "" {
				return fmt.Errorf("%s: role is required", prefix)
			}
			// Non-builtin roles are accepted if Custom config is present
			if npc.Custom == nil && !strings.HasPrefix(npc.Role, "custom_") {
				return fmt.Errorf("%s: unknown role %q (builtin: %v)", prefix, npc.Role, builtinRoleNames())
			}
		}

		// LLM
		if !allowedProviders[npc.LLM.Provider] {
			return fmt.Errorf("%s: llm.provider must be one of deepseek/volcengine/dashscope, got %q", prefix, npc.LLM.Provider)
		}
		if npc.LLM.Endpoint == "" {
			return fmt.Errorf("%s: llm.endpoint is required", prefix)
		}
		if npc.LLM.APIKeyEnv == "" {
			return fmt.Errorf("%s: llm.api_key_env is required", prefix)
		}
		// Resolve API key from environment
		if os.Getenv(npc.LLM.APIKeyEnv) == "" {
			return fmt.Errorf("%s: environment variable %q (referenced by llm.api_key_env) is not set", prefix, npc.LLM.APIKeyEnv)
		}
		if npc.LLM.Model == "" {
			return fmt.Errorf("%s: llm.model is required", prefix)
		}
		if npc.LLM.MaxTokens < 16 || npc.LLM.MaxTokens > 32768 {
			return fmt.Errorf("%s: llm.max_tokens must be in [16, 32768], got %d", prefix, npc.LLM.MaxTokens)
		}
		if npc.LLM.Temperature < 0 || npc.LLM.Temperature > 2 {
			return fmt.Errorf("%s: llm.temperature must be in [0, 2], got %f", prefix, npc.LLM.Temperature)
		}

		// Rate limits
		if npc.RateLimit.LLMPerMinute <= 0 {
			return fmt.Errorf("%s: rate_limit.llm_per_minute must be positive", prefix)
		}
		if npc.RateLimit.PublishPer5s <= 0 {
			return fmt.Errorf("%s: rate_limit.publish_per_5s must be positive", prefix)
		}
	}

	return nil
}

func builtinRoleNames() []string {
	names := make([]string, 0, len(allowedRoles))
	for k := range allowedRoles {
		names = append(names, k)
	}
	return names
}
