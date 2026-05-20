package instance

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/anthropic-cav/cav-npc/internal/role"
	"github.com/anthropic-cav/cav-npc/internal/signal"
)

// Bid represents a single bid observed for a task.
type Bid struct {
	NPCDID     string
	Reputation float64
	Timestamp  time.Time
}

// BidResult indicates whether this NPC won or lost the bid.
type BidResult int

const (
	BidWon  BidResult = iota
	BidLost
	BidSkipped // role not capable
)

// Bidder implements the signal bidding mechanism (R8).
type Bidder struct {
	pub       *Publisher
	role      role.Role
	selfDID   string
	selfRep   float64 // current reputation in relevant domain
	bidWindow time.Duration

	mu       sync.Mutex
	seenBids map[string][]Bid // taskID → observed bids (LRU-like, capped)
}

// NewBidder creates a Bidder with the given bid window (default 10s).
func NewBidder(pub *Publisher, r role.Role, selfDID string, selfRep float64, bidWindow time.Duration) *Bidder {
	if bidWindow <= 0 {
		bidWindow = 10 * time.Second
	}
	return &Bidder{
		pub:       pub,
		role:      r,
		selfDID:   selfDID,
		selfRep:   selfRep,
		bidWindow: bidWindow,
		seenBids:  make(map[string][]Bid),
	}
}

// ShouldBid evaluates whether to bid on a task signal and waits for the bid window.
// Returns BidWon if this NPC should process the task, BidLost if outbid, BidSkipped if not capable.
func (b *Bidder) ShouldBid(ctx context.Context, sig *signal.EntropicSignal, inbox <-chan *signal.EntropicSignal) BidResult {
	// Check if this is a task signal
	if !isTaskSignal(sig) {
		return BidSkipped
	}

	// Check role capability (R8.1)
	if !b.role.Capable(sig) {
		return BidSkipped
	}

	taskID := sig.ID

	// Publish our bid (R8.2)
	bidSig := &signal.OutSignal{
		Type: signal.SignalEndorsement,
		PosteriorShift: &signal.PosteriorShift{
			Subject:             taskID,
			Relation:            "can_handle",
			Object:              b.selfDID,
			PriorConfidence:     0.0,
			PosteriorConfidence: b.selfRep,
			DeltaBits:           b.selfRep,
			Direction:           "up",
		},
		Grounding: &signal.SignalGrounding{
			Type:     "self_assessment",
			Source:   b.selfDID,
			Evidence: "reputation-based capability assessment",
		},
		Uncertainty: &signal.SignalUncertainty{
			Confidence:        b.selfRep,
			KnownFailureModes: []string{"higher-reputation peer may outbid"},
		},
		Falsifiability: "Lower-rep peer wins task; higher-rep peer outbids",
		InReplyTo:      taskID,
		Tags:           []string{"bid"},
	}

	if err := b.pub.Publish(ctx, bidSig, taskID); err != nil {
		slog.Warn("failed to publish bid", "task_id", taskID, "error", err)
		return BidSkipped
	}

	// Wait bid window, observing competing bids (R8.3)
	timer := time.NewTimer(b.bidWindow)
	defer timer.Stop()

	myBid := Bid{
		NPCDID:     b.selfDID,
		Reputation: b.selfRep,
		Timestamp:  time.Now(),
	}

	for {
		select {
		case <-ctx.Done():
			return BidLost

		case <-timer.C:
			// Window expired — check if we're still the highest (R8.4)
			if b.isHighestBid(taskID, myBid) {
				return BidWon
			}
			return BidLost

		case inSig, ok := <-inbox:
			if !ok {
				return BidLost
			}
			// Check if this is a competing bid for the same task
			if inSig.Type == signal.SignalEndorsement && inSig.InReplyTo == taskID {
				competing := Bid{
					NPCDID:     inSig.From,
					Reputation: 0,
					Timestamp:  time.Now(),
				}
				// Extract reputation from posterior_confidence
				if inSig.PosteriorShift != nil {
					competing.Reputation = inSig.PosteriorShift.PosteriorConfidence
				}

				b.recordBid(taskID, competing)

				// Early exit if clearly outbid (R8.5)
				if competing.Reputation > b.selfRep {
					slog.Debug("outbid by higher-rep peer",
						"task_id", taskID,
						"our_rep", b.selfRep,
						"their_rep", competing.Reputation,
					)
					return BidLost
				}
			}
		}
	}
}

// isTaskSignal checks if a signal is a task request (R8.1).
func isTaskSignal(sig *signal.EntropicSignal) bool {
	for _, tag := range sig.Tags {
		if tag == "task_request" || tag == "needs_analysis" {
			return true
		}
	}
	return false
}

// recordBid stores a competing bid for a task.
func (b *Bidder) recordBid(taskID string, bid Bid) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.seenBids[taskID] = append(b.seenBids[taskID], bid)

	// Cap at 1024 tasks tracked
	if len(b.seenBids) > 1024 {
		// Remove oldest (simple eviction)
		for k := range b.seenBids {
			delete(b.seenBids, k)
			break
		}
	}
}

// isHighestBid checks if our bid is the highest for a task (R8.4, R8.6).
func (b *Bidder) isHighestBid(taskID string, myBid Bid) bool {
	b.mu.Lock()
	bids := b.seenBids[taskID]
	b.mu.Unlock()

	for _, bid := range bids {
		if bid.Reputation > myBid.Reputation {
			return false
		}
		// R8.6: equal reputation → earlier timestamp wins
		if bid.Reputation == myBid.Reputation && bid.Timestamp.Before(myBid.Timestamp) {
			return false
		}
	}
	return true
}

// UpdateReputation updates the bidder's self-reputation score.
func (b *Bidder) UpdateReputation(rep float64) {
	b.mu.Lock()
	b.selfRep = rep
	b.mu.Unlock()
}
