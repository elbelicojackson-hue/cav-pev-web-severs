// Package instance implements the NPC_Instance main loop and its sub-components.
package instance

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/anthropic-cav/cav-npc/internal/client"
	"github.com/anthropic-cav/cav-npc/internal/identity"
	"github.com/anthropic-cav/cav-npc/internal/signal"
	"golang.org/x/time/rate"
)

// Publisher handles outbound signal publishing with R13 validation and R5.7 throttling.
type Publisher struct {
	pub     *client.Publisher
	kp      *identity.KeyPair
	limiter *rate.Limiter // 1 signal per 5s (R5.7)
	seq     uint64
}

// NewPublisher creates a Publisher with the given rate limit.
// publishPer5s is the max signals per 5-second window (default 1).
func NewPublisher(pub *client.Publisher, kp *identity.KeyPair, publishPer5s int) *Publisher {
	if publishPer5s <= 0 {
		publishPer5s = 1
	}
	// Rate: publishPer5s signals every 5 seconds
	r := rate.Every(5 * time.Second / time.Duration(publishPer5s))
	return &Publisher{
		pub:     pub,
		kp:      kp,
		limiter: rate.NewLimiter(r, 1),
	}
}

// Publish validates, enriches, and sends an OutSignal to the gateway.
// Returns nil even on validation failure (logs + drops, doesn't block pipeline).
func (p *Publisher) Publish(ctx context.Context, out *signal.OutSignal, inReplyTo string) error {
	// Build full EntropicSignal from OutSignal
	sig := &signal.EntropicSignal{
		ID:             newSignalID(),
		Type:           out.Type,
		From:           p.kp.DID,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Sequence:       p.nextSeq(),
		PosteriorShift: out.PosteriorShift,
		Grounding:      out.Grounding,
		Uncertainty:    out.Uncertainty,
		Falsifiability: out.Falsifiability,
		InReplyTo:      inReplyTo,
		Tags:           out.Tags,
	}

	// If OutSignal had in_reply_to set, prefer that
	if out.InReplyTo != "" {
		sig.InReplyTo = out.InReplyTo
	}

	// R13 validation gate
	if err := signal.Validate(sig); err != nil {
		slog.Warn("signal dropped: validation failed",
			"npc_did", p.kp.DID,
			"error", err,
			"signal_type", sig.Type,
		)
		return nil // Don't block pipeline
	}

	// R5.7: throttle
	if err := p.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("publisher: rate limit wait: %w", err)
	}

	// Sign the signal (simplified — full JCS signing in production)
	sig.From = identity.FingerprintFromPublicKey(p.kp.PublicKey)

	// Broadcast
	if err := p.pub.Broadcast(ctx, sig); err != nil {
		slog.Error("signal publish failed",
			"npc_did", p.kp.DID,
			"signal_id", sig.ID,
			"error", err,
		)
		return err
	}

	slog.Info("signal published",
		"npc_did", p.kp.DID,
		"signal_id", sig.ID,
		"signal_type", sig.Type,
		"in_reply_to", sig.InReplyTo,
	)
	return nil
}

func (p *Publisher) nextSeq() uint64 {
	p.seq++
	return p.seq
}

// newSignalID generates a unique signal ID (UUIDv7-like).
func newSignalID() string {
	t := time.Now().UnixMilli()
	return fmt.Sprintf("sig_%d_%s", t, randomB64(6))
}

func randomB64(n int) string {
	b := make([]byte, n)
	// Use timestamp-based pseudo-random for simplicity (real impl would use crypto/rand)
	t := time.Now().UnixNano()
	for i := range b {
		b[i] = byte(t >> (i * 8))
	}
	return base64.RawURLEncoding.EncodeToString(b)

}

// signJCS signs the signal using Ed25519 over JCS-canonicalized JSON.
// Placeholder — full implementation needs the JCS canonicalizer.
func signJCS(sig *signal.EntropicSignal, priv ed25519.PrivateKey) string {
	// TODO: implement proper JCS canonicalization + Ed25519 signing
	// For now, sign the signal ID as a placeholder
	signature := ed25519.Sign(priv, []byte(sig.ID))
	return base64.RawURLEncoding.EncodeToString(signature)
}
