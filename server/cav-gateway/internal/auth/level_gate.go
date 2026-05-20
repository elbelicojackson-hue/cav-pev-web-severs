// Level-gated authorization middleware.
//
// CAV Citizen Protocol defines four tiers (cav-citizen-gateway/requirements.md):
//
//   Level 0 — Observer:     read-only (fetch, subscribe). No JWT required.
//   Level 1 — Listener:     read + declare capabilities. JWT required.
//   Level 2 — Contributor:  read + publish Praxon. ≥1 verified Praxon.
//   Level 3 — Citizen:      full access (publish + challenge + vote).
//                            ≥3 verified + ≥1 survived challenge.
//
// The middleware reads the `level` claim from the JWT (set at issue time by
// HandleVerify based on the citizen registry). If the citizen's level is below
// the route's minimum, the request is rejected with 403 + a stable error code.
//
// Design notes:
//   - This is a HARD gate, not advisory. Even if the reputation vector says
//     the citizen is trustworthy, they cannot bypass the level requirement.
//   - Level is re-read from the registry on every request (not cached from
//     JWT issue time) so promotions/demotions take effect immediately.
//   - The JWT `level` claim is used as a fast-path hint; the registry is the
//     source of truth. If they diverge, the registry wins.
package auth

import (
	"fmt"
	"net/http"
)

// CitizenLevelProvider is the interface the level gate needs from the citizen
// registry. Keeping it minimal avoids importing the full citizen package.
type CitizenLevelProvider interface {
	// GetLevel returns the current level for a DID. Returns 0 if unknown.
	GetLevel(did string) int
}

// RequireLevel returns middleware that rejects requests from citizens below
// the specified minimum level. It must be chained AFTER WithAuth (which sets
// CitizenDIDKey in context).
//
// Usage in main.go:
//
//	mux.HandleFunc("POST /v1/praxon",
//	    auth.WithAuth(jm, auth.RequireLevel(registry, 2, praxonProxy.HandlePublish(hub))))
//
// Error response (403):
//
//	{"error":{"code":"insufficient_level","message":"requires level 2 (Contributor); current level 1"}}
func RequireLevel(provider CitizenLevelProvider, minLevel int, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		did, ok := r.Context().Value(CitizenDIDKey).(string)
		if !ok || did == "" {
			// Should not happen if WithAuth ran first, but defensive.
			http.Error(w,
				`{"error":{"code":"unauthorized","message":"missing identity in context"}}`,
				http.StatusUnauthorized)
			return
		}

		level := provider.GetLevel(did)
		if level < minLevel {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w,
				`{"error":{"code":"insufficient_level","message":"requires level %d (%s); current level %d (%s)"}}`,
				minLevel, levelName(minLevel), level, levelName(level))
			return
		}

		next(w, r)
	}
}

// levelName maps the integer level to its human-readable name per spec.
func levelName(level int) string {
	switch level {
	case 0:
		return "Observer"
	case 1:
		return "Listener"
	case 2:
		return "Contributor"
	case 3:
		return "Citizen"
	default:
		return "Unknown"
	}
}
