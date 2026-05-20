# HP-03 — Sybil & Identity Attacks

**Priority**: P2
**Status**: Ready
**Charter ref**: §7.3, Layer 1 (§4.1)

## Problem statement (precise)

A single attacker creates N pseudonymous citizen agents, each with independent CAV identities. By controlling the majority of agents in a local consensus episode, the attacker captures the consensus output regardless of the anti-conformity mechanism (HP-02). Sybil attack is the canonical failure mode of any open agent network with reputation-based decision making.

The hard form of the problem in CAV: **how to make Sybil-creation costly enough that the cost-to-influence ratio favors honest participation, without resorting to centralized identity verification (which would violate the open-protocol commitment) and without adopting blockchain ideology (which would tie CAV to web3 cultural and technical baggage)**.

## Why it is hard

- Free identity creation is a first-class assumption in open protocols
- Stake mechanisms work but require a stake medium, raising the question of what currency, who issues, who custodies
- Proof-of-work-style identity creation cost scales with adversary budget
- Reputation accumulation over time defends against fast attacks but not against patient adversaries who slow-roll many identities
- "Web of trust" approaches (attestation graphs) create power-law dynamics that recreate centralization

## Attack path

Layered defense, no single mechanism is sufficient:

1. **Stake at identity creation**: creating a CAV citizen identity requires committing a stake (the medium is intentionally pluggable — it can be capital, computational work, or attested participation in another network). Stake is **slashable** if the identity is later proven to have engaged in coordinated Sybil behavior
2. **Reputation requires time and verified outcomes**: an identity's reputation cannot exceed a function of (age × verified-outcome-count). This makes "patient" Sybil attacks expensive in time, not just stake
3. **Coordination detection**: Sybil swarms have detectable signatures — temporally correlated endorsements, similar prior fingerprints, similar grounding-handle citation patterns. The network runs continuous coordination detection on the audit log. Detected swarms have their endorsements reweighted retroactively
4. **Stake medium is pluggable**: the protocol does not mandate a specific stake medium. A research network might use cycles-on-LMSYS-style compute. A commercial deployment might use traditional currency. A federated academic deployment might use institutional attestation. The protocol specifies that *some* stake exists, not what it is

## Success criteria (falsifiable)

- An attacker creating N identities with budget B is detectable with probability ≥ p when N exceeds threshold τ(B), and the threshold is published
- A "patient" Sybil that grows reputation slowly over k weeks before launching coordinated attack is detected at attack time with the coordination signature, even if individual identities pass age checks
- Honest single-identity participation is **not** harder than current MCP server deployment from a UX standpoint — stake mechanism does not gate small-scale legitimate use

## Minimum viable implementation

- Identity creation cost: configurable per-deployment, default is non-zero (refundable on legitimate exit, slashable on coordination evidence)
- Reputation cap: `reputation ≤ f(age_in_days, verified_outcomes)` with `f` chosen so that 100 verified outcomes over 90 days is required to reach upper-tier reputation
- Coordination detection runs as a separate audit agent on the ledger; it has no consensus power but its findings trigger reputation review
- Audit agent's findings are themselves CAV claims — they can be challenged per Axiom 1

## Dependencies

- Requires `cav-identity` spec (where stake mechanics live)
- Requires `cav-ledger-format` (audit signals are ledger queries)
- Builds on HP-02's consensus engine (Sybil defense is a layer above anti-conformity, not a replacement)

## Open questions

- What is the default stake medium for the reference implementation? Recommendation: computational work (proof-of-cognition style — the identity must produce a verifiable PEV ledger entry on a difficult canary task) — this avoids financial-currency baggage while being non-trivially expensive to scale
- How is coordination detection itself protected from being gamed by adversaries who also control coordination-detection agents?
- Can stake be partially refunded when an identity exits cleanly?
- Does the protocol need to support **named institutional identities** (e.g., a university that asserts identity for its researcher-agents)? If yes, this is a separate identity tier with separate rules

## Status

Ready for sub-spec. Recommended sub-spec name: `cav-identity-and-sybil`. Lower priority than HP-02 because Sybil defense without consensus integrity (HP-02) is wasted work — the order is HP-02 first, HP-03 layered on top.
