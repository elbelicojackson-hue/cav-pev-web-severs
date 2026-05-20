// CAV NPC Runtime — 原住民 Agent 运行时守护进程
//
// 独立于 cav-gateway 的长驻进程,通过 HTTP/WebSocket API 接入 CAV 网络。
// 管理多个 NPC_Instance,每个拥有独立 DID、角色配置和 LLM 后端。
//
// Usage:
//
//	cav-npc --config /path/to/config.toml
//	CAV_NPC_CONFIG=/path/to/config.toml cav-npc
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/anthropic-cav/cav-npc/internal/config"
	"github.com/anthropic-cav/cav-npc/internal/secrets"
)

func main() {
	configPath := flag.String("config", "", "path to TOML configuration file")
	flag.Parse()

	// Resolve config path: --config flag > CAV_NPC_CONFIG env
	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = os.Getenv("CAV_NPC_CONFIG")
	}
	if cfgPath == "" {
		fmt.Fprintln(os.Stderr, "fatal: no config path specified (use --config or CAV_NPC_CONFIG)")
		os.Exit(1)
	}

	// Load encrypted vault if specified (injects keys as env vars before config validation)
	vaultPath := os.Getenv("CAV_NPC_VAULT")
	if vaultPath != "" {
		masterPW := os.Getenv("CAV_NPC_MASTER_PASSWORD")
		if masterPW == "" {
			fmt.Fprintln(os.Stderr, "fatal: CAV_NPC_VAULT is set but CAV_NPC_MASTER_PASSWORD is empty")
			os.Exit(1)
		}
		vault, err := secrets.Open(vaultPath, masterPW)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: vault open failed: %v\n", err)
			os.Exit(1)
		}
		vault.InjectEnv()
		fmt.Fprintf(os.Stderr, "info: loaded %d keys from vault %s\n", len(vault.Keys()), vaultPath)
	}

	// Load and validate configuration
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: config load failed: %v\n", err)
		os.Exit(1)
	}

	// Setup structured logger
	var level slog.Level
	switch cfg.Runtime.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	slog.Info("cav-npc starting",
		"gateway_url", cfg.Runtime.GatewayURL,
		"npc_count", len(cfg.NPCs),
		"health_port", cfg.Runtime.HealthPort,
	)

	// Context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start health endpoint (stub — full implementation in M8/T33)
	healthAddr := fmt.Sprintf(":%d", cfg.Runtime.HealthPort)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		resp := map[string]any{
			"runtime_status": "starting",
			"npc_count":      len(cfg.NPCs),
			"npcs":           []any{},
		}
		for _, npc := range cfg.NPCs {
			resp["npcs"] = append(resp["npcs"].([]any), map[string]any{
				"name":   npc.Name,
				"role":   npc.Role,
				"status": "initializing",
			})
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	srv := &http.Server{Addr: healthAddr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("health server failed", "error", err)
			cancel()
		}
	}()

	slog.Info("health endpoint listening", "addr", healthAddr)

	// TODO: supervisor.Run(ctx, cfg) — implemented in M8/T34
	// For now, block until signal
	select {
	case sig := <-sigCh:
		slog.Info("received signal, shutting down", "signal", sig)
	case <-ctx.Done():
	}

	_ = srv.Close()
	slog.Info("cav-npc stopped")
}
