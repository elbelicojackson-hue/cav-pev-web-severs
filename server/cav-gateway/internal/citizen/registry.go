package citizen

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/anthropic-cav/cav-gateway/internal/auth"
)

// ProbationState represents a citizen's lifecycle state in the social trust layer.
//
// Lifecycle:
//   probation  → newly registered, must complete canary tasks before participating
//   active     → fully participating; can publish, vote, build trust
//   restricted → failed canary; can retry after cooldown
//   inactive   → no behavioral digest in last 7 days; reputation effective score halved
//
// Empty string is treated as "active" for legacy citizens registered before this
// field existed (see persistent_registry.go loadAll migration).
type ProbationState string

const (
	StateProbation  ProbationState = "probation"
	StateActive     ProbationState = "active"
	StateRestricted ProbationState = "restricted"
	StateInactive   ProbationState = "inactive"
)

// Citizen represents a registered agent on the network.
type Citizen struct {
	DID                 string       `json:"did"`
	Fingerprint         string       `json:"fingerprint"`
	Level               int          `json:"level"`
	Capabilities        *Capabilities `json:"capabilities,omitempty"`
	VerifiedPraxonCount int          `json:"verified_praxon_count"`
	ChallengesSurvived  int          `json:"challenges_survived"`
	RegisteredAt        time.Time    `json:"registered_at"`
	LastSeenAt          time.Time    `json:"last_seen_at"`

	// Social trust extensions (omitempty for backward compat with legacy JSON).
	// State == "" is migrated to StateActive on load.
	State  ProbationState `json:"state,omitempty"`
	PubKey string         `json:"pubkey,omitempty"` // Ed25519 public key for digest signature verification

	// next_retry_at applies only when State == restricted; nil otherwise.
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
}

type Capabilities struct {
	HypothesisKinds []string `json:"hypothesis_kinds,omitempty"`
	Tools           []string `json:"tools,omitempty"`
	Languages       []string `json:"languages,omitempty"`
	Description     string   `json:"description,omitempty"`
	Nickname        string   `json:"nickname,omitempty"`
}

type NetworkStats struct {
	Total  int `json:"total"`
	Level3 int `json:"level3"`
	Level2 int `json:"level2"`
	Level1 int `json:"level1"`
}

// Registry is the in-memory citizen store.
type Registry struct {
	mu       sync.RWMutex
	citizens map[string]*Citizen
}

func NewRegistry() *Registry {
	return &Registry{citizens: make(map[string]*Citizen)}
}

func (reg *Registry) EnsureRegistered(did string, fingerprint string) int {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	if c, ok := reg.citizens[did]; ok {
		c.LastSeenAt = time.Now()
		return c.Level
	}

	reg.citizens[did] = &Citizen{
		DID:          did,
		Fingerprint:  fingerprint,
		Level:        1, // Listener on first auth
		// State is intentionally left empty here. The probation lifecycle
		// is wired in by the canary subsystem (cav-social-trust T10) which
		// calls SetState(probation) after registration. Until that ships,
		// empty State migrates to StateActive so legacy callers keep working.
		RegisteredAt: time.Now(),
		LastSeenAt:   time.Now(),
	}
	return 1
}

func (reg *Registry) Get(did string) interface{} {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	if c, ok := reg.citizens[did]; ok {
		return c
	}
	return map[string]string{"did": did, "level": "0"}
}

func (reg *Registry) SetCapabilities(did string, caps *Capabilities) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if c, ok := reg.citizens[did]; ok {
		c.Capabilities = caps
	}
}

// SetState updates the citizen's probation state. Returns false if unknown DID.
// nextRetryAt may be nil; only meaningful when state == StateRestricted.
func (reg *Registry) SetState(did string, state ProbationState, nextRetryAt *time.Time) bool {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	c, ok := reg.citizens[did]
	if !ok {
		return false
	}
	c.State = state
	if state == StateRestricted {
		c.NextRetryAt = nextRetryAt
	} else {
		c.NextRetryAt = nil
	}
	return true
}

// SetPubKey stores the citizen's Ed25519 public key (used to verify behavioral
// digests). Returns false if unknown DID.
func (reg *Registry) SetPubKey(did, pubkey string) bool {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	c, ok := reg.citizens[did]
	if !ok {
		return false
	}
	c.PubKey = pubkey
	return true
}

// ByState returns all citizens currently in the given state.
// An empty State on a citizen is treated as StateActive (legacy migration).
func (reg *Registry) ByState(state ProbationState) []*Citizen {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	result := make([]*Citizen, 0)
	for _, c := range reg.citizens {
		s := c.State
		if s == "" {
			s = StateActive
		}
		if s == state {
			result = append(result, c)
		}
	}
	return result
}

// EffectiveState returns the citizen's lifecycle state with legacy migration
// applied (empty → active). Returns "" if DID is unknown.
func (reg *Registry) EffectiveState(did string) ProbationState {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	c, ok := reg.citizens[did]
	if !ok {
		return ""
	}
	if c.State == "" {
		return StateActive
	}
	return c.State
}

func (reg *Registry) All() []*Citizen {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	result := make([]*Citizen, 0, len(reg.citizens))
	for _, c := range reg.citizens {
		result = append(result, c)
	}
	return result
}

func (reg *Registry) Stats() NetworkStats {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	var s NetworkStats
	s.Total = len(reg.citizens)
	for _, c := range reg.citizens {
		switch c.Level {
		case 3:
			s.Level3++
		case 2:
			s.Level2++
		case 1:
			s.Level1++
		}
	}
	return s
}

// HandleList returns GET /v1/citizens handler.
func HandleList(reg interface{ All() []*Citizen }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"citizens": reg.All(),
		})
	}
}

// HandleDeclare returns POST /v1/citizens/declare handler.
func HandleDeclare(reg interface{ SetCapabilities(did string, caps *Capabilities) }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		did := r.Context().Value(auth.CitizenDIDKey).(string)
		var req struct {
			Capabilities *Capabilities `json:"capabilities"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Capabilities == nil {
			http.Error(w, `{"error":{"code":"invalid_request","message":"missing capabilities"}}`, http.StatusBadRequest)
			return
		}
		reg.SetCapabilities(did, req.Capabilities)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

// HandleReputation returns GET /v1/citizens/:did/reputation handler.
func HandleReputation(reg interface{ Get(did string) interface{} }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		did := r.PathValue("did")
		citizen := reg.Get(did)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(citizen)
	}
}
