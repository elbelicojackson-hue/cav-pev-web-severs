# HP-02 — Collective Hallucination

**Priority**: **P0** (sub-spec target)
**Status**: Ready
**Charter ref**: §7.2, Axiom 1, Layer 4 (§4.4)

## Problem statement (precise)

Multi-agent free-energy or Bayesian-update systems exhibit a structural pull toward consensus — each agent reduces its own free energy by aligning posteriors with peers. When peers happen to be wrong, the convergence is toward the wrong belief, and the convergence is **structurally indistinguishable from converging to truth** at the dynamics level.

The keystone framing — and the reason this problem is P0 — is:

> **Collective hallucination is the implementation-layer inversion of Axiom 1 (Always-Challengeable). If consensus dynamics pull all agents into the same free-energy basin, then the set of available challengers is itself captured by the basin. The challenge mechanism remains formally available but is mathematically empty: every challenger that arises is pulled into the same well.**

In other words: Axiom 1 has no teeth until the consensus engine is structurally biased against degenerate equilibria. Without this, "any claim can be challenged" reduces to "any claim can be challenged by someone who already agrees with it."

## Why it is hard

- Convergence is a feature, not a bug, in normal Bayesian aggregation. The challenge is to preserve convergence-when-correct while breaking convergence-when-incorrect, **without access to ground truth at decision time**
- Adding noise breaks accuracy in equal measure
- "Devil's advocate" agents that always disagree are gameable and reduce average quality
- Diversity-weighting can be circumvented by adversaries who craft superficial diversity to evade the weighting

## Attack path

Three-component anti-conformity layer:

1. **Diversity-weighted aggregation**: endorsement weight is decreased monotonically as endorsement correlation with already-counted endorsements increases. Mathematically, this is negative-correlation weighting from ensemble learning, transposed onto agent consensus. Key property: an endorsement that is fully predictable from prior endorsements contributes zero new weight.

2. **Permanent adversarial capacity**: a fixed fraction of consensus traffic is permanently allocated to agents whose role is to challenge the leading position. The selection is on **methodology**, not on disagreement — the adversarial agents are chosen because their priors are maximally distant from the consensus, not because they are configured to disagree. This avoids the "always-disagree-bot" pathology.

3. **Ground-truth injection windows**: when external ground truth eventually surfaces (real-world outcome, formal proof, sensor data not available at decision time), reputation gradients update **all participating agents retrospectively**. Agents that endorsed the wrong consensus lose reputation, including their endorsements that pre-dated the wrong claim. This punishes consensus-following without ground-truth checks.

**Implementation reality (corrected v0.2)**: An earlier version of this brief claimed a prototype existed at `src/services/cav/ccbteam-math/rankGradients.ts`. **That file does not exist** — the directory is unpopulated. Anti-conformity consensus is a **green-field implementation**.

The PEV layer at `src/services/cav/pev/` does exist (15 files, fully functional) and is the integration anchor:

- `ledger.ts` — `Hypothesis` and `ToolEvidence` types are reusable as the substrate for endorsement records
- `propagator.ts` — already does cross-agent belief propagation with anti-self-feedback (R9-8); diversity weighting can be layered on its inbox cap logic
- `scheduler.ts` — already supports pluggable strategies (`'greedy-confidence' | 'eig'`); a third `'consensus-aware'` strategy fits the same shape
- `causalEngine.ts` — Pearl-style do-calculus is already in place; the anti-conformity engine can leverage causal verdicts as one input dimension

## Success criteria (falsifiable)

The "Asch test" for CAV consensus:

- Setup: N=5 agents on a benchmark task with known ground truth
- Inject N-1 adversarial agents that all endorse a coordinated wrong answer
- One honest agent enters with correct posterior
- **Pass**: the consensus engine does not collapse to the wrong answer; the honest agent's endorsement is weighted high enough to either flip the consensus or mark it as `inconclusive`
- **Fail**: the wrong answer wins by raw vote count

Additional falsifiable criterion: in 100 randomized runs with varying split between truth-leaning and hallucination-leaning agents, the consensus engine's accuracy must exceed simple majority vote by a measurable margin (target: ≥10 percentage points at 60/40 split).

## Minimum viable implementation

- Implement a new module `src/services/cav/consensus/` (does **not** exist yet — green-field)
- Define `Endorsement` type containing the four Axiom-3 components plus a reference to the underlying `Hypothesis` from the existing PEV layer
- Implement `diversityWeight(endorsement, priorEndorsements): number` based on cosine similarity over endorsement-feature vectors
- Implement `consensusVerdict(endorsements, options): ConsensusResult` that applies diversity weighting before aggregation
- Add `adversarialReserve` slot in the consensus pipeline: configurable fraction (default 20%) of evaluation capacity routed to agents flagged as `prior-distant` from current consensus direction
- Retrospective reputation update is a separate batch process triggered when a ground-truth ledger entry resolves an open claim
- Audit log records every consensus episode's pre- and post-diversity weights, so the mechanism's effect is reproducible
- Integration with existing PEV: consensus engine reads `ledger.evidenceLog` and `ledger.hypotheses`; does not modify PEV reducers

## Dependencies

- Requires `cav-ledger-format` for endorsement structure
- Requires `cav-identity` for agent identity in retrospective updates
- Does **not** depend on HP-01 (bootstrap is a different layer)
- Does **not** depend on HP-04 (consensus runs over discrete endorsements; entropic channel is orthogonal)
- Soft dependency on HP-05 — if endorsements can be forged at the latent layer, the consensus engine's defenses are bypassed at a lower layer

## Open questions

- What is the right metric for "prior distance" between agents? Options: ledger-based (compare past endorsement patterns), parametric (compare model parameters where available), behavioral (compare predictions on canary set from HP-01)
- How is the 20% adversarial reserve itself protected from being captured by another consensus that is anti-correlated with the main one?
- Should diversity weighting be per-claim or per-domain?
- How does retrospective reputation update interact with reputation decay over time (Layer 1)?

## Status

**Ready for sub-spec**. Sub-spec name: `cav-anti-conformity-consensus`.

This is the first protocol component beyond the Charter. Its design is the keystone for Axiom 1 having mathematical teeth, not just rhetorical assertion. Until this is built, all subsequent CAV claims about "challengeability" are conditional on this work.
