import { Cpu, GitBranch, Zap, Layers, Target, BarChart3, Network, Shield } from "lucide-react";

export default function PevDocsPage() {
  return (
    <article className="prose prose-invert prose-lg max-w-none">
      <div className="not-prose mb-12">
        <p className="text-primary font-mono text-sm uppercase tracking-widest mb-2">// Algorithm Specification</p>
        <h1 className="text-4xl font-bold mb-4">PEV Algorithm</h1>
        <p className="text-xl text-muted-foreground">Plan-Execute-Verify — Hypothesis-Driven Execution Loop with Information-Theoretic Scheduling</p>
        <div className="mt-4 flex gap-2">
          <span className="inline-flex items-center rounded-full bg-primary/10 border border-primary/20 px-3 py-1 text-xs font-mono text-primary">v1.0</span>
          <span className="inline-flex items-center rounded-full bg-violet-500/10 border border-violet-500/20 px-3 py-1 text-xs font-mono text-violet-400">Reference Impl</span>
        </div>
      </div>

      {/* Overview */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-4 flex items-center gap-2"><Cpu className="h-5 w-5 text-primary" /> Overview</h2>
        <div className="rounded-xl border border-border/50 bg-card/30 p-6">
          <p className="text-muted-foreground leading-relaxed">
            PEV is a hypothesis-driven execution loop that externalises reasoning into a typed <strong className="text-foreground">Hypothesis Bank + Evidence Ledger</strong>, driven by a deterministic scheduler rather than model memory. Agents propose hypotheses, the runner dispatches canonical tool plans, and a regex verdict engine auto-judges results — all without LLM-in-the-loop judgement.
          </p>
          <p className="text-muted-foreground leading-relaxed mt-3">
            The core insight: the most valuable experiment is not the one targeting the highest-confidence hypothesis, but the one whose result — regardless of outcome — <strong className="text-foreground">maximally reduces the total uncertainty</strong> (entropy) of the hypothesis set.
          </p>
        </div>
      </section>

      {/* Architecture Diagram */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><Layers className="h-5 w-5 text-violet-400" /> Architecture</h2>
        <div className="rounded-xl border border-border/50 bg-background/80 p-6 font-mono text-xs overflow-x-auto">
          <pre className="text-muted-foreground">{`┌─────────────────────────────────────────────────────────────────┐
│  PevRunner — async generator main loop                          │
│    round N:                                                     │
│      1. schedule(ledger, agents, round)     ← EIG-optimal       │
│      2. propagate(ledger, agents, round)    ← cross-agent inbox │
│      3. buildPrompt per agent → dispatchArena                   │
│      4. parsePevOutput (3-layer fallback)                       │
│      5. applyHypothesisUpdate (reducer)                         │
│      6. execute tool_call → judgeVerdict → appendEvidence       │
│      7. persistence (writePevEvalLog)                           │
│      8. stop-condition check                                    │
├─────────────────────────────────────────────────────────────────┤
│  Pure-function leaves (no I/O, no state):                       │
│    protocol.ts    — zod schemas + types                         │
│    validator.ts   — cross-validation (referential integrity)    │
│    ledger.ts      — immutable reducer (hypothesis + evidence)   │
│    scheduler.ts   — EIG-optimal per-agent directive             │
│    eigEngine.ts   — Shannon entropy + Bayesian update           │
│    causalEngine.ts— Pearl do-calculus intervention              │
│    propagator.ts  — cross-agent inbox builder                   │
│    verdict.ts     — regex verdict engine                        │
└─────────────────────────────────────────────────────────────────┘`}</pre>
        </div>
      </section>

      {/* EIG Engine */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><BarChart3 className="h-5 w-5 text-emerald-400" /> EIG Engine — Expected Information Gain</h2>
        <p className="text-muted-foreground mb-6">The EIG engine computes the optimal next experiment using Shannon entropy and Bayesian posterior updates.</p>

        <div className="rounded-xl border border-border/50 bg-card/30 p-6 mb-6">
          <h3 className="font-semibold mb-4 text-sm uppercase tracking-wider text-muted-foreground">Mathematical Basis</h3>
          <div className="space-y-4 font-mono text-sm">
            <div>
              <p className="text-muted-foreground mb-1">Binary Entropy:</p>
              <p className="text-foreground">H(p) = -p·log₂(p) - (1-p)·log₂(1-p)</p>
            </div>
            <div>
              <p className="text-muted-foreground mb-1">Expected Information Gain:</p>
              <p className="text-foreground">EIG = H(prior) - E[H(posterior)]</p>
            </div>
            <div>
              <p className="text-muted-foreground mb-1">Expected Posterior Entropy:</p>
              <p className="text-foreground">E[H(post)] = α·H(p+Δ) + β·H(p-Δ) + γ·H(p)</p>
            </div>
            <div className="text-xs text-muted-foreground pt-2 border-t border-border/30">
              <p>α = P(confirm | plan), β = P(falsify | plan), γ = P(inconclusive | plan)</p>
              <p>Δ = DELTA_SCALE × (1-p) for confirm, DELTA_SCALE × p for falsify</p>
            </div>
          </div>
        </div>

        <div className="rounded-xl border border-border/50 bg-card/30 p-6">
          <h3 className="font-semibold mb-4 text-sm uppercase tracking-wider text-muted-foreground">Scheduling Algorithm</h3>
          <ol className="text-sm text-muted-foreground space-y-2 list-decimal list-inside">
            <li>For each agent, collect active hypotheses (status ∈ {`{open, evidence}`})</li>
            <li>Filter to those NOT touched this round (lastTouchedRound &lt; currentRound)</li>
            <li>For each candidate H × untested plan, compute EIG + exploration bonus</li>
            <li>Rank by total score (EIG + exploration weight × novelty)</li>
            <li>Assign top-ranked (H, plan) pair as the agent&apos;s directive</li>
            <li>Deterministic tie-break: lowest hypothesis ID wins (lexicographic)</li>
          </ol>
        </div>
      </section>

      {/* Causal Engine */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><Target className="h-5 w-5 text-red-400" /> Causal Engine — Pearl do-Calculus</h2>
        <p className="text-muted-foreground mb-6">Goes beyond correlation to establish TRUE causation via intervention. Implements Pearl&apos;s Causal Hierarchy Level 2.</p>

        <div className="rounded-xl border border-border/50 bg-card/30 p-6 mb-6">
          <h3 className="font-semibold mb-4 text-sm uppercase tracking-wider text-muted-foreground">Causal Verdict Truth Table</h3>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border/50">
                  <th className="text-left py-2 px-3 text-muted-foreground font-mono text-xs">Original</th>
                  <th className="text-left py-2 px-3 text-muted-foreground font-mono text-xs">Intervention</th>
                  <th className="text-left py-2 px-3 text-muted-foreground font-mono text-xs">Verdict</th>
                  <th className="text-left py-2 px-3 text-muted-foreground font-mono text-xs">Strength</th>
                </tr>
              </thead>
              <tbody className="text-muted-foreground">
                <tr className="border-b border-border/30"><td className="py-2 px-3 text-emerald-400">confirms</td><td className="py-2 px-3 text-red-400">falsifies</td><td className="py-2 px-3 text-foreground font-medium">causal-confirm</td><td className="py-2 px-3">1.0</td></tr>
                <tr className="border-b border-border/30"><td className="py-2 px-3 text-emerald-400">confirms</td><td className="py-2 px-3 text-amber-400">inconclusive</td><td className="py-2 px-3 text-foreground font-medium">causal-confirm</td><td className="py-2 px-3">0.7</td></tr>
                <tr className="border-b border-border/30"><td className="py-2 px-3 text-emerald-400">confirms</td><td className="py-2 px-3 text-emerald-400">confirms</td><td className="py-2 px-3 text-foreground font-medium">correlation-only</td><td className="py-2 px-3">0.0</td></tr>
                <tr className="border-b border-border/30"><td className="py-2 px-3 text-red-400">falsifies</td><td className="py-2 px-3 text-muted-foreground">*</td><td className="py-2 px-3 text-foreground font-medium">causal-falsify</td><td className="py-2 px-3">1.0</td></tr>
                <tr><td className="py-2 px-3 text-amber-400">inconclusive</td><td className="py-2 px-3 text-muted-foreground">*</td><td className="py-2 px-3 text-foreground font-medium">inconclusive</td><td className="py-2 px-3">0.0</td></tr>
              </tbody>
            </table>
          </div>
        </div>

        <div className="rounded-xl border border-border/50 bg-card/30 p-6">
          <h3 className="font-semibold mb-3 text-sm uppercase tracking-wider text-muted-foreground">Key Insight</h3>
          <p className="text-muted-foreground text-sm leading-relaxed">
            If removing the suspected cause (intervention) also removes the effect (confirms → falsifies), then the relationship is <strong className="text-foreground">CAUSAL</strong>, not merely correlational. This goes beyond what DNNs can achieve because: (1) DNNs cannot perform interventions, (2) DNNs cannot distinguish correlation from causation, (3) this implements Pearl&apos;s Level 2 (intervention) while DNNs operate at Level 1 (observation).
          </p>
        </div>
      </section>

      {/* Ledger */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><GitBranch className="h-5 w-5 text-blue-400" /> Hypothesis Ledger</h2>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
          <div className="rounded-xl border border-border/50 bg-card/30 p-6">
            <h3 className="font-semibold mb-3 text-sm">Hypothesis Lifecycle (5 states)</h3>
            <div className="space-y-2 text-sm">
              <div className="flex items-center gap-2"><span className="w-2 h-2 rounded-full bg-blue-400" /><span className="text-foreground font-mono">open</span><span className="text-muted-foreground">— initial state on create</span></div>
              <div className="flex items-center gap-2"><span className="w-2 h-2 rounded-full bg-emerald-400" /><span className="text-foreground font-mono">evidence</span><span className="text-muted-foreground">— promoted (confirmed by tool)</span></div>
              <div className="flex items-center gap-2"><span className="w-2 h-2 rounded-full bg-red-400" /><span className="text-foreground font-mono">falsified</span><span className="text-muted-foreground">— disproven by counter-evidence</span></div>
              <div className="flex items-center gap-2"><span className="w-2 h-2 rounded-full bg-amber-400" /><span className="text-foreground font-mono">mutated</span><span className="text-muted-foreground">— refined into new hypothesis</span></div>
              <div className="flex items-center gap-2"><span className="w-2 h-2 rounded-full bg-gray-400" /><span className="text-foreground font-mono">stale</span><span className="text-muted-foreground">— ancestor falsified (cascade)</span></div>
            </div>
          </div>
          <div className="rounded-xl border border-border/50 bg-card/30 p-6">
            <h3 className="font-semibold mb-3 text-sm">Hypothesis Kinds (8)</h3>
            <div className="space-y-2 text-sm text-muted-foreground">
              <p><span className="font-mono text-foreground">file-class</span> — PE32+, ELF, Mach-O</p>
              <p><span className="font-mono text-foreground">packer</span> — UPX, VMProtect, Themida</p>
              <p><span className="font-mono text-foreground">compiler</span> — .NET, GCC, MSVC</p>
              <p><span className="font-mono text-foreground">family</span> — Emotet, Cobalt Strike</p>
              <p><span className="font-mono text-foreground">algorithm</span> — AES-256-CBC, RC4</p>
              <p><span className="font-mono text-foreground">anti-analysis</span> — TLS callback, VM detect</p>
              <p><span className="font-mono text-foreground">capability</span> — Network C2, keylogger</p>
              <p><span className="font-mono text-foreground">protocol</span> — gRPC, HTTPS, MQTT</p>
            </div>
          </div>
        </div>

        <div className="rounded-xl border border-border/50 bg-card/30 p-6">
          <h3 className="font-semibold mb-3 text-sm uppercase tracking-wider text-muted-foreground">Immutability Invariants</h3>
          <ul className="text-sm text-muted-foreground space-y-2">
            <li>• Every reducer is a <strong className="text-foreground">pure function</strong>: same inputs → same outputs, no I/O</li>
            <li>• Every reducer returns a <strong className="text-foreground">new</strong> SharedLedger object (never mutates in place)</li>
            <li>• lastEvidenceId is monotonically non-decreasing (agents do NOT mint evidence IDs)</li>
            <li>• Invalid ops are silently no-ops (defence-in-depth; the runner stays alive)</li>
            <li>• Stale cascade: falsifying a parent automatically marks all descendants as stale</li>
          </ul>
        </div>
      </section>

      {/* Propagator */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><Network className="h-5 w-5 text-amber-400" /> Cross-Agent Propagator</h2>
        <p className="text-muted-foreground mb-6">Builds per-agent inboxes for the upcoming round. Three information streams flow through:</p>

        <div className="space-y-4">
          <div className="rounded-lg border border-border/50 bg-card/30 p-5">
            <h3 className="font-semibold text-sm mb-2 text-blue-400">1. Lateral Evidence Push</h3>
            <p className="text-sm text-muted-foreground">Evidence from round N-1 is pushed to peer agents whose active hypothesis kinds match (same kind or DERIVE_RULES child). Self-feedback is prevented (R9-8).</p>
          </div>
          <div className="rounded-lg border border-border/50 bg-card/30 p-5">
            <h3 className="font-semibold text-sm mb-2 text-emerald-400">2. Vertical Sub-Hypothesis Hints</h3>
            <p className="text-sm text-muted-foreground">Promoted hypotheses generate synthetic child hints via DERIVE_RULES (e.g., confirmed packer → suggest compiler + capability). Owner decides whether to create.</p>
          </div>
          <div className="rounded-lg border border-border/50 bg-card/30 p-5">
            <h3 className="font-semibold text-sm mb-2 text-amber-400">3. Stale Notices</h3>
            <p className="text-sm text-muted-foreground">Hypotheses with status &apos;stale&apos; push their ID into the owner&apos;s inbox so the agent stops reasoning about dead branches.</p>
          </div>
        </div>

        <div className="mt-6 rounded-xl border border-border/50 bg-card/30 p-6">
          <h3 className="font-semibold mb-3 text-sm uppercase tracking-wider text-muted-foreground">DERIVE_RULES Table</h3>
          <div className="font-mono text-xs text-muted-foreground space-y-1">
            <p><span className="text-foreground">file-class</span> → packer, compiler, capability</p>
            <p><span className="text-foreground">packer</span> → compiler, capability</p>
            <p><span className="text-foreground">compiler</span> → algorithm, capability</p>
            <p><span className="text-foreground">family</span> → capability, protocol</p>
            <p><span className="text-foreground">capability</span> → protocol</p>
            <p><span className="text-foreground">protocol</span> → capability</p>
            <p><span className="text-foreground">algorithm</span> → (terminal)</p>
            <p><span className="text-foreground">anti-analysis</span> → (terminal)</p>
          </div>
        </div>
      </section>

      {/* Stop Conditions */}
      <section className="not-prose">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><Shield className="h-5 w-5 text-red-400" /> Stop Conditions</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="rounded-lg border border-border/50 bg-card/30 p-4">
            <p className="font-mono text-xs text-emerald-400 mb-1">all-resolved</p>
            <p className="text-sm text-muted-foreground">No open hypotheses remain</p>
          </div>
          <div className="rounded-lg border border-border/50 bg-card/30 p-4">
            <p className="font-mono text-xs text-amber-400 mb-1">budget-cap-hit</p>
            <p className="text-sm text-muted-foreground">maxRounds / maxToolCalls / maxTokens / maxWallClock exceeded</p>
          </div>
          <div className="rounded-lg border border-border/50 bg-card/30 p-4">
            <p className="font-mono text-xs text-red-400 mb-1">stall-guard-hit</p>
            <p className="text-sm text-muted-foreground">2 consecutive rounds where all agents observe-only</p>
          </div>
          <div className="rounded-lg border border-border/50 bg-card/30 p-4">
            <p className="font-mono text-xs text-violet-400 mb-1">parse-storm</p>
            <p className="text-sm text-muted-foreground">≥50% of agents fail to parse in a single round</p>
          </div>
        </div>
      </section>
    </article>
  );
}
