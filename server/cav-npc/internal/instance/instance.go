package instance

import (
	"context"
	"log/slog"
	"time"

	"github.com/anthropic-cav/cav-npc/internal/client"
	"github.com/anthropic-cav/cav-npc/internal/config"
	"github.com/anthropic-cav/cav-npc/internal/identity"
	"github.com/anthropic-cav/cav-npc/internal/llm"
	"github.com/anthropic-cav/cav-npc/internal/role"
	"github.com/anthropic-cav/cav-npc/internal/signal"
	"golang.org/x/sync/errgroup"
)

// Instance represents a single NPC agent instance with its own identity,
// role, LLM backend, and goroutine tree.
type Instance struct {
	cfg       config.NPCConfig
	kp        *identity.KeyPair
	auth      *client.AuthClient
	gw        *client.GatewayClient
	provider  llm.Provider
	budget    *llm.Budget
	role      role.Role
	inbox     chan *signal.EntropicSignal
	bidInbox  chan *signal.EntropicSignal

	// Sub-components
	heartbeat  *HeartbeatLoop
	digest     *DigestLoop
	diversity  *DiversityChecker
	publisher  *Publisher
	pipeline   *Pipeline
	bidder     *Bidder
}

// New creates a new NPC Instance from configuration.
func New(cfg config.NPCConfig, kp *identity.KeyPair, gatewayURL string, globalBudget *llm.Budget) (*Instance, error) {
	// Auth client
	auth := client.NewAuthClient(gatewayURL, kp)
	gw := client.NewGatewayClient(gatewayURL, auth)

	// LLM provider
	perNPCBudget := llm.NewBudget(cfg.RateLimit.LLMPerMinute, globalBudget.Stats().MaxHourly, globalBudget.Stats().MaxDaily)
	provider, err := llm.Build(cfg.LLM, perNPCBudget)
	if err != nil {
		return nil, err
	}

	// Role
	factory, err := role.Lookup(cfg.Role)
	if err != nil {
		// Try custom role
		r, err2 := role.LoadCustom(cfg.Role, cfg.Custom)
		if err2 != nil {
			return nil, err
		}
		return buildInstance(cfg, kp, auth, gw, provider, perNPCBudget, r), nil
	}
	r, err := factory(cfg.Custom)
	if err != nil {
		return nil, err
	}

	return buildInstance(cfg, kp, auth, gw, provider, perNPCBudget, r), nil
}

func buildInstance(cfg config.NPCConfig, kp *identity.KeyPair, auth *client.AuthClient, gw *client.GatewayClient, provider llm.Provider, budget *llm.Budget, r role.Role) *Instance {
	pub := client.NewPublisher(gw)
	instPub := NewPublisher(pub, kp, cfg.RateLimit.PublishPer5s)
	bidder := NewBidder(instPub, r, kp.DID, 0.5, 10*time.Second)

	return &Instance{
		cfg:       cfg,
		kp:        kp,
		auth:      auth,
		gw:        gw,
		provider:  provider,
		budget:    budget,
		role:      r,
		inbox:     make(chan *signal.EntropicSignal, 256),
		bidInbox:  make(chan *signal.EntropicSignal, 64),
		heartbeat: NewHeartbeatLoop(pub, budget, kp.DID, cfg.Role, cfg.GuildTag),
		digest:    NewDigestLoop(pub, kp),
		diversity: NewDiversityChecker(kp.DID),
		publisher: instPub,
		pipeline:  NewPipeline(r, provider, instPub, bidder, kp.DID, 0.5),
		bidder:    bidder,
	}
}

// Run starts all instance goroutines and blocks until ctx is cancelled or a fatal error occurs.
// Design §2.3: errgroup model — any goroutine error cancels all others.
func (inst *Instance) Run(ctx context.Context) error {
	slog.Info("instance starting",
		"npc_did", inst.kp.DID,
		"role", inst.cfg.Role,
		"name", inst.cfg.Name,
	)

	// Authenticate first
	inst.auth.StartRefreshLoop(ctx)
	if _, err := inst.auth.Token(ctx); err != nil {
		return err
	}

	g, gctx := errgroup.WithContext(ctx)

	// Stream loop: WS → inbox
	g.Go(func() error {
		stream := client.NewStreamClient(
			inst.gw.BaseURL,
			inst.auth,
			inst.role.Filter(),
			inst.inbox,
		)
		return stream.Run(gctx)
	})

	// Heartbeat loop: 60s ticker
	g.Go(func() error {
		return inst.heartbeat.Run(gctx)
	})

	// Digest loop: 1h ticker
	g.Go(func() error {
		return inst.digest.Run(gctx)
	})

	// Diversity loop: 24h ticker
	g.Go(func() error {
		return inst.diversityLoop(gctx)
	})

	// Pipeline loop: inbox → process
	g.Go(func() error {
		return inst.pipelineLoop(gctx)
	})

	return g.Wait()
}

// pipelineLoop reads from inbox and processes each signal.
func (inst *Instance) pipelineLoop(ctx context.Context) error {
	batchEnabled, _ := inst.role.BatchMode()

	if batchEnabled {
		return inst.batchPipelineLoop(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig, ok := <-inst.inbox:
			if !ok {
				return nil
			}
			inst.heartbeat.MarkProcessing("processing signal " + sig.ID)
			inst.pipeline.Process(ctx, sig, inst.bidInbox)
			inst.heartbeat.MarkIdle()
		}
	}
}

// batchPipelineLoop accumulates signals and processes them in batches.
func (inst *Instance) batchPipelineLoop(ctx context.Context) error {
	_, interval := inst.role.BatchMode()
	batcher := role.NewBatcher(interval, 100)

	// Start batcher in background
	go batcher.Run()
	defer batcher.Close()

	// Feed inbox to batcher
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-inst.inbox:
				if !ok {
					return
				}
				batcher.Add(sig)
			}
		}
	}()

	// Process batches
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case batch, ok := <-batcher.Out():
			if !ok {
				return nil
			}
			inst.heartbeat.MarkProcessing("processing batch of " + string(rune('0'+len(batch)%10)) + " signals")
			inst.pipeline.ProcessBatch(ctx, batch)
			inst.heartbeat.MarkIdle()
		}
	}
}

// diversityLoop runs the 24h diversity self-check.
func (inst *Instance) diversityLoop(ctx context.Context) error {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_, shouldAdjust := inst.diversity.Check()
			if shouldAdjust {
				slog.Warn("diversity check triggered temperature adjustment",
					"npc_did", inst.kp.DID,
				)
				// TODO: adjust LLM temperature via config hot-reload (T35)
			}
			inst.diversity.Reset()
		}
	}
}

// Name returns the NPC instance name.
func (inst *Instance) Name() string { return inst.cfg.Name }

// DID returns the NPC's DID.
func (inst *Instance) DID() string { return inst.kp.DID }

// Role returns the role name.
func (inst *Instance) Role() string { return inst.cfg.Role }
