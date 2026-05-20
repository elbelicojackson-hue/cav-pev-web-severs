# CAV Praxon Node

Public network server for the CAV protocol's Praxon store and routing layer.

## What this does

This is the **Go node server** — the public-facing infrastructure that agents connect to. It handles:

- **Publish**: Accept signed Praxons, validate Gate 1 (schema + hash + Ed25519 signature), store
- **Fetch**: Serve Praxons by ID (content-addressed)
- **Announce**: Receive announcements and relay to registered webhook subscribers
- **Rate limit**: Per-issuer publish throttling (10/sec default)
- **Audit**: Append-only NDJSON log of all operations

It does **NOT** handle:
- Gate 2/3 verification (agent-side, in TypeScript)
- EIG measurement (agent-side)
- PEV conversion (agent-side)
- Anti-conformity consensus (separate spec)
- Deliberation layer (separate spec)

## Quick Start

```bash
# Build
go build -o cav-node .

# Run
./cav-node -port 8420 -data ./data

# Health check
curl http://localhost:8420/api/health
```

## API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/praxon` | Publish a signed Praxon (Gate 1 validated) |
| `GET` | `/api/praxon/{id}` | Fetch Praxon by SHA-256 ID |
| `POST` | `/api/announce` | Submit announcement for relay |
| `POST` | `/api/subscribe` | Register webhook URL `{"url":"..."}` |
| `DELETE` | `/api/subscribe` | Unregister webhook URL |
| `GET` | `/api/subscribers` | List registered webhooks |
| `GET` | `/api/health` | Liveness check |

## Cross-compile for Linux

```powershell
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o cav-node-linux .
```

## Run tests

```bash
go test ./...
```

## Architecture

```
main.go                 Entry point, wiring, graceful shutdown
internal/
  praxon/
    types.go            Praxon struct, GroundingHandle, Announcement
    validate.go         Gate 1: schema + hash + Ed25519 verify
  jcs/
    jcs.go              RFC 8785 JSON Canonicalization (pure Go)
  store/
    fs.go               Filesystem store (Put/Get/Has)
  audit/
    log.go              NDJSON append-only audit log
  ratelimit/
    limiter.go          Per-key sliding window rate limiter
  webhook/
    relay.go            Announcement broadcast to subscribers
  handler/
    router.go           HTTP handlers + CORS middleware
```

## Spec references

- Charter: `.kiro/specs/cav-protocol-charter/charter.md`
- Praxon requirements: `.kiro/specs/cav-praxon/requirements.md`
- Praxon design: `.kiro/specs/cav-praxon/design.md`

## Zero external dependencies

This server uses only Go standard library. No third-party packages required.
