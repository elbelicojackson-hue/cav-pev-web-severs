package instance

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/anthropic-cav/cav-npc/internal/client"
	"github.com/anthropic-cav/cav-npc/internal/identity"
	"github.com/anthropic-cav/cav-npc/internal/signal"
)

// DigestLoop produces hourly behavioral digests (R7.3-4).
type DigestLoop struct {
	pub    *client.Publisher
	kp     *identity.KeyPair
	npcDID string

	mu             sync.Mutex
	publishedTypes map[signal.SignalType]int // type → count in current hour
	publishedTags  map[string]bool          // unique domain tags
	endorsements   int                      // endorsement signals sent
	verdictMatches int                      // endorsements that matched verdicts
	totalVerdicts  int                      // verdicts observed
}

// NewDigestLoop creates a digest loop.
func NewDigestLoop(pub *client.Publisher, kp *identity.KeyPair) *DigestLoop {
	return &DigestLoop{
		pub:            pub,
		kp:             kp,
		npcDID:         kp.DID,
		publishedTypes: make(map[signal.SignalType]int),
		publishedTags:  make(map[string]bool),
	}
}

// Run starts the hourly digest ticker. Blocks until ctx is cancelled.
func (d *DigestLoop) Run(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			d.emitDigest(ctx)
		}
	}
}

// RecordPublished tracks a published signal for digest computation.
func (d *DigestLoop) RecordPublished(sig *signal.EntropicSignal) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.publishedTypes[sig.Type]++
	for _, tag := range sig.Tags {
		d.publishedTags[tag] = true
	}
	if sig.Type == signal.SignalEndorsement {
		d.endorsements++
	}
}

// RecordVerdict tracks a verdict signal for alignment computation.
func (d *DigestLoop) RecordVerdict(matched bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.totalVerdicts++
	if matched {
		d.verdictMatches++
	}
}

func (d *DigestLoop) emitDigest(ctx context.Context) {
	d.mu.Lock()
	// Compute stats
	alignment := 0.0
	if d.totalVerdicts > 0 {
		alignment = float64(d.verdictMatches) / float64(d.totalVerdicts)
	}
	uniqueDomains := len(d.publishedTags)
	entropy := d.computeEntropy()
	signalCount := 0
	for _, c := range d.publishedTypes {
		signalCount += c
	}

	// Reset for next hour
	d.publishedTypes = make(map[signal.SignalType]int)
	d.publishedTags = make(map[string]bool)
	d.endorsements = 0
	d.verdictMatches = 0
	d.totalVerdicts = 0
	d.mu.Unlock()

	now := time.Now().UTC()
	periodEnd := now.Format(time.RFC3339)
	periodStart := now.Add(-1 * time.Hour).Format(time.RFC3339)

	digest := &client.BehavioralDigest{
		DID:                       d.npcDID,
		PeriodStart:               periodStart,
		PeriodEnd:                 periodEnd,
		VoteAlignmentWithMajority: alignment,
		UniqueDomainsActive:       uniqueDomains,
		SignalDiversityEntropy:    entropy,
		SignalCount:               signalCount,
		VoteCount:                 d.endorsements,
		PublicKey:                 base64.RawURLEncoding.EncodeToString(d.kp.PublicKey),
	}

	// Sign the digest (JCS simplified — sign the JSON body)
	body, _ := json.Marshal(digest)
	sig := ed25519.Sign(d.kp.PrivateKey, body)
	digest.Signature = base64.RawURLEncoding.EncodeToString(sig)

	if err := d.pub.SubmitDigest(ctx, digest); err != nil {
		slog.Warn("digest submission failed",
			"npc_did", d.npcDID,
			"error", err,
		)
		return
	}

	slog.Info("behavioral digest submitted",
		"npc_did", d.npcDID,
		"signal_count", signalCount,
		"domains", uniqueDomains,
		"entropy", entropy,
	)
}

// computeEntropy calculates Shannon entropy of signal type distribution.
func (d *DigestLoop) computeEntropy() float64 {
	total := 0
	for _, c := range d.publishedTypes {
		total += c
	}
	if total == 0 {
		return 0
	}

	entropy := 0.0
	for _, c := range d.publishedTypes {
		if c == 0 {
			continue
		}
		p := float64(c) / float64(total)
		entropy -= p * math.Log2(p)
	}
	return entropy
}
