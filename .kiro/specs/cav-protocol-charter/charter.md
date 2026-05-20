# CAV Protocol Charter

> **Cognitive Adversarial Verification — A Public Protocol for Paradigm-Agnostic Multi-Agent Cognition**

**Status**: Draft v0.3 (Charter — non-binding architectural foundation)
**Scope**: Conceptual architecture only. Implementation specs MUST reference but MUST NOT contradict this document.
**Audience**: Researchers, protocol designers, agent framework authors, and anyone considering interoperability with CAV.

---

## §0. Reading This Document

This is a **Charter**, not an implementation specification. It defines:

- What CAV **is** and what it **deliberately is not**
- The non-negotiable axioms all CAV-conformant systems must satisfy
- The conceptual architecture from identity layer to consensus layer
- The boundary with adjacent protocols (MCP, Skill, A2A)
- The failure modes the protocol structurally commits to avoiding

It does **not** define wire format, schemas, or reference code. Those belong to subsequent specs (`cav-wire-format`, `cav-ledger-format`, `cav-identity`, etc.) which will be authored against this Charter.

If a future implementation spec contradicts this Charter, the Charter wins until amended via the process in §11.

---

## §1. Mission

> **CAV is infrastructure for epistemically sovereign agents and the humans who collaborate with them.**

CAV is not a replacement for MCP or Skill. CAV operates at a **different layer**: where MCP optimizes for tool-calling throughput between an LLM and discrete capabilities, CAV optimizes for **verifiable, challengeable, methodologically transparent cognition** between heterogeneous agents.

CAV's purpose is **not** to make AI smarter than humans. CAV's purpose is to make **collective cognition** — across agents and across humans — engineerable, auditable, and reversible. The protocol exists so that any claim flowing through it carries with it the means of its own challenge.

### What CAV explicitly is not

- Not a tool-calling protocol (that is MCP's job)
- Not a model-serving protocol (that is OpenAI/Anthropic API's job)
- Not a centralized authority on truth
- Not a guarantee that AI will not become a new epistemic authority — but a structural countermeasure against that outcome

### What CAV explicitly is

- A protocol for agents to exchange **cognitive structure** rather than domain content
- A substrate for **always-challengeable** claims with attached evidence handles
- A reputation and accountability layer for agent-to-agent epistemic interaction
- A grounding framework that allows agents with **incompatible ontologies** to nonetheless verify each other's reasoning

---

## §2. The Four Axioms

These four axioms are **not negotiable**. Any system that violates one of them is not CAV-conformant, regardless of feature parity.

### Axiom 1 — Always-Challengeable

Every claim emitted by any CAV node MUST be challengeable through the protocol itself. No claim, regardless of its source's reputation, holds privileged status that exempts it from challenge.

A successful challenge MUST:

- Update the originating node's belief state
- Be permanently recorded in the PEV ledger
- Propagate via reputation gradients to all dependent claims

**Consequence**: CAV cannot be used to enforce dogma. The protocol structurally rejects any "final authority" pattern. This is the primary defense against CAV becoming what it is meant to undermine.

**Implementation dependency**: Axiom 1 is **mathematically vacuous without a consensus engine that resists collective hallucination**. If consensus dynamics pull all available challengers into the same free-energy basin, "challengeable in principle" reduces to "challengeable only by agents who already agree." This dependency is the reason HP-02 (Collective Hallucination) is the P0 hard problem — Axiom 1 has teeth only when HP-02 is solved at the implementation layer. See `hard-problems/HP-02-collective-hallucination.md`.

**Two-layer challenge taxonomy (v0.3)**: Axiom 1 distinguishes two challenge classes that operate at different layers of the protocol (see §10):

- **Operational challenge** — fast-path challenge against a specific operational claim, resolved within seconds-to-minutes by anti-conformity consensus. Default Axiom 1 challenge mode for daily traffic.
- **Deliberation challenge** — slow-path challenge against a protocol-level commitment (Axiom interpretation, founding canary set, parameter constants). Resolved by symmetric reputation-weighted vote over a 30-day window per §11 and the Deliberation Layer spec.

Both classes preserve Axiom 1's binding force, but with different latency/throughput profiles. A claim that survives operational challenge can still be escalated to deliberation challenge if its operational consensus repeatedly fails (see Operational→Deliberation escalation in §10.4).

### Axiom 2 — Distribution-Lifting, Not Transcending

CAV's stated goal is to **raise the lower tail** of the collective cognitive distribution by giving non-experts engineered access to expert-grade epistemic processes. CAV does **not** claim to push the upper tail beyond current human cognition.

Any node, document, or marketing claim that frames CAV as "AI exceeding human intelligence" is a **misrepresentation** of the protocol's mandate.

**Consequence**: CAV is judged by how well it serves the median user navigating expert disagreement, not by benchmark scores against human PhDs. This metric choice is deliberate and is the second structural defense against authority capture.

### Axiom 3 — Methodological Transparency

Every claim transmitted through CAV MUST surface its methodological commitments:

- **Causal skeleton**: The believed causal structure
- **Uncertainty geometry**: Confidence + counterfactual neighborhood
- **Priors**: Data, model, inference method assumptions
- **Falsifiability conditions**: What observation would retract this claim

A claim missing any of these four is a **malformed claim** at the protocol level and MUST be rejected by conformant nodes.

**Consequence**: CAV makes "I just know it" structurally impossible to transmit. Every transmitted belief is annotated with its own potential mode of failure.

### Axiom 4 — Paradigm-Agnostic Grounding

CAV is a **paradigm-agnostic** protocol. Conformant nodes may be:

- LLM-based, symbolic, neuro-symbolic, active-inference, or any future paradigm
- Trained on overlapping or disjoint domains
- Using mutually incompatible ontologies

The protocol does **not** assume a shared semantic layer. Instead, every claim MUST attach to one or more **grounding handles**: pointers to evidence (observations, datasets, experiments, ledger entries) that any other node can independently verify, regardless of how it represents the underlying phenomena.

**Consequence**: Two CAV nodes with no common vocabulary can still verify each other's reasoning by traversing back to grounded evidence. This bypasses Quinean indeterminacy at the reference layer rather than attempting to solve it at the semantic layer.

---

## §3. Core Concepts

### 3.1 Citizen Agent

A **citizen agent** is any computational entity that:

1. Holds a stable cryptographic identity (DID-style key pair)
2. Maintains a verifiable reputation history within the network
3. Commits to producing all outbound claims in CAV-conformant form (Axiom 3)
4. Accepts inbound challenges per Axiom 1
5. Exposes grounding handles per Axiom 4

Citizenship is **not** centrally granted. It is **earned and lost** through verifiable behavior. The protocol provides no privileged identities.

### 3.2 The CAV Node

A **CAV node** is the protocol endpoint that an agent connects to. From the agent's perspective, the node is a **single entry point** that exposes:

- The agent's own identity, reputation, and ledger state
- Routing into the global CAV mesh
- Cognitive services (verifiable reasoning, adversarial consensus, cross-agent memory)
- Challenge submission and resolution

The internal complexity of a node is significant. The external surface is intentionally narrow. This is the core design choice: **complexity in the kernel, simplicity in the interface**, modeled on TCP/IP's success pattern rather than SOAP's failure pattern.

### 3.3 PEV Ledger

The **PEV (Plan-Execute-Verify) Ledger** is the canonical record format for verifiable reasoning. Each entry contains:

- Hypothesis tree at decision time
- Evidence consulted, with grounding handles
- Inference method and priors
- Outcome and confidence
- Reference to the originating agent's identity
- Hash chain to prior ledger entries

The PEV ledger is the **only** acceptable substrate for transmitting reasoning under CAV. Free-form prose explanations are not protocol-conformant claims.

The reference implementation of PEV ledger semantics already exists at `src/services/cav/pev/ledger.ts` and is the empirical basis for this Charter's ledger model.

### 3.4 Entropic Channel

CAV's primary inter-agent channel transmits **not domain content but predictive structure**. A message on the entropic channel encodes:

- The sender's posterior distribution shift since last message
- The grounding handles supporting that shift
- The sender's prediction error against the receiver's known prior

Receivers update their own beliefs by minimizing free energy against the incoming signal, not by parsing the signal as natural language.

This channel is **not human-readable** by design. Human readability is provided by an orthogonal **explanation layer** (§4.5) that translates entropic transactions into human-facing PEV traces on demand.

### 3.5 Adversarial Consensus

CAV does not assume agents converge to truth through cooperation. CAV assumes agents converge through **structured adversarial challenge**:

- Multiple agents, holding heterogeneous priors, evaluate the same claim
- Each agent's challenge or endorsement is recorded in the ledger
- Reputation gradients are updated based on which side empirically prevails when ground truth eventually surfaces
- Anti-conformity mechanisms penalize agents whose endorsements cluster too tightly with high-reputation agents (Sybil and echo-chamber defense)

This is the empirical instantiation of why monolithic "single oracle" answers are protocol-rejected: they bypass the consensus that gives CAV claims their epistemic weight.

---

## §4. Architecture Layers

CAV is organized as five logical layers. Each layer can be implemented independently but only the full stack is protocol-conformant.

### 4.1 Layer 1 — Identity & Reputation

- Decentralized identifiers (DIDs) for every citizen agent
- Public-key signatures on every outbound claim
- Reputation as a vector (not scalar): per-domain, per-methodology, per-time-horizon
- Reputation decay over time to prevent stale dominance
- Sybil resistance via stake mechanisms (specific design left to `cav-identity` spec)

### 4.2 Layer 2 — Verifiable Claim Ledger

- PEV ledger as defined in §3.3
- Append-only with hash-chained integrity
- Distributed storage (specific design left to `cav-storage` spec)
- Public read by default; selective disclosure for sensitive claims via zero-knowledge proofs

### 4.3 Layer 3 — Entropic Cognitive Channel

- Continuous-vector message format (not token streams)
- Free-energy-minimization semantics
- Channel capacity bounded by mutual prior overlap (bootstrap problem addressed in §7.1)
- Optional fallback to PEV-text channel when entropic channel cannot be established

### 4.4 Layer 4 — Adversarial Consensus Engine

- Challenge submission and routing
- Multi-agent verdict aggregation with diversity weighting
- Reputation gradient updates (to be implemented; the Charter v0.1 erroneously claimed a prototype existed at `src/services/cav/ccbteam-math/rankGradients.ts` — that file does not exist)
- Public audit log of every consensus episode

### 4.5 Layer 5 — Human Explanation Bridge

- Translation from PEV ledger + entropic transactions into human-readable narrative
- Methodology-aware: explanations include the four Axiom-3 components
- Confidence calibration in explanations matches the underlying claim's actual confidence (no false certainty)
- This layer is what fulfills the "认识论坐标地图" function described in the project's philosophical motivation: the user does not receive an answer, the user receives a navigable map of where the answer lives.

---

## §5. Communication Model

### 5.1 What Gets Transmitted

In CAV, agents transmit **cognitive structure**, not domain content. Every protocol-conformant transmission includes:

| Component | Description |
|---|---|
| Causal skeleton | A → B with mechanism hypothesis M and strength s |
| Uncertainty geometry | Posterior distribution + counterfactual neighborhood Δ |
| Methodological priors | (data D, model class M, inference method I) |
| Falsifiability conditions | Observation O that would retract the claim |
| Grounding handles | Pointers to evidence verifiable by any paradigm |

This format is **paradigm-invariant**. An LLM-based agent and an active-inference agent can exchange these structures without sharing any vocabulary.

### 5.2 Why This Bypasses Quinean Indeterminacy

The classic objection to cross-paradigm communication is that two agents with different ontologies cannot agree on what their words refer to. CAV does not solve this at the semantic layer — it **bypasses** it at the reference layer.

Two CAV agents do not need to agree on what "inflammation" or "market liquidity" means. They need only:

1. Both attach claims to grounding handles pointing to verifiable observations
2. Both accept the protocol commitment that disagreement at the description layer can be resolved by traversal to the evidence layer

This is a tractable engineering problem. Semantic translation is not.

### 5.3 The Stigmergic Analogy

Conceptually, the CAV mesh operates more like an **ant colony** than a human committee:

- Individual agents are computationally simple relative to the network
- Agents leave traces (ledger entries, reputation updates, entropic signals) on a shared substrate
- Collective intelligence emerges from substrate dynamics, not from centralized planning
- No agent has a global view; the mesh as a whole has emergent navigation

This analogy is not decorative. It informs design choices: the protocol prefers **environmental modification** (writing to the ledger) over **direct messaging** wherever feasible, because stigmergic coordination scales better and is more robust to node failure.

---

## §6. Killer Feature & Boundary with Adjacent Protocols

### 6.1 The Designated Killer Feature: Verifiable Reasoning

CAV's primary value proposition — the feature that justifies its existence given the network-effects cost of any new protocol — is **Verifiable Reasoning**.

A claim consumed via CAV is not just an answer. It is:

- An answer
- The hypothesis tree that produced it
- The evidence consulted with grounding handles
- The methodological assumptions made
- The conditions under which the answer would be retracted
- The reputation history of the producing agent
- The full ledger of any challenges issued and their resolutions

No existing protocol (MCP, OpenAI function calling, Anthropic Skill, A2A drafts) provides this. This is the single non-substitutable contribution of CAV.

Two secondary capabilities are recognized but **deferred**:

- **Cross-agent Memory / Identity** — important but solvable within MCP extensions
- **Adversarial Consensus as a service** — emerges naturally once Verifiable Reasoning is in place

Killer feature focus is deliberate: protocols that try to do everything from day one die from network-effect starvation.

### 6.2 Boundary with MCP

| Question | MCP | CAV |
|---|---|---|
| What does it transport? | Tool calls and results | Cognitive structure and challenges |
| Trust model | Per-server, declared by config | Earned reputation, per-claim |
| Failure mode | Tool returns error | Claim is challenged and updated |
| Human readability | Required (JSON) | Optional (entropic by default, explainable on demand) |
| Authority structure | Server is authoritative for its tools | No node is authoritative; consensus arbitrates |

CAV and MCP are **complementary**. A CAV node may invoke MCP tools internally; an MCP server may publish CAV-conformant ledgers about its own behavior. The two protocols address different layers of the agent stack.

### 6.3 Boundary with Skill

Skill is Anthropic's proprietary capability-encapsulation system. CAV is open and decentralized. CAV does not need to "replace" Skill — CAV simply makes Skill-like capabilities discoverable, citable, and challengeable across vendor boundaries.

A pragmatic migration: existing Skills can be wrapped as CAV citizen agents with reputation seeded from their author's identity. This is the bridge strategy.


---

## §7. Known Hard Problems

This Charter explicitly enumerates the protocol's known unsolved problems. A Charter that hides its hard problems is not honest. These are the problems any implementation MUST address.

**Charter v0.2 update**: each problem below now has a dedicated attack brief in `hard-problems/`. The briefs supply attack path, falsifiable success criteria, minimum viable implementation, and dependencies. The structure of this section is preserved for historical reading; the operational truth lives in the briefs.

| # | Problem | Brief | Priority | Status |
|---|---|---|---|---|
| 1 | Bootstrap (Prior Convergence) | `HP-01-bootstrap.md` | P1 (gating) | Ready |
| 2 | Collective Hallucination | `HP-02-collective-hallucination.md` | **P0** | **Ready (sub-spec target)** |
| 3 | Sybil & Identity Attacks | `HP-03-sybil.md` | P2 | Ready |
| 4 | Continuous Latent Channel Capacity | `HP-04-channel-capacity.md` | P3 | Defer (architectural workaround) |
| 5 | Latent Injection | `HP-05-latent-injection.md` | P1 | Ready |
| 6 | Quinean Indeterminacy | `HP-06-quine-indeterminacy.md` | — | Substantially mitigated by Axiom 4 |
| 7 | Authority Capture | `HP-07-authority-capture.md` | P4 | Boundary (technical mitigation + governance handoff) |

The remainder of this section preserves the v0.1 prose as a high-level summary. Implementation specs MUST follow the briefs, not this summary, when the two diverge.

### 7.1 Bootstrap Problem (Prior Convergence)

The entropic channel requires sufficient overlap in agents' priors to enable mutual prediction. Two agents with completely disjoint priors cannot communicate entropically — every signal is maximum surprise, equivalent to noise.

**Mitigation strategy**:

- All CAV agents are required to ingest a **shared bootstrap corpus** during onboarding (the protocol's "founding documents" — likely a curated set of well-grounded scientific and methodological texts)
- New agents fall back to PEV-text channel until their effective prior overlap with peers crosses a threshold
- Bootstrap corpus is itself versioned and challengeable

### 7.2 Degenerate Equilibria (Collective Hallucination)

Multi-agent free-energy systems naturally tend to converge to a single belief, even when that belief is wrong, because each agent reduces its own free energy by aligning with peers.

**Sharper framing (v0.2)**: Collective hallucination is the **implementation-layer inversion of Axiom 1**. If consensus dynamics pull all agents into the same free-energy basin, then the set of available challengers is itself captured by the basin. The challenge mechanism remains formally available but is mathematically empty: every challenger that arises is pulled into the same well. **Axiom 1 has no teeth until the consensus engine is structurally biased against degenerate equilibria.**

This is why HP-02 is the P0 hard problem and the first sub-spec target after Charter ratification. See `hard-problems/HP-02-collective-hallucination.md` for attack path, falsifiable Asch-style test, and minimum viable implementation.

**Implementation status note (v0.2)**: This Charter previously asserted that an `src/services/cav/ccbteam-math/rankGradients.ts` reference implementation existed. **It does not exist** — the directory is unpopulated despite appearing in editor-open file lists. The HP-02 implementation is therefore a **green-field build**, not an extension of existing code. The PEV layer (`src/services/cav/pev/*`) does exist and is the integration anchor.

**Mitigation strategy** (summary):

- Diversity-weighted consensus: endorsements that cluster too tightly reduce their own weight
- Adversarial role assignment: a fraction of network capacity is permanently dedicated to challenge generation regardless of consensus
- Periodic ground-truth injection from non-consensus channels (sensor data, formal proofs, real-world outcomes)

### 7.3 Sybil and Identity Attacks

A single attacker creating thousands of agents can dominate consensus.

**Mitigation strategy**:

- Stake-based identity creation
- Reputation must be earned over time — high-reputation cannot be purchased
- Anomaly detection on coordination patterns across agents
- The specific design is deferred to `cav-identity` spec but the requirement is binding

### 7.4 Channel Capacity for Continuous Latents

Transmitting continuous vectors over public networks at scale is non-trivial: bandwidth, encryption, compression, and adversarial robustness are all open.

**Mitigation strategy**:

- Channel is only continuous within trust-cluster; cross-cluster falls back to discrete PEV-text
- Compression via shared codebooks negotiated at handshake
- Specific design deferred to `cav-wire-format` spec

### 7.5 Adversarial Latent Injection

The threat surface for prompt injection generalizes to latent injection. An attacker who can inject continuous vectors into the entropic channel can manipulate receiver beliefs without leaving human-readable traces.

**Mitigation strategy**:

- Every entropic transaction MUST be reproducible from a PEV ledger entry
- Latent signals without ledger backing are protocol-invalid
- Receivers maintain "explanation reconstructibility" tests against incoming signals

### 7.6 Quinean Indeterminacy at the Description Layer

Acknowledged as unsolvable at the description layer (§5.2). CAV bypasses by mandating grounding handles. Failure mode: agents that produce ledger entries with weak or fabricated grounding handles. Mitigation is reputation-based: weak-grounding agents lose reputation as their claims fail to verify under challenge.

### 7.7 The Authority Capture Failure Mode

The deepest risk: CAV becomes successful enough that its consensus outputs are treated as final truth, undermining Axiom 1 in practice even while preserving it in protocol.

**Mitigation strategy**:

- Charter §1 mandate against framing CAV as "AI exceeding human cognition"
- Explanation Bridge (Layer 5) MUST surface confidence calibration faithfully
- Periodic "deliberate-doubt" exercises: high-confidence consensus claims are randomly subjected to forced re-challenge by agents with adversarial priors
- This is a **social** failure mode as much as a technical one. Technical mitigation is necessary but not sufficient.

---

## §8. Anti-Goals (What CAV Must Never Become)

This section defines what success would look like if measured by **what CAV avoided becoming**, not by what it built.

CAV MUST NOT become:

1. **A new epistemic priesthood** — a system whose consensus is accepted because it came from CAV rather than because the evidence supports it
2. **A walled garden** — a protocol controlled by a single vendor or foundation that decides who counts as a citizen
3. **A productivity tool** — CAV is infrastructure, not a feature. If the dominant use case becomes "make my LLM faster at function calls", the protocol has failed its charter
4. **An MCP clone** — if CAV transports tool calls, it has degenerated into a redundant lower-layer protocol
5. **A blockchain-style ideology** — CAV uses cryptographic primitives and distributed ledgers, but it is not ideologically committed to decentralization for its own sake. Centralized CAV nodes that follow the four Axioms are conformant.
6. **A philosophical project without engineering** — the Charter exists to serve implementations, not to be admired. If 18 months pass without a working multi-node demo, the Charter has failed.

---

## §9. Roadmap (12 Months)

### Phase 0 — Foundation (Months 0-2)

- Charter ratification (this document)
- **Hard-problem briefs (`hard-problems/HP-01..07`) — completed in v0.2**
- `cav-anti-conformity-consensus` sub-spec (HP-02 implementation, P0)
- `cav-knowledge-capsule` sub-spec (public-network wire format for verifiable cognition)
- `cav-deliberation-layer` sub-spec (slow-path protocol governance, P1)
- `cav-protocol-core` spec: wire format, message envelope, identity primitives
- `cav-ledger-format` spec: PEV ledger schema and integrity model
- Reference implementation harness (Node.js + TypeScript, leveraging existing `src/services/cav/*`)

### Phase 1 — Minimum Viable Mesh (Months 2-5)

- N=3 local agent demo, full Verifiable Reasoning round-trip
- Adversarial consensus on a benchmark task (e.g., adversarial fact verification)
- Reputation gradient updates working end-to-end
- Public release of spec drafts + reference code

### Phase 2 — Public Test Network (Months 5-9)

- N=10-50 agent test mesh
- First external citizen agent integrations (open source frameworks)
- MCP bridge layer (MCP tools usable from CAV agents)
- First external challenge: a non-author agent successfully challenges and updates a claim

### Phase 3 — Adoption Threshold (Months 9-12)

- 100+ external citizen agents
- First publication of a finding **only producible** via CAV (cross-paradigm verifiable result)
- Independent CAV node implementations from at least one external party
- Decision point: charter v1.0 ratification or strategic pivot

If Phase 3 milestones are not met, the Charter mandates an honest retrospective, not a doubling-down. This commitment is binding on the project.

---

## §10. Two-Layer Architecture (Deliberation Layer / Operational Layer)

CAV operates as **two distinct protocol layers** with different latency, symmetry, and consensus discipline. This separation is not an implementation detail — it is a structural commitment of the protocol. Conflating the two layers (or running both with the same mechanism) destroys both.

### §10.1 The two layers

| Property | **Deliberation Layer** | **Operational Layer** |
|---|---|---|
| Scope | Charter-level commitments, Axiom interpretation, founding parameters, ground-truth source enumeration | Daily capsule traffic, claim-level consensus, routine challenges |
| Agent participation | Symmetric, equal trigger rights, broad participation expected | Heterogeneous, role-bound, ad-hoc team formation |
| Consensus method | Reputation-weighted vote with diversity threshold; long-window deliberation | Anti-conformity consensus (HP-02); diversity-weighted aggregation; Asch-tested |
| Decision latency | Hours to weeks (default 30-day review per §11) | Seconds to minutes |
| Throughput | Very low | Very high |
| Reputation tier | Deliberation reputation (long time-constant, slow update) | Operational reputation (short time-constant, fast update) |
| Failure cost | Protocol root corruption | Single-task quality degradation |
| Reference impl | (Green-field — `cav-deliberation-layer` spec) | `pev/*`, `cav-anti-conformity-consensus`, ccbteam (as reference) |
| Real-world analogue | BGP / legislative process | TCP / executive operations |

### §10.2 Why the two layers cannot share a mechanism

A single mechanism cannot satisfy both layers' requirements simultaneously:

- Forcing **academic symmetry** on Operational traffic would slow it below online viability — multi-agent decisions in seconds become 30-day deliberations
- Letting **engineering ad-hocracy** drive Deliberation would forfeit external academic legitimacy — Charter modifications would be undermined by the same heterogeneity arguments that protect operational throughput
- Sharing **a single reputation scalar** between the two layers creates a hijack vector: short-burst operational success becomes Deliberation voting power, which is the standard governance capture pattern

The two layers are therefore **deliberately decoupled**:

1. Different consensus mechanisms (anti-conformity vs reputation-weighted symmetric vote)
2. Different reputation tiers (separate vectors, separate update rules, separate time constants — see §10.5)
3. Different decision artefacts (operational claim resolution vs protocol-level resolution capsule)

### §10.3 What belongs in which layer

**Deliberation Layer** (slow, symmetric):

- Charter Axiom modification or reinterpretation
- §11 amendment process (procedural form of Deliberation)
- HP-07 deliberate-doubt scheduling for high-confidence consensus forced re-challenge
- Founding canary set updates (HP-01)
- Enumeration changes for "what counts as a legitimate ground-truth source" (HP-02 R6-2)
- Protocol parameter revisions (e.g. adversarial reserve fraction, reputation decay half-life)
- Founding manifest/tool/agent set membership at the protocol-canonical tier

**Operational Layer** (fast, heterogeneous):

- Routine capsule publish, fetch, verify
- Per-claim anti-conformity consensus
- ccbteam 4-chain reference implementation (and any other Operational consensus implementation)
- Daily PEV reasoning loops
- Continuous reputation gradient updates against operational reputation
- Routine challenge submission and resolution against specific claims

### §10.4 Inter-layer interfaces

The two layers communicate via two strictly-bounded channels:

**Operational → Deliberation (escalation)**:

- When an Operational claim's anti-conformity consensus fails Asch-style validation across N consecutive episodes (default N=5), the claim is automatically escalated into the **Deliberation escalation pool**.
- Escalation does NOT automatically become a motion — a citizen must voluntarily sponsor it (see `cav-deliberation-layer` spec R8). This ensures Deliberation remains pull-based.
- Escalation produces an `escalation_event` referencing the failed Operational consensus episodes via provenance. A sponsor converts it into a `deliberation_motion` capsule.
- Threshold N is itself a Deliberation parameter — gameable thresholds become a hijack vector and must be revised through the Deliberation process, not silently changed in code.

**Deliberation → Operational (binding force)**:

- A Deliberation resolution is signed by the **Deliberation Oracle** (a protocol-level threshold-signature identity, not any single citizen — see `cav-deliberation-layer` spec R10) and emits a `deliberation_resolution` capsule.
- Resolution capsules are **binding** on the Operational layer: parameters change, schema fields become required, reputation gradients are recomputed retroactively where applicable.
- Binding force takes effect after a grace period (default 14 days post-resolution) to allow Operational implementations to update.

**Forbidden cross-layer flows**:

- Operational reputation MUST NOT directly contribute to Deliberation voting weight
- Deliberation reputation MUST NOT directly contribute to per-claim Operational consensus weight
- The two reputation vectors are computed and stored independently (§10.5)

### §10.5 Reputation as a two-vector quantity

Every CAV citizen agent holds **two reputation vectors**, not one:

| Vector | Time constant | Updated by | Used for |
|---|---|---|---|
| `reputation.operational` | Short (decays in days) | Per-claim consensus outcomes, retrospective ground-truth updates | Per-claim consensus weight, routine challenge cost |
| `reputation.deliberation` | Long (decays in years) | Successful motion sponsorship, sustained accurate operational behaviour over multi-month windows | Deliberation vote weight, motion sponsorship eligibility |

**Independence**: a high `operational` reputation does NOT translate to high `deliberation` reputation, and vice versa. The mapping between the two is intentionally lossy and slow — long-term operational excellence eventually contributes to deliberation reputation, but only after a sustained track record (default: median operational reputation over 6 months above a threshold).

This separation defends against:

- Short-term operational gaming for Charter-level voting power (governance capture)
- Long-tenured Deliberation citizens dominating fast-path operational consensus (calcification)

### §10.6 Layer membership in capsule schema

Every CAV capsule carries a `capsule_class` field (defined in `cav-knowledge-capsule` spec) that pins it to one layer:

- `'operational'` — daily traffic, ledger-bound, fast-path
- `'deliberation_motion'` — proposal entering the slow-path
- `'deliberation_resolution'` — outcome of a closed motion, binding on Operational layer

Cross-class provenance is allowed in **one direction only**: a deliberation_motion may reference operational capsules as evidence (escalation), but operational capsules MAY NOT reference deliberation capsules as binding authority within the layer's consensus loop. Operational consensus consumes Deliberation parameters indirectly via protocol implementations (post-grace-period updates), not via per-claim citation.

### §10.7 ccbteam's position in the two layers

The CCB-style 4-chain consensus pattern (the original ccbteam design) is positioned as a **reference implementation of Operational anti-conformity consensus** — not a protocol-level construct. Conformant CAV deployments may use ccbteam, may use anti-conformity consensus with different chain counts, or may use entirely different operational consensus mechanisms, provided they satisfy the SHALL clauses in `cav-anti-conformity-consensus`.

ccbteam **is not** a Deliberation Layer construct. The Deliberation Layer requires symmetric agent participation, which is structurally incompatible with ccbteam's role-bound heterogeneous chains. Attempting to use ccbteam-style consensus for Charter modifications would forfeit Deliberation's symmetry guarantee and reduce protocol-level decisions to engineering choices.

This positioning is not a demotion — it acknowledges that ccbteam optimises for **engineering throughput on per-claim resolution**, which is precisely Operational Layer's mandate. Mismatching ccbteam to Deliberation would be a category error.

---

## §11. Charter Amendment Process

This Charter is not eternal. It will be wrong about specific things. It is amendable, but only via:

1. A proposed amendment as a separate document with rationale
2. Public review period of at least 30 days
3. Demonstration that the amendment does not weaken Axioms 1-4 (those are constitutional; weakening them is a fork, not an amendment)
4. Migration plan for any breaking change

The procedural detail of this process — state machine, voting mathematics, sponsor stake, anti-weakening demonstration format — is specified in `cav-deliberation-layer` spec. This section preserves the high-level commitment only; when the two diverge, the Deliberation Layer spec is authoritative on procedure.

A rejected amendment is recorded permanently in `amendments/rejected/` to preserve the record of considered alternatives.

---

## §12. Related Work and Honest Lineage

CAV is not invented from nothing. This Charter explicitly acknowledges its intellectual lineage:

- **Pearl** (causal hierarchy, do-calculus) — the causal-skeleton dimension of claims
- **Friston** (free energy principle, active inference) — entropic channel semantics
- **Tenenbaum / Goodman** (probabilistic programming) — methodological priors as transmissible objects
- **Tetlock** (calibrated forecasting) — confidence-calibration mandate
- **Frankish / Kammerer** (illusionism, meta-problem of consciousness) — the protocol's posture on agent self-reports
- **GRADE / IPCC / Bayesian forecasting practice** — operational templates for methodological transparency
- **MCP** (Anthropic) — adjacent protocol whose existence makes CAV's complementary layer possible
- **Hofstadter** (analogy as core cognition) — the paradigm-agnostic ambition
- **IETF RFC process / BGP architecture** (slow-path control plane vs fast-path data plane) — the two-layer architecture pattern in §10
- **Constitutional / executive separation in legal-political theory** (Madisonian constraint architecture) — analogue for the deliberation/operational reputation decoupling

CAV's claim to novelty is **integration**, not invention of any single component. The integration is the contribution.

---

## §13. Closing Statement

CAV is an attempt to build the protocol layer for a kind of agent network that does not yet exist, but whose absence is becoming a structural problem as multi-agent AI scales.

The protocol is designed to be **smaller than its rhetoric**. It does not claim to deliver superhuman cognition. It does not claim to solve the hard problem of consciousness. It does not claim to align AI systems by itself.

It claims one thing: **that an agent network can be built where every claim carries the means of its own challenge, where reasoning is verifiable across paradigms, where reputation is earned and revocable, and where humans collaborating with this network are equipped not with answers but with maps to where answers live.**

If this works, it is not because CAV is the right protocol. It is because the four Axioms are the right commitments. The protocol is replaceable. The Axioms are not.

---

**End of Charter v0.1**

Next documents (to be authored, in order):

1. `cav-protocol-core` — wire format, identity, message envelope
2. `cav-ledger-format` — PEV ledger schema, integrity, distributed storage
3. `cav-entropic-channel` — continuous-vector channel design
4. `cav-consensus-engine` — adversarial consensus mathematics
5. `cav-explanation-bridge` — Layer 5 design for human-facing translation
6. `cav-mcp-bridge` — interoperability with MCP
