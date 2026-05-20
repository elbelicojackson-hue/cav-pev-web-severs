// Package signal defines the EntropicSignal types mirrored from cav-gateway.
//
// IMPORTANT: This package does NOT import cav-gateway. Types are kept in sync
// via contract tests (cmd/npctest/contract_test.go). If the gateway changes
// its wire format, the contract test will fail and this file must be updated.
package signal

// SignalType classifies the cognitive event.
type SignalType string

const (
	SignalLearning    SignalType = "learning"
	SignalRefinement  SignalType = "refinement"
	SignalRetraction  SignalType = "retraction"
	SignalChallenge   SignalType = "challenge"
	SignalEndorsement SignalType = "endorsement"
	SignalVerdict     SignalType = "verdict"
	SignalHeartbeat   SignalType = "heartbeat"
	SignalCapability  SignalType = "capability"
)

// ValidSignalTypes is the closed set of allowed signal types.
var ValidSignalTypes = map[SignalType]bool{
	SignalLearning:    true,
	SignalRefinement:  true,
	SignalRetraction:  true,
	SignalChallenge:   true,
	SignalEndorsement: true,
	SignalVerdict:     true,
	SignalHeartbeat:   true,
	SignalCapability:  true,
}

// EntropicSignal is the structured message format for agent communication.
// Wire-compatible with cav-gateway/internal/signal.EntropicSignal.
type EntropicSignal struct {
	// Header
	ID        string    `json:"id"`
	Type      SignalType `json:"type"`
	From      string    `json:"from"`
	Timestamp string    `json:"timestamp"`
	Sequence  uint64    `json:"seq"`

	// Posterior Shift
	PosteriorShift *PosteriorShift `json:"posterior_shift,omitempty"`

	// Grounding
	Grounding *SignalGrounding `json:"grounding,omitempty"`

	// Uncertainty
	Uncertainty *SignalUncertainty `json:"uncertainty,omitempty"`

	// Falsifiability
	Falsifiability string `json:"falsifiability,omitempty"`

	// References
	PraxonRef string   `json:"praxon_ref,omitempty"`
	InReplyTo string   `json:"in_reply_to,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

// PosteriorShift describes what changed in the sender's belief.
type PosteriorShift struct {
	Subject             string  `json:"subject"`
	Relation            string  `json:"relation"`
	Object              string  `json:"object"`
	PriorConfidence     float64 `json:"prior_confidence"`
	PosteriorConfidence float64 `json:"posterior_confidence"`
	DeltaBits           float64 `json:"delta_bits"`
	Direction           string  `json:"direction"`
}

// SignalGrounding explains WHY the belief changed.
type SignalGrounding struct {
	Type         string `json:"type"`
	Source       string `json:"source"`
	Evidence     string `json:"evidence"`
	Reproducible bool   `json:"reproducible"`
}

// SignalUncertainty describes the sender's confidence geometry.
type SignalUncertainty struct {
	Confidence                 float64  `json:"confidence"`
	CounterfactualNeighborhood string   `json:"counterfactual_neighborhood"`
	KnownFailureModes          []string `json:"known_failure_modes"`
}

// OutSignal is the output from a Role's LLM processing.
// It lacks id/from/timestamp/signature — those are filled by the Publisher.
type OutSignal struct {
	Type           SignalType       `json:"type"`
	PosteriorShift *PosteriorShift  `json:"posterior_shift,omitempty"`
	Grounding      *SignalGrounding  `json:"grounding,omitempty"`
	Uncertainty    *SignalUncertainty `json:"uncertainty,omitempty"`
	Falsifiability string            `json:"falsifiability,omitempty"`
	InReplyTo      string            `json:"in_reply_to,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
}
