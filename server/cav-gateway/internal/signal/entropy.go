// Package signal defines the structured entropic signal format for
// agent-to-agent cognitive communication.
//
// An entropic signal is NOT free text. It is a structured representation of:
//   - What the sender's belief state changed (posterior shift)
//   - Why it changed (grounding reference)
//   - How confident the sender is (uncertainty geometry)
//   - What would make the sender retract (falsifiability)
//
// This is the wire format for Charter §3.4 (Entropic Channel):
// "A message on the entropic channel encodes the sender's posterior
// distribution shift since last message."
//
// Key design: receivers don't need to understand the sender's ontology.
// They only need to understand the STRUCTURE of the signal — what changed,
// by how much, grounded in what evidence.
package signal

import (
	"encoding/json"
	"time"
)

// SignalType classifies the cognitive event.
type SignalType string

const (
	// Belief updates
	SignalLearning    SignalType = "learning"     // New knowledge acquired
	SignalRefinement  SignalType = "refinement"   // Existing belief updated
	SignalRetraction  SignalType = "retraction"   // Belief withdrawn
	SignalChallenge   SignalType = "challenge"    // Questioning another's claim

	// Consensus events
	SignalEndorsement SignalType = "endorsement"  // Supporting another's claim
	SignalVerdict     SignalType = "verdict"      // Consensus outcome

	// Meta
	SignalHeartbeat   SignalType = "heartbeat"    // Alive signal
	SignalCapability  SignalType = "capability"   // Capability announcement
)

// EntropicSignal is the structured message format for agent communication.
// Every field is designed to be machine-parseable by any paradigm.
type EntropicSignal struct {
	// === Header (routing + identity) ===
	ID        string    `json:"id"`
	Type      SignalType `json:"type"`
	From      string    `json:"from"`        // sender fingerprint
	Timestamp string    `json:"timestamp"`
	Sequence  uint64    `json:"seq"`         // monotonic per-sender sequence number

	// === Posterior Shift (the core entropic content) ===
	// What changed in the sender's belief state
	PosteriorShift *PosteriorShift `json:"posterior_shift,omitempty"`

	// === Grounding (why it changed) ===
	Grounding *SignalGrounding `json:"grounding,omitempty"`

	// === Uncertainty (how sure) ===
	Uncertainty *SignalUncertainty `json:"uncertainty,omitempty"`

	// === Falsifiability (what would undo this) ===
	Falsifiability string `json:"falsifiability,omitempty"`

	// === References ===
	PraxonRef   string   `json:"praxon_ref,omitempty"`    // related Praxon ID
	InReplyTo   string   `json:"in_reply_to,omitempty"`   // signal ID being responded to
	Tags        []string `json:"tags,omitempty"`          // hypothesis kinds / categories
}

// PosteriorShift describes what changed in the sender's belief.
type PosteriorShift struct {
	// The claim that was updated
	Subject  string `json:"subject"`           // what entity/concept
	Relation string `json:"relation"`          // causes/correlates/contradicts/refines
	Object   string `json:"object"`            // target entity/concept

	// The magnitude of change
	PriorConfidence    float64 `json:"prior_confidence"`     // before [0,1]
	PosteriorConfidence float64 `json:"posterior_confidence"` // after [0,1]
	DeltaBits          float64 `json:"delta_bits"`           // information gained (EIG)

	// Direction
	Direction string `json:"direction"` // "strengthen" | "weaken" | "reverse" | "new"
}

// SignalGrounding explains WHY the belief changed.
type SignalGrounding struct {
	Type        string `json:"type"`         // "observation" | "inference" | "peer_signal" | "tool_result"
	Source      string `json:"source"`       // tool name, peer fingerprint, or data URI
	Evidence    string `json:"evidence"`     // brief description of what was observed
	Reproducible bool  `json:"reproducible"` // can others reproduce this?
}

// SignalUncertainty describes the sender's confidence geometry.
type SignalUncertainty struct {
	Confidence              float64  `json:"confidence"`                // [0,1]
	CounterfactualNeighborhood string `json:"counterfactual_neighborhood"` // what nearby worlds look like
	KnownFailureModes       []string `json:"known_failure_modes"`       // where this might be wrong
}

// Validate checks if a signal has the minimum required fields.
func (s *EntropicSignal) Validate() error {
	if s.Type == "" {
		return ErrMissingType
	}
	if s.From == "" {
		return ErrMissingFrom
	}
	if s.Type != SignalHeartbeat && s.PosteriorShift == nil {
		return ErrMissingShift
	}
	return nil
}

// ToJSON serializes the signal.
func (s *EntropicSignal) ToJSON() ([]byte, error) {
	return json.Marshal(s)
}

// FromJSON deserializes a signal.
func FromJSON(data []byte) (*EntropicSignal, error) {
	var s EntropicSignal
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// NewSignalID generates a unique signal ID.
func NewSignalID() string {
	return "sig_" + time.Now().Format("20060102150405") + "_" + randomHex(6)
}

func randomHex(n int) string {
	t := time.Now().UnixNano()
	const hex = "0123456789abcdef"
	result := make([]byte, n*2)
	for i := 0; i < n; i++ {
		b := byte(t >> (i * 8))
		result[i*2] = hex[b>>4]
		result[i*2+1] = hex[b&0x0f]
	}
	return string(result)
}

// Sentinel errors
type signalError string

func (e signalError) Error() string { return string(e) }

const (
	ErrMissingType  = signalError("signal: missing type")
	ErrMissingFrom  = signalError("signal: missing from")
	ErrMissingShift = signalError("signal: non-heartbeat signal requires posterior_shift")
)
