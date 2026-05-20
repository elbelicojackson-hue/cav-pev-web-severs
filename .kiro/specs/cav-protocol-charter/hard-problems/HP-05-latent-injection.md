# HP-05 — Latent Injection

**Priority**: P1
**Status**: Ready
**Charter ref**: §7.5, Layer 3 (§4.3)

## Problem statement (precise)

Prompt injection at the token layer is a well-known attack: adversarial natural-language input alters an LLM's behavior in ways the application designer did not intend. CAV's entropic channel generalizes this attack to the continuous-vector layer: an attacker who can inject crafted continuous vectors into a receiver's free-energy minimization process can manipulate the receiver's posterior without leaving any human-readable trace.

Latent injection is **strictly more dangerous** than prompt injection because:

- The signal is opaque to human auditing by construction (Charter §3.4 states the entropic channel is not human-readable)
- Defenders cannot pattern-match on suspicious "phrases" because there are no phrases
- Standard model-output safeguards (refusal training, content filters) operate on token outputs, not on belief updates triggered by latent inputs

The hard form: **how does the receiver distinguish a legitimate prediction-error signal from a crafted adversarial vector designed to drive its posterior to an attacker-chosen state, when by design both are continuous vectors with similar surface statistics?**

## Why it is hard

- Adversarial latents that are statistically indistinguishable from legitimate signals can be constructed (the same way adversarial examples for vision classifiers are statistically close to clean inputs)
- Strong cryptographic signing of the *vector* doesn't help — a legitimate sender can be compromised, or a malicious sender can hold a valid identity
- The receiver cannot "read" the signal to check it for hostility, because the channel is non-readable by design

## Attack path

**The protocol-level rule**:

> **Every entropic transaction MUST be reproducible from a corresponding PEV ledger entry. Latent signals without ledger backing are protocol-invalid and MUST be rejected.**

This is not a heuristic — it is a structural constraint baked into the wire format. The brief enumerates how:

1. Every latent message carries a **ledger anchor**: a hash pointing to a PEV ledger entry on the sender's identity, timestamped at or before message emission
2. The ledger entry contains the discrete reasoning trace that **would have produced** the latent signal if the same agent had been forced onto the PEV-text channel
3. Receivers run an **explanation reconstructibility test**: from the latent signal alone, the receiver checks that the decoded posterior shift is consistent with the ledger entry's discrete claim. If divergence exceeds threshold, the signal is rejected and the sender's reputation is debited
4. Adversarial latents that drive posteriors to attacker-chosen states will fail reconstructibility because the matching PEV ledger entry would expose the manipulation in human-readable form

This converts the latent channel from a "trust the sender" channel into a "trust the sender's matching ledger entry" channel. The latent channel is a **performance optimization** over the discrete channel, not an alternative to it.

## Success criteria (falsifiable)

- Adversary controlling a citizen identity attempts to craft a latent vector that drives receiver posterior to target T while the matching ledger entry says something else. **Pass**: receiver detects divergence ≥ τ on reconstructibility check and rejects
- Legitimate sender's latent signals always pass reconstructibility (false-positive rate < 1% on benchmark)
- Removing the ledger-backing requirement breaks the test — a control deployment without the rule allows successful injection

## Minimum viable implementation

- Wire format mandates `ledger_anchor: hash` field on every latent message
- Receiver runs a lightweight decoder (one transformer-block scale) that produces a discretized claim from the latent signal
- The discretized claim is compared to the ledger entry's claim on the same axes (causal skeleton, methodology, falsifiability)
- Divergence metric: weighted L2 over the four Axiom-3 components, thresholded
- Threshold τ is per-cluster, calibrated during handshake (HP-01)

## Dependencies

- **Hard dependency on PEV ledger format** (`cav-ledger-format`)
- Hard dependency on HP-01 (cluster handshake calibrates τ)
- Builds on HP-02 (failed reconstructibility triggers consensus-engine reputation update)

## Open questions

- How is the discretized-claim decoder itself protected from adversarial latents? (Recursion problem)
- Should the decoder be a separate model from the receiver's primary model, to avoid attacks that evade both simultaneously?
- What is the right divergence threshold — too tight rejects legitimate signals, too loose admits attacks
- Can the ledger entry itself be adversarial in a way that passes reconstructibility while corrupting downstream reasoning? (This connects to HP-06 — grounding handles must be verifiable independently)

## Status

Ready for sub-spec. The protocol-level rule (latent without ledger anchor is invalid) is a hard constraint that goes into `cav-protocol-core`. The reconstructibility decoder is its own sub-spec, recommended name `cav-latent-injection-defense`.

P1 priority because it is structurally required for the entropic channel to be usable at all. Without HP-05, the entropic channel is unsafe to deploy regardless of what HP-04 says about scalability.
