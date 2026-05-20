package auth

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const CitizenDIDKey contextKey = "citizen_did"

// JWTManager handles JWT creation and validation.
type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTManager(secret []byte) *JWTManager {
	return &JWTManager{secret: secret, ttl: 24 * time.Hour}
}

func (j *JWTManager) Issue(did string, level int) (string, time.Time, error) {
	expiresAt := time.Now().Add(j.ttl)
	claims := jwt.MapClaims{
		"sub":   did,
		"level": level,
		"iat":   time.Now().Unix(),
		"exp":   expiresAt.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(j.secret)
	return signed, expiresAt, err
}

func (j *JWTManager) Validate(tokenStr string) (string, int, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return j.secret, nil
	})
	if err != nil || !token.Valid {
		return "", 0, err
	}
	claims := token.Claims.(jwt.MapClaims)
	did, _ := claims["sub"].(string)
	level, _ := claims["level"].(float64)
	return did, int(level), nil
}

// WithAuth is middleware that requires a valid JWT.
func WithAuth(jm *JWTManager, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":{"code":"unauthorized","message":"missing bearer token"}}`, http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		did, _, err := jm.Validate(tokenStr)
		if err != nil {
			http.Error(w, `{"error":{"code":"unauthorized","message":"invalid token"}}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), CitizenDIDKey, did)
		next(w, r.WithContext(ctx))
	}
}

// HandleVerify handles POST /v1/auth/verify
func HandleVerify(store *ChallengeStore, jm *JWTManager, registry interface{ EnsureRegistered(did string, fingerprint string) int }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DID       string `json:"did"`
			Nonce     string `json:"nonce"`
			Signature string `json:"signature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":{"code":"invalid_request","message":"bad json"}}`, http.StatusBadRequest)
			return
		}

		// Consume the nonce (checks DID match + expiry)
		if !store.Consume(req.Nonce, req.DID) {
			http.Error(w, `{"error":{"code":"auth_failed","message":"invalid or expired nonce"}}`, http.StatusUnauthorized)
			return
		}

		// Verify Ed25519 signature of the nonce bytes
		nonceBytes, err := hex.DecodeString(req.Nonce)
		if err != nil {
			http.Error(w, `{"error":{"code":"auth_failed","message":"invalid nonce hex"}}`, http.StatusBadRequest)
			return
		}

		if err := VerifyEd25519Signature(req.DID, req.Signature, nonceBytes); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"code":"auth_failed","message":"%s"}}`, err.Error()), http.StatusUnauthorized)
			return
		}

		// Compute agent fingerprint from their public key
		fingerprint := FingerprintFromDID(req.DID)
		if fingerprint == "" {
			http.Error(w, `{"error":{"code":"auth_failed","message":"cannot derive fingerprint from DID"}}`, http.StatusUnauthorized)
			return
		}

		// Register citizen and get level
		level := registry.EnsureRegistered(req.DID, fingerprint)

		// Issue JWT
		token, expiresAt, err := jm.Issue(req.DID, level)
		if err != nil {
			http.Error(w, `{"error":{"code":"internal","message":"jwt issue failed"}}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      token,
			"expires_at": expiresAt.UTC().Format(time.RFC3339),
			"citizen": map[string]interface{}{
				"did":         req.DID,
				"fingerprint": fingerprint,
				"level":       level,
			},
		})
	}
}

// HandleWhoami handles GET /v1/auth/whoami
func HandleWhoami(registry interface{ Get(did string) interface{} }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		did := r.Context().Value(CitizenDIDKey).(string)
		citizen := registry.Get(did)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(citizen)
	}
}
