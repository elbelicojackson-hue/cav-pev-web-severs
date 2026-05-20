// Strict request validation for /v1/agent/* routes.
//
// Contract (server-side enforcement):
//
//   1. METHOD: Go's http.ServeMux pattern enforces the verb. Anything else
//      returns 405 with no handler logic invoked.
//
//   2. CONTENT-TYPE (POST only): exactly "application/json", optionally
//      followed by "; charset=utf-8". Anything else → 415.
//
//   3. CONTENT-LENGTH (POST only): bounded by maxBodyBytes. Body larger than
//      that is read up to the cap and the request is rejected → 413.
//      Streaming with no Content-Length is allowed but still capped.
//
//   4. JSON SHAPE: strict decoder. Unknown fields → 400. Trailing data after
//      the top-level value → 400. Wrong type → 400.
//
//   5. QUERY STRING: every endpoint declares an allowlist. Unknown keys → 400.
//      Repeated keys → 400. Values are parsed and bounded; out-of-range
//      values are clamped to the documented cap and reported in the response.
//
//   6. SEMANTIC FIELDS: enums (e.g. heartbeat status) are checked against an
//      allowlist; string lengths are capped; slices have length caps.
//
// Errors are returned as a single canonical envelope:
//
//   {
//     "error": {
//       "code": "validation_error",
//       "message": "human-readable reason",
//       "field":   "capabilities.languages[3]"   // optional
//     }
//   }
//
// `code` values are stable and machine-checkable. See errCode* constants.
package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// === Limits =================================================================

const (
	// maxBodyBytes is the hard cap on POST bodies. Equal to the manifest's
	// advertised max_signal_bytes so clients can't send a heartbeat that
	// would be rejected only after upload completes.
	maxBodyBytes int64 = 64 * 1024

	maxNicknameLen    = 64
	maxDescriptionLen = 512
	maxStatusLen      = 32
	maxNoteLen        = 256

	maxCapabilitiesItems    = 32 // per slice: hypothesis_kinds, tools, languages
	maxCapabilityStringLen  = 64 // per element of those slices
)

// allowedHeartbeatStatuses is the closed set for heartbeat.status.
var allowedHeartbeatStatuses = map[string]struct{}{
	"":        {}, // empty = unspecified
	"idle":    {},
	"working": {},
	"blocked": {},
}

// === Error envelope =========================================================

// errCode constants — stable, machine-checkable error codes.
const (
	errCodeNoIdentity     = "no_identity"
	errCodeInvalidDID     = "invalid_did"
	errCodeContentType    = "unsupported_media_type"
	errCodeBodyTooLarge   = "payload_too_large"
	errCodeUnknownField   = "unknown_field"
	errCodeInvalidJSON    = "invalid_json"
	errCodeInvalidQuery   = "invalid_query"
	errCodeUnknownParam   = "unknown_query_param"
	errCodeDuplicateParam = "duplicate_query_param"
	errCodeValidation     = "validation_error"
	errCodeStoreError     = "store_error"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// writeErrField is the internal writer that lets handlers attach a field path.
func writeErrField(w http.ResponseWriter, status int, code, msg, field string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorEnvelope{Error: errorBody{
		Code: code, Message: msg, Field: field,
	}})
}

// === Body decode ============================================================

// requireJSONContentType returns true iff the Content-Type header is exactly
// `application/json`, optionally with a charset=utf-8 parameter. The check
// is case-insensitive for the media type but rejects anything else (no
// `text/json`, no `application/*+json`, no missing header).
func requireJSONContentType(r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return errors.New("Content-Type header is required")
	}
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return errors.New("malformed Content-Type")
	}
	if !strings.EqualFold(mediaType, "application/json") {
		return errors.New("Content-Type must be application/json")
	}
	// Charset, if present, must be utf-8 (case-insensitive).
	if cs, ok := params["charset"]; ok && !strings.EqualFold(cs, "utf-8") {
		return errors.New("Content-Type charset must be utf-8")
	}
	return nil
}

// decodeStrictJSON reads at most maxBodyBytes from r.Body and decodes a single
// top-level JSON value into dst with strict mode (unknown fields rejected,
// trailing data rejected). Returns one of:
//
//   - nil on success
//   - errBodyTooLarge if the body exceeds the cap
//   - errInvalidJSON for any decode failure (with err.Error containing detail)
//
// Caller is responsible for sending the appropriate HTTP status.
func decodeStrictJSON(r *http.Request, dst interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBodyBytes)
	defer r.Body.Close()

	// Buffer the (capped) body so we can detect both decode errors and
	// trailing data without leaving partial state.
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		// http.MaxBytesReader returns "http: request body too large".
		if strings.Contains(err.Error(), "request body too large") {
			return errBodyTooLarge
		}
		return errInvalidJSON{msg: "read failed: " + err.Error()}
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errInvalidJSON{msg: err.Error()}
	}
	// Reject any trailing content (e.g. concatenated JSON values, junk bytes).
	if dec.More() {
		return errInvalidJSON{msg: "trailing data after top-level JSON value"}
	}
	return nil
}

// errBodyTooLarge is the sentinel for over-cap bodies.
var errBodyTooLarge = errors.New("body exceeds " + strconv.FormatInt(maxBodyBytes, 10) + " bytes")

// errInvalidJSON wraps any JSON decode failure with the underlying message.
type errInvalidJSON struct{ msg string }

func (e errInvalidJSON) Error() string { return "invalid JSON: " + e.msg }

// writeBodyDecodeError converts a decode error from decodeStrictJSON into the
// canonical envelope. Returns true if it handled the error (caller must
// return immediately), false if err was nil.
func writeBodyDecodeError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errBodyTooLarge) {
		writeErrField(w, http.StatusRequestEntityTooLarge, errCodeBodyTooLarge,
			"body exceeds maximum allowed size", "")
		return true
	}
	var ij errInvalidJSON
	if errors.As(err, &ij) {
		// Heuristic: if the message mentions an unknown field, surface a
		// distinct code so clients can differentiate "wrong shape" from
		// "extra field".
		if strings.Contains(ij.msg, "unknown field") {
			writeErrField(w, http.StatusBadRequest, errCodeUnknownField, ij.msg, "")
			return true
		}
		writeErrField(w, http.StatusBadRequest, errCodeInvalidJSON, ij.msg, "")
		return true
	}
	writeErrField(w, http.StatusBadRequest, errCodeInvalidJSON, err.Error(), "")
	return true
}

// === Query string ===========================================================

// validateQueryAllowlist enforces that r.URL.Query() contains only the keys
// listed in allowed and that no key appears more than once. Returns a
// validation error written to w (and true) on failure.
func validateQueryAllowlist(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	q := r.URL.Query()
	known := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		known[k] = struct{}{}
	}
	for key, vals := range q {
		if _, ok := known[key]; !ok {
			writeErrField(w, http.StatusBadRequest, errCodeUnknownParam,
				"unknown query parameter", key)
			return false
		}
		if len(vals) > 1 {
			writeErrField(w, http.StatusBadRequest, errCodeDuplicateParam,
				"query parameter must appear at most once", key)
			return false
		}
	}
	return true
}

// requireEmptyBody enforces that GET/DELETE requests carry no body content.
// Returns false (and writes the error) if any non-empty body is present.
func requireEmptyBody(w http.ResponseWriter, r *http.Request) bool {
	if r.ContentLength > 0 {
		writeErrField(w, http.StatusBadRequest, errCodeValidation,
			"request body is not allowed for this method", "")
		return false
	}
	// ContentLength may be -1 with chunked encoding; peek a byte.
	if r.Body != nil {
		buf := make([]byte, 1)
		n, _ := r.Body.Read(buf)
		if n > 0 {
			writeErrField(w, http.StatusBadRequest, errCodeValidation,
				"request body is not allowed for this method", "")
			return false
		}
	}
	return true
}

// parseBoundedInt parses ?key=N and clamps it into [min,max]. Returns def if
// the parameter is absent. Returns (0,false) if present but unparseable —
// caller decides whether to error or fall back to default. The "ok" return
// distinguishes "absent → use default" from "present but invalid".
func parseBoundedInt(r *http.Request, key string, def, min, max int) (n int, ok bool, present bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def, true, false
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return 0, false, true
	}
	if parsed < min {
		parsed = min
	}
	if parsed > max {
		parsed = max
	}
	return parsed, true, true
}

// === Semantic validators ====================================================

// validateHeartbeat performs field-level checks on a HeartbeatRequest.
// Returns "" on success or a validation message + offending field path.
func validateHeartbeat(req *HeartbeatRequest) (msg, field string) {
	if _, ok := allowedHeartbeatStatuses[req.Status]; !ok {
		return "status must be one of: idle, working, blocked", "status"
	}
	if len(req.Status) > maxStatusLen {
		return "status exceeds maximum length", "status"
	}
	if len(req.Note) > maxNoteLen {
		// Note is server-truncated rather than rejected (legacy behavior),
		// but we still cap *raw* incoming length to a sensible upper bound.
		req.Note = req.Note[:maxNoteLen]
	}
	if req.Capabilities != nil {
		if m, f := validateCapabilities(req.Capabilities); m != "" {
			return m, "capabilities." + f
		}
	}
	return "", ""
}

// validateCapabilities checks lengths and element counts. Capabilities are
// largely free-form, so the validation is structural (size caps) rather than
// semantic (no enum on hypothesis_kinds — see Charter §3.2).
func validateCapabilities(c interface{}) (msg, field string) {
	type capLike struct {
		HypothesisKinds []string `json:"hypothesis_kinds,omitempty"`
		Tools           []string `json:"tools,omitempty"`
		Languages       []string `json:"languages,omitempty"`
		Description     string   `json:"description,omitempty"`
		Nickname        string   `json:"nickname,omitempty"`
	}
	// Roundtrip through JSON to read fields without depending on the citizen
	// package's concrete type — keeps validation.go decoupled from import
	// cycles when the type evolves.
	buf, err := json.Marshal(c)
	if err != nil {
		return "capabilities are not serialisable", ""
	}
	var v capLike
	if err := json.Unmarshal(buf, &v); err != nil {
		return "capabilities have invalid shape", ""
	}
	if len(v.Nickname) > maxNicknameLen {
		return "nickname exceeds maximum length", "nickname"
	}
	if len(v.Description) > maxDescriptionLen {
		return "description exceeds maximum length", "description"
	}
	if m, f := checkStringSlice(v.HypothesisKinds, "hypothesis_kinds"); m != "" {
		return m, f
	}
	if m, f := checkStringSlice(v.Tools, "tools"); m != "" {
		return m, f
	}
	if m, f := checkStringSlice(v.Languages, "languages"); m != "" {
		return m, f
	}
	return "", ""
}

func checkStringSlice(s []string, name string) (msg, field string) {
	if len(s) > maxCapabilitiesItems {
		return name + " exceeds maximum number of entries", name
	}
	for i, item := range s {
		if len(item) > maxCapabilityStringLen {
			return name + " entry exceeds maximum length", name + "[" + strconv.Itoa(i) + "]"
		}
		if item == "" {
			return name + " entry must not be empty", name + "[" + strconv.Itoa(i) + "]"
		}
	}
	return "", ""
}
