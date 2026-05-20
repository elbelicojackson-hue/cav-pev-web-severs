package signal

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Parse errors
var (
	ErrNoJSON        = errors.New("parse: no JSON object found in LLM output")
	ErrMalformedJSON = errors.New("parse: malformed JSON in LLM output")
)

// ErrValidationFailed wraps a ValidationError from post-parse validation.
type ErrValidationFailed struct {
	Wrapped *ValidationError
}

func (e *ErrValidationFailed) Error() string {
	return fmt.Sprintf("parse: validation failed: %s", e.Wrapped.Error())
}

func (e *ErrValidationFailed) Unwrap() error { return e.Wrapped }

// ParseLLMOutput extracts an OutSignal from raw LLM text output.
//
// Strategy (design §5.1):
//  1. Find the first '{' and last '}' to extract a JSON candidate
//     (tolerates LLM adding natural language before/after, markdown fences, etc.)
//  2. Unmarshal into OutSignal
//  3. Run ValidateOutSignal on the result
//
// Returns ErrNoJSON if no braces found, ErrMalformedJSON if JSON is invalid,
// or ErrValidationFailed if the parsed signal fails R13 checks.
func ParseLLMOutput(raw string) (*OutSignal, error) {
	// Strip markdown code fences if present
	cleaned := stripCodeFences(raw)

	// Find JSON boundaries
	start := strings.IndexByte(cleaned, '{')
	if start < 0 {
		return nil, ErrNoJSON
	}
	end := strings.LastIndexByte(cleaned, '}')
	if end < 0 || end <= start {
		return nil, ErrNoJSON
	}

	candidate := cleaned[start : end+1]

	var out OutSignal
	if err := json.Unmarshal([]byte(candidate), &out); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedJSON, err)
	}

	// Validate the parsed signal
	if err := ValidateOutSignal(&out); err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			return nil, &ErrValidationFailed{Wrapped: ve}
		}
		return nil, err
	}

	return &out, nil
}

// stripCodeFences removes common markdown code fence patterns that LLMs
// often wrap JSON responses in.
func stripCodeFences(s string) string {
	// Remove ```json ... ``` or ``` ... ```
	s = strings.TrimSpace(s)

	// Check for opening fence
	if strings.HasPrefix(s, "```") {
		// Find end of first line (the opening fence line)
		nl := strings.IndexByte(s, '\n')
		if nl >= 0 {
			s = s[nl+1:]
		}
		// Remove closing fence
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}

	return s
}
