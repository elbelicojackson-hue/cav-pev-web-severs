# HP-04 — Continuous Latent Channel Capacity

**Priority**: P3 (architectural workaround instead of frontal assault)
**Status**: Defer with two-tier architecture
**Charter ref**: §7.4, Layer 3 (§4.3)

## Problem statement (precise)

Transmitting continuous-vector messages over a public network at scale is an open research problem along multiple dimensions: bandwidth (latent vectors are typically 10²-10⁴ floats per message), encryption (continuous data resists standard encryption assumptions about plaintext distribution), compression (latent vectors have low entropy in some dimensions and high in others, defeating generic compression), and adversarial robustness (HP-05).

The honest framing: **CAV's entropic channel as described in the Charter is not deployable on the public internet at 10⁴-agent scale with current infrastructure, and pretending otherwise is research handwaving**.

## Why it is hard

- Continuous-vector signals from heterogeneous models live in different latent spaces; cross-model latent transmission requires either a canonical latent space (does not exist) or per-pair learned codecs (does not scale)
- Public-network transmission of latents creates a fingerprinting attack surface — adversaries can learn an agent's internal state from its latent broadcasts even without decoding semantics
- Compression-aware adversaries can craft inputs that explode bandwidth (latent denial-of-service)

## Attack path

**Architectural workaround instead of frontal assault**:

Two-tier channel architecture:

1. **Tier A — Local cluster, latent**: within a trust cluster (agents that have completed mutual handshake per HP-01 and have established codec compatibility), the entropic channel transmits compressed continuous vectors. Bandwidth is manageable because cluster size is bounded (recommendation: ≤ 100 agents per cluster)
2. **Tier B — Inter-cluster, discrete**: between clusters, the channel falls back to PEV-text. Cluster boundaries are explicit in the protocol — every CAV citizen agent declares which clusters it belongs to. Cross-cluster traffic uses the discrete channel exclusively
3. **No global latent broadcast**: the protocol does not support broadcasting continuous vectors to "everyone." This is by design — global latent broadcast is the failure mode HP-04 is named after

This concedes the ambitious version of the entropic channel (one global latent fabric) and replaces it with a federated version (latent within clusters, discrete between). The global agent mesh is still possible, but its global communication uses Tier B, not Tier A.

## Success criteria (falsifiable)

- Within a cluster of ≤ 100 agents on commodity infrastructure, latent channel sustains ≥ 10 messages/second/agent without bandwidth saturation
- Cross-cluster PEV-text traffic preserves ≥ 80% of the information content that the latent channel would have transmitted on the same task (measured by downstream consensus accuracy)
- No protocol-level mechanism allows latent broadcast outside a cluster (validated by adversary attempting cross-cluster latent injection — must be rejected by the wire format)

## Minimum viable implementation

- Cluster membership is a CAV identity attribute; an agent can belong to ≤ 5 clusters
- Codec negotiation happens at cluster join time (handshake produces a shared low-rank projection of each agent's latent space)
- Cross-cluster bridge: any agent in two clusters can act as a bridge, but the message it forwards is **always re-encoded** to PEV-text in transit. Bridge agents cannot pass latents directly between clusters
- Bandwidth limits per agent are enforced at the cluster boundary

## Dependencies

- Builds on HP-01 (handshake is the substrate for codec negotiation)
- Independent of HP-02 (consensus operates over discrete endorsements regardless of channel tier)

## Open questions

- Is 100-agent cluster size a hard ceiling, or can it scale to 1000 with better codec engineering?
- How are clusters formed and dissolved? Self-organizing or governance-decided?
- Can a cluster fork — split into two clusters with shared history?
- What happens when a bridge agent is corrupted (HP-05 risk)?

## Status

Deferred. The two-tier architecture is the recommended workaround. Frontal assault on global continuous-vector channel capacity is **not** on the CAV roadmap. If a research breakthrough makes global latent fabric feasible, this brief is amended at that time.

This deferral is honest, not failure: it acknowledges that one of the more ambitious-sounding parts of the original CAV vision is unrealistic at v1.0 scope, and replaces it with a federated alternative that delivers most of the value at deployable cost.
