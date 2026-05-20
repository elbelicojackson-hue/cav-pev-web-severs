# HP-07 — Authority Capture

**Priority**: P4 (governance, not protocol)
**Status**: Boundary brief — protocol layer cannot fully solve this; hands off to governance
**Charter ref**: §7.7, §1, §8 (Anti-Goals)

## Problem statement (precise)

The deepest risk to CAV: the protocol is successful enough that its consensus outputs are treated by humans (and by other agents) as **final truth**, undermining Axiom 1 in practice even while preserving it formally. Once "what CAV says" becomes a default-trust position, the *availability* of challenge mechanisms is irrelevant — they are not used, because the social cost of disagreeing with consensus exceeds the perceived benefit, and the network's resources stop flowing toward challenges.

Authority capture is structurally analogous to what happened with peer review (originally a tool for surfacing disagreement, now widely treated as truth-stamping), with major news outlets in the 20th century, with central banks' inflation forecasts, and with credentialing systems generally. The pattern is consistent: an institution designed to be **questioned** becomes one that is **deferred to**, and the deference is not because the institution earned it but because questioning becomes costly.

## Why it is hard (and partially impossible)

This is not primarily a technical problem. The technical layer can ensure that challenge mechanisms remain *available*. The technical layer cannot ensure that challenges are actually *issued* at the rate required to keep the consensus honest. The latter depends on:

- Social incentives for challengers (which require a culture of challenge, not just a button)
- Resource allocation for adversarial work (who pays for the agents that exist solely to challenge consensus?)
- Governance over what counts as a legitimate challenge versus harassment
- Resistance to capture of the governance layer itself

These are governance questions. The protocol layer **cannot** decide them, and pretending it can is a category error.

## Attack path

The protocol layer commits to three structural defenses, then hands off to governance:

1. **Confidence calibration must not lie**: the Explanation Bridge (Layer 5) MUST surface the actual confidence level of any CAV claim, including the breakdown of how much confidence comes from consensus weight vs. from grounding evidence. This is enforced at the protocol level — Layer 5 implementations that round up confidence are non-conformant
2. **Deliberate-doubt scheduling**: at protocol level, a fraction of high-confidence consensus claims is **automatically resampled** for challenge — high-confidence claims are not exempt from re-evaluation. The selection is randomized to prevent adversarial gaming. Mechanism: every N seconds, the network re-opens a challenge slot on a random high-reputation claim, with adversarial agents (HP-02) given priority access to that slot
3. **Anti-canonization in protocol vocabulary**: the protocol does not provide any field, status, or record type meaning "settled," "canonical," or "authoritative." The strongest status a claim can hold is "high reputation, recently challenged, no successful refutation." The protocol vocabulary structurally lacks the words for "final"

After these three, the rest is governance:

- **Who has the right to operate a CAV node?** (governance question)
- **Who decides what the deliberate-doubt schedule should look like in practice?** (governance question)
- **What happens when a CAV consensus output is cited in legislation or regulation?** (governance, legal)
- **How is the governance layer itself protected from capture?** (the recursive form of the problem)

The protocol explicitly declines to answer these. It hands them to a governance layer that is **outside protocol scope** and is the responsibility of the broader CAV community when it exists.

## Success criteria (falsifiable, but only the technical part)

Technical-layer falsifiable criteria:

- Confidence calibration in Layer 5 outputs matches actual claim confidence within ε (auditable by external review)
- Deliberate-doubt schedule triggers re-challenges at the specified rate, verifiable from audit log
- Protocol vocabulary contains no field meaning "final" (verifiable by reading the wire format spec)

Governance-layer criteria are explicitly **not specified by the protocol**. Different deployments may answer them differently. The protocol's success criterion is that it does not foreclose the governance question.

## Minimum viable implementation

- Layer 5 confidence calibration is part of `cav-explanation-bridge` spec
- Deliberate-doubt scheduler is a small component, prototyped on top of HP-02's consensus engine
- Anti-canonization is enforced at wire-format design time (`cav-protocol-core`)

No new sub-spec is required for HP-07 itself. It is a constraint absorbed into other specs.

## Dependencies

- Layer 5 spec (confidence calibration cannot lie)
- HP-02 (deliberate-doubt depends on functioning consensus engine)
- Wire format (anti-canonization at the schema level)

## Open questions

These are open by design — they are governance questions and are not the protocol's to answer:

- Who has the right to revoke a citizen identity? (Currently: stake mechanism in HP-03 handles bad-faith identities; permanent revocation requires governance)
- What is the role of governments and regulators in a public CAV network?
- Should CAV consensus outputs be admissible as evidence in legal proceedings? Should they be?
- How does CAV interact with academic institutions, peer review, regulatory science?

## Status

**Boundary brief**. Technical mitigation is in place via the three structural defenses listed in Attack Path. Governance is explicitly out of protocol scope and will be addressed in a separate (eventual) governance document if the network reaches the scale where governance matters.

This is the protocol's most honest acknowledgment of its own limits: there is a class of failure mode that the protocol cannot solve by being well-designed, only by being **embedded in a broader social structure** that takes Axiom 1 seriously as a cultural norm, not just a technical guarantee. The protocol's job is to make that cultural norm easy to maintain. The job of maintaining it is humanity's.
