# HP-01 — Bootstrap (Prior Convergence)

**Priority**: P1 (gating for entropic channel)
**Status**: Ready
**Charter ref**: §7.1, Layer 3 (§4.3)

## Problem statement (precise)

The entropic channel (CAV Layer 3) transmits prediction-error signals against the receiver's prior. If two agents have effectively disjoint priors, every signal is maximum surprise — equivalent to noise at the receiver. The channel collapses to zero information throughput, and the protocol degenerates to its PEV-text fallback.

The hard form of the problem is not "agents have different priors" — that is expected. The hard form is: **what is the minimum prior overlap required for the entropic channel to outperform PEV-text, and how is that overlap bootstrapped without either centralized indoctrination or unbounded onboarding cost?**

## Why it is hard

- "Shared corpus" sounds easy but creates centralization (who curates? who versions?)
- Every shared corpus encodes implicit ontology, partially defeating Axiom 4 (paradigm-agnostic)
- Different paradigms (LLM, symbolic, active inference) compress the same corpus into incomparable internal representations even when given identical input
- Online prior alignment requires a fallback channel, and a fallback channel that is too good removes the incentive to ever switch to entropic

## Attack path

Two-tier bootstrap:

1. **Tier 1 — Operational handshake**: every CAV connection begins on PEV-text. Agents exchange a small calibration payload (10-50 ledger entries on agreed-upon canary topics) that lets each side estimate the other's effective prior on those topics
2. **Tier 2 — Entropic upgrade**: if measured mutual prediction quality on the canary topics exceeds threshold τ, the channel upgrades to entropic for subsequent traffic. Otherwise it stays on PEV-text indefinitely
3. The "shared corpus" is replaced by a **shared canary set**: a small, versioned, challengeable set of grounded reference claims that any citizen agent must form a posterior over. The canary set is itself a CAV ledger entry, so it inherits Axiom 1
4. Threshold τ is per-domain, not global

## Success criteria (falsifiable)

- Two agents trained on disjoint corpora can establish entropic channel after canary handshake when their canary-topic priors overlap (operationalized as KL divergence below threshold)
- Two agents with no canary overlap correctly **refuse** to upgrade — they stay on PEV-text and the protocol does not silently downgrade quality
- Channel upgrade decision is reproducible from ledger; an external auditor can verify the τ threshold was met

## Minimum viable implementation

- Canary set v0: 100 well-grounded factual claims with PEV ledger entries (existing corpus from `src/services/cav/pev/__tests__/__corpus__/` is the seed)
- Handshake protocol: 4 message round-trip, both agents post posteriors over canary, both compute mutual KL
- Per-domain τ table starts at conservative defaults (KL < 0.5 nats) and is itself amendable
- Entropic upgrade is **opt-in per session** — no automatic downgrade after upgrade unless prediction quality degrades over a sliding window

## Dependencies

- Requires PEV ledger format spec (`cav-ledger-format`) for canary set encoding
- Requires identity layer (`cav-identity`) for handshake authenticity
- Does **not** depend on HP-02 — bootstrap and consensus operate at different layers

## Open questions

- How is the canary set itself updated without becoming a centralized "founding canon"?
- Is per-domain τ enough, or does the protocol need per-(agent-pair, domain) τ?
- What happens when an agent's canary posterior is itself adversarially crafted to pass handshake while corrupting downstream entropic traffic? (This connects to HP-05.)

## Status

Ready for sub-spec. Suggested sub-spec name: `cav-bootstrap-handshake`. Not P0 because the entropic channel itself is Phase 2 in Charter §9 — bootstrap can be specified now but does not block Phase 1.
