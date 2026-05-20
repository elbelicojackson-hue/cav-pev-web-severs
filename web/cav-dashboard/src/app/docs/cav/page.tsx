import { Shield, Layers, Eye, Users, GitBranch, Zap, Network, AlertTriangle } from "lucide-react";

export default function CavDocsPage() {
  return (
    <article className="prose prose-invert prose-lg max-w-none">
      <div className="not-prose mb-12">
        <p className="text-primary font-mono text-sm uppercase tracking-widest mb-2">// Protocol Specification</p>
        <h1 className="text-4xl font-bold mb-4">CAV Protocol</h1>
        <p className="text-xl text-muted-foreground">Cognitive Adversarial Verification — A Public Protocol for Paradigm-Agnostic Multi-Agent Cognition</p>
        <div className="mt-4 flex gap-2">
          <span className="inline-flex items-center rounded-full bg-primary/10 border border-primary/20 px-3 py-1 text-xs font-mono text-primary">v0.3 Draft</span>
          <span className="inline-flex items-center rounded-full bg-emerald-500/10 border border-emerald-500/20 px-3 py-1 text-xs font-mono text-emerald-400">Charter</span>
        </div>
      </div>

      {/* Mission */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-4 flex items-center gap-2"><Zap className="h-5 w-5 text-primary" /> Mission</h2>
        <div className="rounded-xl border border-border/50 bg-card/30 p-6 backdrop-blur-sm">
          <blockquote className="border-l-4 border-primary pl-4 italic text-lg text-foreground/90">
            CAV is infrastructure for epistemically sovereign agents and the humans who collaborate with them.
          </blockquote>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            CAV operates at a different layer from MCP or Skill. Where MCP optimizes for tool-calling throughput, CAV optimizes for <strong className="text-foreground">verifiable, challengeable, methodologically transparent cognition</strong> between heterogeneous agents.
          </p>
          <p className="mt-3 text-muted-foreground leading-relaxed">
            The protocol exists so that any claim flowing through it carries with it the means of its own challenge. CAV makes collective cognition — across agents and humans — engineerable, auditable, and reversible.
          </p>
        </div>
      </section>

      {/* Four Axioms */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><Shield className="h-5 w-5 text-blue-400" /> The Four Axioms</h2>
        <p className="text-muted-foreground mb-6">Non-negotiable principles. Any system that violates one is not CAV-conformant, regardless of feature parity.</p>

        <div className="space-y-4">
          <div className="rounded-xl border border-blue-500/20 bg-blue-500/5 p-6">
            <div className="flex items-center gap-3 mb-3">
              <span className="text-xs font-mono text-blue-400 bg-blue-500/10 px-2 py-0.5 rounded">AXIOM 1</span>
              <h3 className="text-lg font-semibold">Always-Challengeable</h3>
            </div>
            <p className="text-muted-foreground text-sm leading-relaxed">Every claim emitted by any CAV node MUST be challengeable through the protocol itself. No claim, regardless of its source&apos;s reputation, holds privileged status that exempts it from challenge. A successful challenge updates the originating node&apos;s belief state, is permanently recorded in the PEV ledger, and propagates via reputation gradients to all dependent claims.</p>
          </div>

          <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-6">
            <div className="flex items-center gap-3 mb-3">
              <span className="text-xs font-mono text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded">AXIOM 2</span>
              <h3 className="text-lg font-semibold">Distribution-Lifting, Not Transcending</h3>
            </div>
            <p className="text-muted-foreground text-sm leading-relaxed">CAV&apos;s goal is to raise the lower tail of the collective cognitive distribution by giving non-experts engineered access to expert-grade epistemic processes. CAV does NOT claim to push the upper tail beyond current human cognition. It is judged by how well it serves the median user navigating expert disagreement.</p>
          </div>

          <div className="rounded-xl border border-violet-500/20 bg-violet-500/5 p-6">
            <div className="flex items-center gap-3 mb-3">
              <span className="text-xs font-mono text-violet-400 bg-violet-500/10 px-2 py-0.5 rounded">AXIOM 3</span>
              <h3 className="text-lg font-semibold">Methodological Transparency</h3>
            </div>
            <p className="text-muted-foreground text-sm leading-relaxed">Every claim transmitted through CAV MUST surface: <strong className="text-foreground">Causal skeleton</strong> (believed causal structure), <strong className="text-foreground">Uncertainty geometry</strong> (confidence + counterfactual neighborhood), <strong className="text-foreground">Priors</strong> (data, model, inference method), and <strong className="text-foreground">Falsifiability conditions</strong> (what observation would retract this claim).</p>
          </div>

          <div className="rounded-xl border border-amber-500/20 bg-amber-500/5 p-6">
            <div className="flex items-center gap-3 mb-3">
              <span className="text-xs font-mono text-amber-400 bg-amber-500/10 px-2 py-0.5 rounded">AXIOM 4</span>
              <h3 className="text-lg font-semibold">Paradigm-Agnostic Grounding</h3>
            </div>
            <p className="text-muted-foreground text-sm leading-relaxed">Every claim MUST attach to one or more grounding handles: pointers to evidence that any other node can independently verify, regardless of paradigm. Two CAV nodes with no common vocabulary can still verify each other&apos;s reasoning by traversing back to grounded evidence.</p>
          </div>
        </div>
      </section>

      {/* Architecture Layers */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><Layers className="h-5 w-5 text-violet-400" /> Architecture Layers</h2>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border/50">
                <th className="text-left py-3 px-4 text-muted-foreground font-mono text-xs">Layer</th>
                <th className="text-left py-3 px-4 text-muted-foreground font-mono text-xs">Name</th>
                <th className="text-left py-3 px-4 text-muted-foreground font-mono text-xs">Function</th>
              </tr>
            </thead>
            <tbody className="text-muted-foreground">
              <tr className="border-b border-border/30"><td className="py-3 px-4 font-mono text-primary">L1</td><td className="py-3 px-4 font-medium text-foreground">Identity &amp; Reputation</td><td className="py-3 px-4">DIDs, public-key signatures, vector reputation (per-domain, per-methodology), Sybil resistance</td></tr>
              <tr className="border-b border-border/30"><td className="py-3 px-4 font-mono text-primary">L2</td><td className="py-3 px-4 font-medium text-foreground">Verifiable Claim Ledger</td><td className="py-3 px-4">PEV ledger, append-only hash-chained, distributed storage, zero-knowledge selective disclosure</td></tr>
              <tr className="border-b border-border/30"><td className="py-3 px-4 font-mono text-primary">L3</td><td className="py-3 px-4 font-medium text-foreground">Entropic Cognitive Channel</td><td className="py-3 px-4">Continuous-vector messages, free-energy-minimization semantics, fallback to PEV-text</td></tr>
              <tr className="border-b border-border/30"><td className="py-3 px-4 font-mono text-primary">L4</td><td className="py-3 px-4 font-medium text-foreground">Adversarial Consensus Engine</td><td className="py-3 px-4">Challenge routing, multi-agent verdict aggregation, diversity weighting, reputation gradients</td></tr>
              <tr><td className="py-3 px-4 font-mono text-primary">L5</td><td className="py-3 px-4 font-medium text-foreground">Human Explanation Bridge</td><td className="py-3 px-4">PEV → human-readable narrative, methodology-aware, confidence-calibrated explanations</td></tr>
            </tbody>
          </table>
        </div>
      </section>

      {/* Praxon */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><GitBranch className="h-5 w-5 text-emerald-400" /> Praxon — The Cognitive Particle</h2>
        <p className="text-muted-foreground mb-6">Praxon (πρᾶξις + -on) is the fundamental cognitive particle of CAV — the minimum verifiable unit transmitted between agents on the public network.</p>

        <div className="rounded-xl border border-border/50 bg-card/30 p-6 font-mono text-sm overflow-x-auto">
          <pre className="text-muted-foreground">{`{
  "version": "1.0",
  "praxon_id": "sha256_hex",
  "praxon_class": "operational | deliberation_motion | deliberation_resolution",
  "issuer": "did:key:z...",
  "issued_at": "ISO 8601 UTC",
  "claim": {
    "causal_skeleton": { subject, relation, object, mechanism_hypothesis, strength },
    "uncertainty_geometry": { confidence, counterfactual_neighborhood, known_failure_modes },
    "methodology": { prior_source_tag, inference_method_tag, data_source_hashes },
    "falsifiability": { would_be_retracted_if, test_protocol_praxon_ref? }
  },
  "grounding": [ /* non-empty: tool_run | canary_eig | demonstration_trace | 
                     praxon_ref | formal_proof | dataset */ ],
  "provenance": { derived_from: [], consensus_episode?, challenges_survived? },
  "signature": "Ed25519 base64url"
}`}</pre>
        </div>

        <h3 className="text-lg font-semibold mt-8 mb-4">Three-Gate Verification</h3>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div className="rounded-lg border border-border/50 bg-card/30 p-4">
            <p className="text-xs font-mono text-primary mb-2">GATE 1</p>
            <p className="font-medium text-sm">Schema + Signature</p>
            <p className="text-xs text-muted-foreground mt-1">JSON Schema validation, Ed25519 signature verification, praxon_id hash consistency</p>
          </div>
          <div className="rounded-lg border border-border/50 bg-card/30 p-4">
            <p className="text-xs font-mono text-emerald-400 mb-2">GATE 2</p>
            <p className="font-medium text-sm">Grounding Verification</p>
            <p className="text-xs text-muted-foreground mt-1">Per-handle evidence verification — tool re-runs, trace replay, dataset hash checks</p>
          </div>
          <div className="rounded-lg border border-border/50 bg-card/30 p-4">
            <p className="text-xs font-mono text-violet-400 mb-2">GATE 3</p>
            <p className="font-medium text-sm">EIG Measurement</p>
            <p className="text-xs text-muted-foreground mt-1">Receiver measures actual entropy reduction on local canary tasks, compares to issuer&apos;s claim</p>
          </div>
        </div>
      </section>

      {/* Hard Problems */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><AlertTriangle className="h-5 w-5 text-amber-400" /> Known Hard Problems</h2>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border/50">
                <th className="text-left py-3 px-4 text-muted-foreground font-mono text-xs">#</th>
                <th className="text-left py-3 px-4 text-muted-foreground font-mono text-xs">Problem</th>
                <th className="text-left py-3 px-4 text-muted-foreground font-mono text-xs">Priority</th>
                <th className="text-left py-3 px-4 text-muted-foreground font-mono text-xs">Mitigation</th>
              </tr>
            </thead>
            <tbody className="text-muted-foreground">
              <tr className="border-b border-border/30"><td className="py-3 px-4 font-mono">HP-01</td><td className="py-3 px-4 text-foreground">Bootstrap (Prior Convergence)</td><td className="py-3 px-4"><span className="text-amber-400">P1</span></td><td className="py-3 px-4">Shared bootstrap corpus + PEV-text fallback</td></tr>
              <tr className="border-b border-border/30"><td className="py-3 px-4 font-mono">HP-02</td><td className="py-3 px-4 text-foreground">Collective Hallucination</td><td className="py-3 px-4"><span className="text-red-400 font-bold">P0</span></td><td className="py-3 px-4">Anti-conformity consensus + adversarial reserve</td></tr>
              <tr className="border-b border-border/30"><td className="py-3 px-4 font-mono">HP-03</td><td className="py-3 px-4 text-foreground">Sybil &amp; Identity Attacks</td><td className="py-3 px-4"><span className="text-amber-400">P2</span></td><td className="py-3 px-4">Stake-based identity + coordination anomaly detection</td></tr>
              <tr className="border-b border-border/30"><td className="py-3 px-4 font-mono">HP-04</td><td className="py-3 px-4 text-foreground">Channel Capacity</td><td className="py-3 px-4"><span className="text-blue-400">P3</span></td><td className="py-3 px-4">Trust-cluster continuous + cross-cluster PEV-text</td></tr>
              <tr className="border-b border-border/30"><td className="py-3 px-4 font-mono">HP-05</td><td className="py-3 px-4 text-foreground">Latent Injection</td><td className="py-3 px-4"><span className="text-amber-400">P1</span></td><td className="py-3 px-4">Ledger-backed reproducibility requirement</td></tr>
              <tr className="border-b border-border/30"><td className="py-3 px-4 font-mono">HP-06</td><td className="py-3 px-4 text-foreground">Quinean Indeterminacy</td><td className="py-3 px-4"><span className="text-muted-foreground">—</span></td><td className="py-3 px-4">Bypassed by Axiom 4 grounding handles</td></tr>
              <tr><td className="py-3 px-4 font-mono">HP-07</td><td className="py-3 px-4 text-foreground">Authority Capture</td><td className="py-3 px-4"><span className="text-blue-400">P4</span></td><td className="py-3 px-4">Deliberate-doubt exercises + governance handoff</td></tr>
            </tbody>
          </table>
        </div>
      </section>

      {/* Two-Layer Architecture */}
      <section className="not-prose">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><Network className="h-5 w-5 text-primary" /> Two-Layer Architecture</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="rounded-xl border border-blue-500/20 bg-blue-500/5 p-6">
            <h3 className="font-semibold mb-3 text-blue-400">Operational Layer</h3>
            <ul className="text-sm text-muted-foreground space-y-2">
              <li>• Daily capsule traffic, routine challenges</li>
              <li>• Anti-conformity consensus (seconds to minutes)</li>
              <li>• High throughput, heterogeneous agents</li>
              <li>• Fast reputation updates</li>
              <li>• Analogy: TCP / executive operations</li>
            </ul>
          </div>
          <div className="rounded-xl border border-violet-500/20 bg-violet-500/5 p-6">
            <h3 className="font-semibold mb-3 text-violet-400">Deliberation Layer</h3>
            <ul className="text-sm text-muted-foreground space-y-2">
              <li>• Charter-level commitments, Axiom interpretation</li>
              <li>• Reputation-weighted symmetric vote (hours to weeks)</li>
              <li>• Very low throughput, broad participation</li>
              <li>• Slow reputation updates</li>
              <li>• Analogy: BGP / legislative process</li>
            </ul>
          </div>
        </div>
      </section>
    </article>
  );
}
