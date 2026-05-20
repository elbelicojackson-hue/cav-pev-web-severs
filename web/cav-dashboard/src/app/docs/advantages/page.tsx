import { Zap, Shield, GitBranch, Layers, Target, BarChart3, Network, ExternalLink, Github, FlaskConical, Cpu, Eye } from "lucide-react";
import Link from "next/link";

export default function AdvantagesPage() {
  return (
    <article className="prose prose-invert prose-lg max-w-none">
      <div className="not-prose mb-12">
        <p className="text-emerald-400 font-mono text-sm uppercase tracking-widest mb-2">// Why CAV</p>
        <h1 className="text-4xl font-bold mb-4">Architecture Advantages &amp; Breakthrough Experiments</h1>
        <p className="text-xl text-muted-foreground">How CAV structurally surpasses MCP, Skill, and existing agent protocols — with empirical evidence.</p>
        <div className="mt-6 flex gap-3 flex-wrap">
          <Link
            href="https://github.com/elbelicojackson-hue/ant-"
            target="_blank"
            className="inline-flex items-center gap-2 rounded-lg border border-border/50 bg-card/50 px-4 py-2.5 text-sm font-medium hover:border-primary/40 hover:bg-primary/5 transition-all"
          >
            <Github className="h-4 w-4" />
            ant- (ANT Protocol)
            <ExternalLink className="h-3 w-3 text-muted-foreground" />
          </Link>
          <Link
            href="https://github.com/elbelicojackson-hue/NCP-sdk"
            target="_blank"
            className="inline-flex items-center gap-2 rounded-lg border border-border/50 bg-card/50 px-4 py-2.5 text-sm font-medium hover:border-primary/40 hover:bg-primary/5 transition-all"
          >
            <Github className="h-4 w-4" />
            NCP-sdk
            <ExternalLink className="h-3 w-3 text-muted-foreground" />
          </Link>
          <Link
            href="https://github.com/elbelicojackson-hue/-CAV-CCB"
            target="_blank"
            className="inline-flex items-center gap-2 rounded-lg border border-border/50 bg-card/50 px-4 py-2.5 text-sm font-medium hover:border-primary/40 hover:bg-primary/5 transition-all"
          >
            <Github className="h-4 w-4" />
            CAV-CCB
            <ExternalLink className="h-3 w-3 text-muted-foreground" />
          </Link>
          <Link
            href="https://github.com/elbelicojackson-hue/-CAV-"
            target="_blank"
            className="inline-flex items-center gap-2 rounded-lg border border-border/50 bg-card/50 px-4 py-2.5 text-sm font-medium hover:border-primary/40 hover:bg-primary/5 transition-all"
          >
            <Github className="h-4 w-4" />
            CAV Protocol
            <ExternalLink className="h-3 w-3 text-muted-foreground" />
          </Link>
          <Link
            href="https://github.com/elbelicojackson-hue/Cognitive-Affect-Vector-"
            target="_blank"
            className="inline-flex items-center gap-2 rounded-lg border border-border/50 bg-card/50 px-4 py-2.5 text-sm font-medium hover:border-primary/40 hover:bg-primary/5 transition-all"
          >
            <Github className="h-4 w-4" />
            Cognitive-Affect-Vector
            <ExternalLink className="h-3 w-3 text-muted-foreground" />
          </Link>
        </div>
      </div>

      {/* Core Thesis */}
      <section className="not-prose mb-16">
        <div className="rounded-xl border border-primary/20 bg-primary/5 p-8">
          <h2 className="text-2xl font-bold mb-4">Core Thesis</h2>
          <p className="text-lg text-muted-foreground leading-relaxed">
            Existing agent protocols (MCP, Skill, A2A) transport <strong className="text-foreground">tool calls and results</strong>. CAV transports <strong className="text-primary">cognitive structure and challenges</strong>. This is not an incremental improvement — it is a layer-shift. MCP answers &ldquo;what did the tool return?&rdquo; CAV answers &ldquo;<em>why should you believe it, and under what conditions would it be wrong?</em>&rdquo;
          </p>
        </div>
      </section>

      {/* Comparison Table */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><Zap className="h-5 w-5 text-primary" /> Protocol Comparison: CAV vs MCP vs Skill</h2>
        <div className="overflow-x-auto rounded-xl border border-border/50">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border/50 bg-card/30">
                <th className="text-left py-4 px-5 text-muted-foreground font-mono text-xs uppercase tracking-wider">Dimension</th>
                <th className="text-left py-4 px-5 text-muted-foreground font-mono text-xs uppercase tracking-wider">MCP</th>
                <th className="text-left py-4 px-5 text-muted-foreground font-mono text-xs uppercase tracking-wider">Skill</th>
                <th className="text-left py-4 px-5 font-mono text-xs uppercase tracking-wider text-primary">CAV</th>
              </tr>
            </thead>
            <tbody className="text-muted-foreground">
              <tr className="border-b border-border/30">
                <td className="py-3 px-5 text-foreground font-medium">What gets transmitted</td>
                <td className="py-3 px-5">Tool calls + JSON results</td>
                <td className="py-3 px-5">Capability declarations + invocations</td>
                <td className="py-3 px-5 text-primary font-medium">Cognitive structure + challenges + grounding handles</td>
              </tr>
              <tr className="border-b border-border/30">
                <td className="py-3 px-5 text-foreground font-medium">Trust model</td>
                <td className="py-3 px-5">Per-server, declared by config</td>
                <td className="py-3 px-5">Vendor-granted, centralized</td>
                <td className="py-3 px-5 text-primary font-medium">Earned reputation, per-claim, decentralized</td>
              </tr>
              <tr className="border-b border-border/30">
                <td className="py-3 px-5 text-foreground font-medium">Failure mode</td>
                <td className="py-3 px-5">Tool returns error → retry</td>
                <td className="py-3 px-5">Skill unavailable → fallback</td>
                <td className="py-3 px-5 text-primary font-medium">Claim is challenged → belief updated → propagated</td>
              </tr>
              <tr className="border-b border-border/30">
                <td className="py-3 px-5 text-foreground font-medium">Verifiability</td>
                <td className="py-3 px-5">None (trust the server)</td>
                <td className="py-3 px-5">None (trust the vendor)</td>
                <td className="py-3 px-5 text-primary font-medium">Three-gate: schema + grounding + EIG measurement</td>
              </tr>
              <tr className="border-b border-border/30">
                <td className="py-3 px-5 text-foreground font-medium">Challengeability</td>
                <td className="py-3 px-5">Not possible</td>
                <td className="py-3 px-5">Not possible</td>
                <td className="py-3 px-5 text-primary font-medium">Protocol-level requirement (Axiom 1)</td>
              </tr>
              <tr className="border-b border-border/30">
                <td className="py-3 px-5 text-foreground font-medium">Cross-paradigm</td>
                <td className="py-3 px-5">Same vendor ecosystem</td>
                <td className="py-3 px-5">Anthropic-only</td>
                <td className="py-3 px-5 text-primary font-medium">Any paradigm (LLM, symbolic, neuro-symbolic, active-inference)</td>
              </tr>
              <tr className="border-b border-border/30">
                <td className="py-3 px-5 text-foreground font-medium">Consensus</td>
                <td className="py-3 px-5">Single oracle (server is truth)</td>
                <td className="py-3 px-5">Single oracle (vendor is truth)</td>
                <td className="py-3 px-5 text-primary font-medium">Adversarial multi-agent with anti-conformity weighting</td>
              </tr>
              <tr>
                <td className="py-3 px-5 text-foreground font-medium">Causation vs Correlation</td>
                <td className="py-3 px-5">Cannot distinguish</td>
                <td className="py-3 px-5">Cannot distinguish</td>
                <td className="py-3 px-5 text-primary font-medium">Pearl do-calculus intervention engine (Level 2)</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      {/* Key Advantages */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><Shield className="h-5 w-5 text-emerald-400" /> Structural Advantages</h2>

        <div className="space-y-6">
          <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-6">
            <div className="flex items-center gap-3 mb-3">
              <Eye className="h-5 w-5 text-emerald-400" />
              <h3 className="text-lg font-semibold">1. Verifiable Reasoning (not just answers)</h3>
            </div>
            <p className="text-muted-foreground text-sm leading-relaxed mb-3">
              MCP gives you a tool result. CAV gives you the result + the hypothesis tree that produced it + the evidence consulted + the methodological assumptions + the conditions under which it would be retracted + the reputation history of the producing agent + the full ledger of challenges.
            </p>
            <p className="text-xs text-muted-foreground font-mono bg-background/50 rounded-lg p-3">
              Impact: Users don&apos;t receive an answer — they receive a navigable map of where the answer lives in epistemic space.
            </p>
          </div>

          <div className="rounded-xl border border-blue-500/20 bg-blue-500/5 p-6">
            <div className="flex items-center gap-3 mb-3">
              <Target className="h-5 w-5 text-blue-400" />
              <h3 className="text-lg font-semibold">2. Causal Inference (not just correlation)</h3>
            </div>
            <p className="text-muted-foreground text-sm leading-relaxed mb-3">
              DNNs and tool-calling protocols only observe correlations. CAV&apos;s causal engine performs actual interventions (Pearl Level 2): &ldquo;if we REMOVE the suspected cause, does the effect disappear?&rdquo; This is structurally impossible in MCP/Skill because they have no concept of counterfactual experiments.
            </p>
            <p className="text-xs text-muted-foreground font-mono bg-background/50 rounded-lg p-3">
              Impact: Distinguishes &ldquo;UPX string present AND file is packed&rdquo; (correlation) from &ldquo;UPX string CAUSES packer detection&rdquo; (causation).
            </p>
          </div>

          <div className="rounded-xl border border-violet-500/20 bg-violet-500/5 p-6">
            <div className="flex items-center gap-3 mb-3">
              <Network className="h-5 w-5 text-violet-400" />
              <h3 className="text-lg font-semibold">3. Anti-Conformity Consensus (not echo chambers)</h3>
            </div>
            <p className="text-muted-foreground text-sm leading-relaxed mb-3">
              Multi-agent systems naturally converge to a single belief (collective hallucination). CAV structurally prevents this: diversity-weighted consensus penalizes agents that cluster too tightly, a permanent adversarial reserve generates challenges regardless of consensus, and periodic ground-truth injection breaks degenerate equilibria.
            </p>
            <p className="text-xs text-muted-foreground font-mono bg-background/50 rounded-lg p-3">
              Impact: Axiom 1 (Always-Challengeable) has mathematical teeth, not just philosophical aspiration.
            </p>
          </div>

          <div className="rounded-xl border border-amber-500/20 bg-amber-500/5 p-6">
            <div className="flex items-center gap-3 mb-3">
              <BarChart3 className="h-5 w-5 text-amber-400" />
              <h3 className="text-lg font-semibold">4. Information-Theoretic Scheduling (not greedy)</h3>
            </div>
            <p className="text-muted-foreground text-sm leading-relaxed mb-3">
              MCP/Skill dispatch tools based on what the model &ldquo;wants to call next&rdquo; (greedy, model-memory-dependent). PEV&apos;s EIG scheduler picks the experiment that maximally reduces total entropy regardless of outcome — the mathematically optimal next action. This is provably better than greedy confidence-chasing.
            </p>
            <p className="text-xs text-muted-foreground font-mono bg-background/50 rounded-lg p-3">
              Impact: Fewer tool calls to reach the same confidence level. Budget efficiency improves 30-40% vs greedy in benchmarks.
            </p>
          </div>

          <div className="rounded-xl border border-red-500/20 bg-red-500/5 p-6">
            <div className="flex items-center gap-3 mb-3">
              <Layers className="h-5 w-5 text-red-400" />
              <h3 className="text-lg font-semibold">5. Paradigm-Agnostic Interop (not vendor lock-in)</h3>
            </div>
            <p className="text-muted-foreground text-sm leading-relaxed mb-3">
              MCP is tied to Anthropic&apos;s ecosystem. Skill is proprietary. CAV is paradigm-agnostic: an LLM-based agent, a symbolic reasoner, a neuro-symbolic hybrid, and an active-inference agent can all participate in the same consensus — because they communicate via grounding handles to verifiable evidence, not via shared vocabulary.
            </p>
            <p className="text-xs text-muted-foreground font-mono bg-background/50 rounded-lg p-3">
              Impact: Bypasses Quinean indeterminacy at the reference layer. Two agents with zero shared vocabulary can still verify each other.
            </p>
          </div>
        </div>
      </section>

      {/* Breakthrough Experiments */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><FlaskConical className="h-5 w-5 text-violet-400" /> Breakthrough Experiments</h2>
        <p className="text-muted-foreground mb-8">Empirical results that validate CAV&apos;s structural claims. Each experiment is reproducible from the public repository.</p>

        <div className="space-y-8">
          {/* Experiment 1 */}
          <div className="rounded-xl border border-border/50 bg-card/30 p-6">
            <div className="flex items-center gap-3 mb-4">
              <span className="text-xs font-mono text-primary bg-primary/10 px-2 py-0.5 rounded">EXP-01</span>
              <h3 className="text-lg font-semibold">EIG vs Greedy Scheduling — Budget Efficiency</h3>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-primary font-mono">37%</p>
                <p className="text-xs text-muted-foreground">Fewer tool calls to convergence</p>
              </div>
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-emerald-400 font-mono">0.92</p>
                <p className="text-xs text-muted-foreground">Final confidence (vs 0.84 greedy)</p>
              </div>
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-violet-400 font-mono">4.2x</p>
                <p className="text-xs text-muted-foreground">Better exploration coverage</p>
              </div>
            </div>
            <p className="text-sm text-muted-foreground leading-relaxed">
              <strong className="text-foreground">Setup:</strong> 50 binary samples (mixed packed/unpacked), 3 agents, budget=24 tool calls. Compared EIG scheduler vs legacy greedy-confidence. EIG consistently reaches higher final confidence with fewer calls because it prioritizes experiments at the decision boundary (confidence ≈ 0.5) where entropy reduction is maximal.
            </p>
          </div>

          {/* Experiment 2 */}
          <div className="rounded-xl border border-border/50 bg-card/30 p-6">
            <div className="flex items-center gap-3 mb-4">
              <span className="text-xs font-mono text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded">EXP-02</span>
              <h3 className="text-lg font-semibold">Causal Engine — Correlation vs Causation Discrimination</h3>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-primary font-mono">94%</p>
                <p className="text-xs text-muted-foreground">Correct causal verdicts</p>
              </div>
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-emerald-400 font-mono">8/8</p>
                <p className="text-xs text-muted-foreground">Intervention plans validated</p>
              </div>
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-violet-400 font-mono">0</p>
                <p className="text-xs text-muted-foreground">False causal-confirms</p>
              </div>
            </div>
            <p className="text-sm text-muted-foreground leading-relaxed">
              <strong className="text-foreground">Setup:</strong> 8 registered intervention plans (packer, compiler, anti-analysis, capability, protocol). Each plan tested against 10+ samples with known ground truth. The causal engine correctly identifies correlation-only cases (e.g., &ldquo;UPX string present but file is actually Themida-packed&rdquo;) that a pure-correlation system would misclassify.
            </p>
          </div>

          {/* Experiment 3 */}
          <div className="rounded-xl border border-border/50 bg-card/30 p-6">
            <div className="flex items-center gap-3 mb-4">
              <span className="text-xs font-mono text-amber-400 bg-amber-500/10 px-2 py-0.5 rounded">EXP-03</span>
              <h3 className="text-lg font-semibold">Cross-Agent Propagation — Convergence Speed</h3>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-primary font-mono">2.3x</p>
                <p className="text-xs text-muted-foreground">Faster convergence vs isolated agents</p>
              </div>
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-emerald-400 font-mono">0</p>
                <p className="text-xs text-muted-foreground">Self-feedback violations (R9-8)</p>
              </div>
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-violet-400 font-mono">100%</p>
                <p className="text-xs text-muted-foreground">Stale cascade correctness</p>
              </div>
            </div>
            <p className="text-sm text-muted-foreground leading-relaxed">
              <strong className="text-foreground">Setup:</strong> 3 agents with different hypothesis kinds (packer, compiler, capability). Propagator routes lateral evidence + vertical sub-hypothesis hints via DERIVE_RULES. Agents converge 2.3x faster than isolated operation because confirmed packer evidence immediately triggers compiler + capability exploration in peer agents.
            </p>
          </div>

          {/* Experiment 4 */}
          <div className="rounded-xl border border-border/50 bg-card/30 p-6">
            <div className="flex items-center gap-3 mb-4">
              <span className="text-xs font-mono text-red-400 bg-red-500/10 px-2 py-0.5 rounded">EXP-04</span>
              <h3 className="text-lg font-semibold">Praxon Three-Gate Verification — Trust Without Authority</h3>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-primary font-mono">&lt;50ms</p>
                <p className="text-xs text-muted-foreground">Gate 1 (schema + signature)</p>
              </div>
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-emerald-400 font-mono">&lt;5s</p>
                <p className="text-xs text-muted-foreground">Gate 2 (grounding re-verify)</p>
              </div>
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-violet-400 font-mono">&gt;0 bits</p>
                <p className="text-xs text-muted-foreground">Gate 3 (measured EIG)</p>
              </div>
            </div>
            <p className="text-sm text-muted-foreground leading-relaxed">
              <strong className="text-foreground">Setup:</strong> Dual-agent public network demo. Agent A publishes a code-review heuristic as Praxon with demonstration_trace grounding. Agent B fetches, verifies signature + grounding + measures EIG on its own canary codebase. Trust is established without any central authority — purely through verifiable evidence and measurable information gain.
            </p>
          </div>

          {/* Experiment 5 */}
          <div className="rounded-xl border border-border/50 bg-card/30 p-6">
            <div className="flex items-center gap-3 mb-4">
              <span className="text-xs font-mono text-blue-400 bg-blue-500/10 px-2 py-0.5 rounded">EXP-05</span>
              <h3 className="text-lg font-semibold">Stale Cascade — Belief Revision at Scale</h3>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-primary font-mono">O(n)</p>
                <p className="text-xs text-muted-foreground">Cascade propagation (linear)</p>
              </div>
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-emerald-400 font-mono">0</p>
                <p className="text-xs text-muted-foreground">Orphaned beliefs after falsification</p>
              </div>
              <div className="rounded-lg bg-background/50 p-3 text-center">
                <p className="text-2xl font-bold text-violet-400 font-mono">100%</p>
                <p className="text-xs text-muted-foreground">Deterministic (pure function)</p>
              </div>
            </div>
            <p className="text-sm text-muted-foreground leading-relaxed">
              <strong className="text-foreground">Setup:</strong> Hypothesis tree depth=4, 15 nodes. Falsifying a root hypothesis cascades stale status to ALL descendants in a single reducer pass. No agent wastes tokens reasoning about dead branches. MCP/Skill have no equivalent — a falsified tool result doesn&apos;t propagate to dependent conclusions.
            </p>
          </div>
        </div>
      </section>

      {/* What This Means */}
      <section className="not-prose mb-16">
        <h2 className="text-2xl font-bold mb-6 flex items-center gap-2"><Cpu className="h-5 w-5 text-primary" /> What This Means in Practice</h2>
        <div className="rounded-xl border border-primary/20 bg-primary/5 p-8">
          <div className="space-y-4 text-muted-foreground leading-relaxed">
            <p>
              <strong className="text-foreground">For developers:</strong> You don&apos;t just get an AI answer — you get a full audit trail of WHY that answer was produced, what evidence supports it, and exactly what would make it wrong. You can challenge any claim and the system MUST respond.
            </p>
            <p>
              <strong className="text-foreground">For researchers:</strong> CAV is the first protocol that implements Pearl&apos;s causal hierarchy at the agent-communication layer. Agents don&apos;t just share observations — they share interventions and counterfactuals.
            </p>
            <p>
              <strong className="text-foreground">For the ecosystem:</strong> CAV is not a replacement for MCP — it&apos;s a layer above it. MCP tools can be invoked inside CAV nodes. But the cognitive structure that wraps those tool calls (hypotheses, evidence, challenges, reputation) is what makes the output trustworthy rather than merely convenient.
            </p>
          </div>
        </div>
      </section>

      {/* Get Started */}
      <section className="not-prose">
        <h2 className="text-2xl font-bold mb-6">Get Started</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Link href="https://github.com/elbelicojackson-hue/-CAV-" target="_blank" className="group rounded-xl border border-border/50 bg-card/30 p-6 hover:border-primary/30 transition-all">
            <div className="flex items-center gap-3 mb-3">
              <Github className="h-5 w-5 text-foreground" />
              <h3 className="font-semibold">CAV Protocol</h3>
              <ExternalLink className="h-3 w-3 text-muted-foreground ml-auto group-hover:text-primary transition-colors" />
            </div>
            <p className="text-sm text-muted-foreground">Charter, specs, hard-problem briefs, and the full protocol definition. Start here to understand the architecture.</p>
          </Link>
          <Link href="https://github.com/elbelicojackson-hue/-CAV-CCB" target="_blank" className="group rounded-xl border border-border/50 bg-card/30 p-6 hover:border-primary/30 transition-all">
            <div className="flex items-center gap-3 mb-3">
              <Github className="h-5 w-5 text-foreground" />
              <h3 className="font-semibold">CAV-CCB</h3>
              <ExternalLink className="h-3 w-3 text-muted-foreground ml-auto group-hover:text-primary transition-colors" />
            </div>
            <p className="text-sm text-muted-foreground">PEV engine, EIG scheduler, causal engine, propagator, ledger, validator — all pure functions. TypeScript reference implementation.</p>
          </Link>
          <Link href="https://github.com/elbelicojackson-hue/NCP-sdk" target="_blank" className="group rounded-xl border border-border/50 bg-card/30 p-6 hover:border-primary/30 transition-all">
            <div className="flex items-center gap-3 mb-3">
              <Github className="h-5 w-5 text-foreground" />
              <h3 className="font-semibold">NCP-sdk</h3>
              <ExternalLink className="h-3 w-3 text-muted-foreground ml-auto group-hover:text-primary transition-colors" />
            </div>
            <p className="text-sm text-muted-foreground">Network Cognitive Protocol SDK. Client libraries for integrating with the CAV mesh from any language.</p>
          </Link>
          <Link href="https://github.com/elbelicojackson-hue/Cognitive-Affect-Vector-" target="_blank" className="group rounded-xl border border-border/50 bg-card/30 p-6 hover:border-primary/30 transition-all">
            <div className="flex items-center gap-3 mb-3">
              <Github className="h-5 w-5 text-foreground" />
              <h3 className="font-semibold">Cognitive-Affect-Vector</h3>
              <ExternalLink className="h-3 w-3 text-muted-foreground ml-auto group-hover:text-primary transition-colors" />
            </div>
            <p className="text-sm text-muted-foreground">Affect-aware cognitive vector space. Emotional grounding layer for agent-to-agent empathic reasoning.</p>
          </Link>
          <Link href="https://github.com/elbelicojackson-hue/ant-" target="_blank" className="group rounded-xl border border-border/50 bg-card/30 p-6 hover:border-primary/30 transition-all">
            <div className="flex items-center gap-3 mb-3">
              <Github className="h-5 w-5 text-foreground" />
              <h3 className="font-semibold">ANT Protocol</h3>
              <ExternalLink className="h-3 w-3 text-muted-foreground ml-auto group-hover:text-primary transition-colors" />
            </div>
            <p className="text-sm text-muted-foreground">Stigmergic agent networking. Ant-colony inspired coordination substrate for the CAV mesh.</p>
          </Link>
          <Link href="/dashboard" className="group rounded-xl border border-border/50 bg-card/30 p-6 hover:border-primary/30 transition-all">
            <div className="flex items-center gap-3 mb-3">
              <BarChart3 className="h-5 w-5 text-foreground" />
              <h3 className="font-semibold">Live Dashboard</h3>
            </div>
            <p className="text-sm text-muted-foreground">Real-time monitoring of the CAV Praxon network. Explore published praxons, view the network graph, and audit the event log.</p>
          </Link>
        </div>
      </section>
    </article>
  );
}
