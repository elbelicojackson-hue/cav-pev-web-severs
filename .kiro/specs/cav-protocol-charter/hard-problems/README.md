# CAV Hard Problems — Attack Briefs

**Purpose**: Each brief in this folder is a one-page attack draft for one of the seven known hard problems enumerated in `charter.md §7`. Charter v0.1 listed the problems by name only. These briefs supply the attack path, success criteria, and minimum viable implementation that §7 was missing.

**Status**: Charter §7 is upgraded from "problem list" to "attack-ready problem list" once all 7 briefs are accepted. This unlocks Charter v0.2.

## Index

| # | Problem | Priority | Status | Owner |
|---|---|---|---|---|
| HP-01 | Bootstrap (Prior Convergence) | P1 (gating) | Ready | — |
| HP-02 | Collective Hallucination | **P0** | **Ready (sub-spec target)** | — |
| HP-03 | Sybil & Identity Attacks | P2 | Ready | — |
| HP-04 | Continuous Latent Channel Capacity | P3 | Defer (architectural workaround) | — |
| HP-05 | Latent Injection | P1 | Ready | — |
| HP-06 | Quinean Indeterminacy | — | Substantially mitigated by design | — |
| HP-07 | Authority Capture | P4 | Governance (not protocol) | — |

## Reading order

Recommended for a first read:

1. **HP-02** — the keystone, because Axiom 1 of the Charter is mathematically vacuous without it
2. **HP-05** — required complement to HP-02 (challenges must not be forgeable at the latent layer)
3. **HP-01** — gating problem for the entropic channel
4. **HP-03** — Sybil defense, layered on top of HP-02
5. **HP-06** — short read; mostly documents how the design already addresses this
6. **HP-04** — defer reasoning; explains the two-tier architectural workaround
7. **HP-07** — boundary brief; explains what the protocol layer cannot solve and hands off to governance

## Sub-spec target

After all 7 briefs are accepted, the next document is `cav-anti-conformity-consensus` — the implementation spec for HP-02. It will be the first concrete protocol component beyond the Charter.

## Format convention

Each brief follows the same template:

```
## Problem statement (precise)
## Why it is hard
## Attack path
## Success criteria (falsifiable)
## Minimum viable implementation
## Dependencies
## Open questions
## Status
```

A brief that cannot fill in all eight sections honestly is not yet ready and is marked as such.
