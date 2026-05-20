package signal

import "fmt"

// ValidationError describes a specific field-level validation failure.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("signal validation: %s — %s", e.Field, e.Reason)
}

// Validate checks an outgoing EntropicSignal against the R13 quality gate.
// Returns nil if the signal is publishable.
//
// Checks performed:
//   - R13.1: type is a valid SignalType
//   - R13.1: posterior_shift has all required fields, confidence in [0,1], delta_bits >= 0
//   - R13.2: prior_confidence != posterior_confidence (zero-gain = noise)
//   - R13.3: grounding.type, grounding.source, grounding.evidence are non-empty
//   - R13.4: falsifiability is non-empty
//   - R13.6: uncertainty.known_failure_modes has at least 1 entry
func Validate(s *EntropicSignal) error {
	if s == nil {
		return &ValidationError{Field: "", Reason: "signal is nil"}
	}

	// Type
	if !ValidSignalTypes[s.Type] {
		return &ValidationError{Field: "type", Reason: fmt.Sprintf("invalid signal type %q", s.Type)}
	}

	// Heartbeat signals have relaxed requirements
	if s.Type == SignalHeartbeat {
		return nil
	}

	// Posterior Shift (required for non-heartbeat)
	if s.PosteriorShift == nil {
		return &ValidationError{Field: "posterior_shift", Reason: "required for non-heartbeat signals"}
	}
	ps := s.PosteriorShift

	if ps.Subject == "" {
		return &ValidationError{Field: "posterior_shift.subject", Reason: "must not be empty"}
	}
	if ps.Relation == "" {
		return &ValidationError{Field: "posterior_shift.relation", Reason: "must not be empty"}
	}
	if ps.Object == "" {
		return &ValidationError{Field: "posterior_shift.object", Reason: "must not be empty"}
	}
	if ps.PriorConfidence < 0 || ps.PriorConfidence > 1 {
		return &ValidationError{Field: "posterior_shift.prior_confidence", Reason: fmt.Sprintf("must be in [0,1], got %f", ps.PriorConfidence)}
	}
	if ps.PosteriorConfidence < 0 || ps.PosteriorConfidence > 1 {
		return &ValidationError{Field: "posterior_shift.posterior_confidence", Reason: fmt.Sprintf("must be in [0,1], got %f", ps.PosteriorConfidence)}
	}
	if ps.DeltaBits < 0 {
		return &ValidationError{Field: "posterior_shift.delta_bits", Reason: fmt.Sprintf("must be >= 0, got %f", ps.DeltaBits)}
	}

	// R13.2: zero information gain = noise
	if ps.PriorConfidence == ps.PosteriorConfidence {
		return &ValidationError{Field: "posterior_shift", Reason: "prior_confidence == posterior_confidence (zero information gain)"}
	}

	// Grounding (R13.3)
	if s.Grounding == nil {
		return &ValidationError{Field: "grounding", Reason: "required"}
	}
	if s.Grounding.Type == "" {
		return &ValidationError{Field: "grounding.type", Reason: "must not be empty"}
	}
	if s.Grounding.Source == "" {
		return &ValidationError{Field: "grounding.source", Reason: "must not be empty"}
	}
	if s.Grounding.Evidence == "" {
		return &ValidationError{Field: "grounding.evidence", Reason: "must not be empty"}
	}

	// Falsifiability (R13.4)
	if s.Falsifiability == "" {
		return &ValidationError{Field: "falsifiability", Reason: "must not be empty"}
	}

	// Uncertainty (R13.6)
	if s.Uncertainty == nil {
		return &ValidationError{Field: "uncertainty", Reason: "required"}
	}
	if len(s.Uncertainty.KnownFailureModes) == 0 {
		return &ValidationError{Field: "uncertainty.known_failure_modes", Reason: "must have at least 1 entry"}
	}

	return nil
}

// ValidateOutSignal checks an OutSignal (pre-publisher enrichment).
// Same rules as Validate but doesn't require id/from/timestamp.
func ValidateOutSignal(s *OutSignal) error {
	if s == nil {
		return &ValidationError{Field: "", Reason: "signal is nil"}
	}

	// Convert to EntropicSignal for validation (without header fields)
	full := &EntropicSignal{
		Type:           s.Type,
		PosteriorShift: s.PosteriorShift,
		Grounding:      s.Grounding,
		Uncertainty:    s.Uncertainty,
		Falsifiability: s.Falsifiability,
	}
	return Validate(full)
}
