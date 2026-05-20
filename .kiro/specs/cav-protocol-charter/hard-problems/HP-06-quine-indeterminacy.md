# HP-06 — Quinean Indeterminacy

**Priority**: — (substantially mitigated by Axiom 4)
**Status**: Documented; partially solved by design
**Charter ref**: §7.6, Axiom 4 (§2), §5.2

## Problem statement (precise)

W. V. O. Quine's indeterminacy of translation argument: there is no fact of the matter about what foreign-language utterances mean, because multiple incompatible translation manuals can be consistent with all available behavioral evidence. The engineering form for CAV: **two agents with different ontologies cannot in principle agree on what their messages refer to**, because the same external behavior of both agents is consistent with multiple incompatible interpretations of what their internal states represent.

This is the deep version of the "shared semantic layer" problem that defeats most cross-paradigm communication attempts.

## Why it is hard

It is not engineering-hard. It is **philosophically unsolvable at the semantic layer**. No amount of communication bandwidth, no shared corpus, no fine-tuning can establish that agent A's "X" means the same as agent B's "X". The only thing two agents can ever share is **observable behavior** and **the entities those behaviors point at in the world**.

## Attack path

CAV does not solve Quine. CAV **bypasses Quine at the reference layer**.

The mechanism is Axiom 4 (Paradigm-Agnostic Grounding):

1. Every CAV claim MUST attach to one or more **grounding handles**: pointers to verifiable evidence (an experimental result, a dataset row, a sensor measurement, another ledger entry) that any agent — regardless of its internal ontology — can independently access
2. When two agents disagree at the description layer, the protocol does not attempt to translate between their descriptions. Instead, both agents traverse to the grounding handle and run their own observation/verification on the underlying evidence
3. Disagreement at the description layer is acceptable. Disagreement on what the grounding handle resolves to is a different category of disagreement (often a real empirical question, sometimes a measurement-procedure dispute) and is handled by the consensus engine

This is not a complete solution. Two agents can still talk past each other on the descriptive layer indefinitely. But the protocol **never depends on description-layer agreement** for verification — only on grounding-layer access, which is tractable.

## Success criteria (falsifiable)

- Two agents trained on disjoint ontologies (e.g., one trained on biomedical literature, one trained on legal corpora) can verify each other's claims when both are presented as grounded ledger entries pointing at a common dataset
- An agent that produces ledger entries with weak, vague, or fabricated grounding handles is detected by reputation degradation as its claims fail to verify under challenge
- The protocol explicitly distinguishes "we disagree about description" (allowed, recorded as such) from "we disagree about grounding" (raises a challenge per Axiom 1)

## Minimum viable implementation

This is largely an existing design commitment, not new work:

- Ledger format requires non-null `grounding_handles: List[Reference]` on every claim
- Reference types include: dataset URI + row, experimental setup ID, prior ledger entry hash, sensor reading with timestamp, formal proof artifact
- A claim with empty grounding_handles is malformed and rejected
- Validator (`src/services/cav/pev/validator.ts`) is extended to enforce non-emptiness
- Challenge mechanism distinguishes description-layer challenge from grounding-layer challenge in the audit log

## Dependencies

- This is essentially a consequence of Axiom 4 plus PEV ledger format requirements. No new sub-spec required.

## Open questions

- What is a sufficient grounding handle in domains where direct observation is impossible (theoretical physics, mathematics, ethics)? Recommendation: in those domains, grounding handles point to formal artifacts (proofs, formal models) or to derived empirical predictions
- How does the protocol handle cases where both agents accept the grounding handle but extract incompatible features from the same evidence? (This is genuinely hard and is the failure mode of the workaround — but it is rarer than full description-layer indeterminacy)
- Is there a threshold below which a grounding handle is "too thin" to count? Yes, but the threshold is domain-specific and is itself a CAV claim subject to challenge

## Status

**Substantially mitigated by Axiom 4 and PEV ledger format requirements**. No new sub-spec is required. The constraints are absorbed into `cav-ledger-format` and `cav-protocol-core`.

This brief exists to document why no further work is required, and to make the Quinean argument visible in the protocol's intellectual record so that future contributors do not re-discover it as a new problem.
