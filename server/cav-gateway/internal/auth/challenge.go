package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// ChallengeStore holds pending nonces for signature verification.
type ChallengeStore struct {
	mu       sync.RWMutex
	pending  map[string]*pendingChallenge
	nonceTTL time.Duration
}

type pendingChallenge struct {
	DID       string
	Nonce     string
	ExpiresAt time.Time
}

func NewChallengeStore() *ChallengeStore {
	cs := &ChallengeStore{
		pending:  make(map[string]*pendingChallenge),
		nonceTTL: 60 * time.Second,
	}
	go cs.cleanup()
	return cs
}

func (cs *ChallengeStore) Create(did string) (string, time.Time) {
	nonce := make([]byte, 32)
	rand.Read(nonce)
	nonceHex := hex.EncodeToString(nonce)
	expiresAt := time.Now().Add(cs.nonceTTL)

	cs.mu.Lock()
	cs.pending[nonceHex] = &pendingChallenge{DID: did, Nonce: nonceHex, ExpiresAt: expiresAt}
	cs.mu.Unlock()

	return nonceHex, expiresAt
}

func (cs *ChallengeStore) Consume(nonce, did string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	p, ok := cs.pending[nonce]
	if !ok {
		return false
	}
	if p.DID != did || time.Now().After(p.ExpiresAt) {
		delete(cs.pending, nonce)
		return false
	}
	delete(cs.pending, nonce)
	return true
}

func (cs *ChallengeStore) cleanup() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		cs.mu.Lock()
		now := time.Now()
		for k, v := range cs.pending {
			if now.After(v.ExpiresAt) {
				delete(cs.pending, k)
			}
		}
		cs.mu.Unlock()
	}
}

// HandleChallenge returns a handler for POST /v1/auth/challenge
func HandleChallenge(store *ChallengeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DID string `json:"did"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DID == "" {
			http.Error(w, `{"error":{"code":"invalid_request","message":"missing did"}}`, http.StatusBadRequest)
			return
		}

		nonce, expiresAt := store.Create(req.DID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"nonce":      nonce,
			"expires_at": expiresAt.UTC().Format(time.RFC3339),
		})
	}
}
