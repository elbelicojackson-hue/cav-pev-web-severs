# Design: PEV EIG Scheduler — 信息论最优实验设计

## Overview

将 PEV scheduler 的调度策略从"贪心选最高 confidence"升级为"选 Expected Information Gain 最大的 (H, plan) 对"。核心洞察：**最值得测试的不是你最确定的假设，而是测试结果能最大程度改变你认知的假设。**

数学基础：Shannon 信息论 + Bayesian 实验设计。在药物发现和主动学习领域已有 30 年理论积累，但从未被应用于多智能体逆向工程工具调度。

## Architecture

```
scheduler.ts (现有，不修改签名)
  └─ 内部调用 eigEngine.ts (新增)
       ├─ computeEIG(H, plan, ledger)     → { eig, breakdown }
       ├─ computePlanStats(planId, ledger) → { confirmRate, ... }
       └─ rankCandidates(candidates[])    → sorted by EIG desc

planStats.ts (新增)
  └─ 纯函数：从 evidenceLog 聚合 per-plan 统计
```

## Core Algorithm: EIG Computation

### 数学推导

对于一个二元假设 H（要么 true 要么 false），当前 confidence = p：

```
当前熵: H(p) = -p·log₂(p) - (1-p)·log₂(1-p)
```

如果我们跑一个 tool plan，可能的结果有三种：
- confirm (概率 α)
- falsify (概率 β)  
- inconclusive (概率 γ = 1 - α - β)

每种结果后的后验 confidence（Bayesian update）：

```
p_after_confirm = p · sensitivity / (p · sensitivity + (1-p) · false_positive_rate)
                ≈ min(p + Δ_confirm, 1.0)  [简化：直接加一个 step]

p_after_falsify = p · (1 - sensitivity) / (p · (1-sensitivity) + (1-p) · specificity)
                ≈ max(p - Δ_falsify, 0.0)  [简化]

p_after_inconclusive = p  [不变]
```

**工程简化**（v1 不做完整 Bayesian，用经验 step）：

```
Δ_confirm = 0.2 · (1 - p)    [越不确定，confirm 的 lift 越大]
Δ_falsify = 0.2 · p           [越确定，falsify 的 drop 越大]
```

期望后验熵：

```
E[H_after] = α · H(p + Δ_confirm) + β · H(p - Δ_falsify) + γ · H(p)
```

最终 EIG：

```
EIG = H(p) - E[H_after]
```

### Pseudocode

```
ALGORITHM computeEIG(hypothesis, plan, ledger)
INPUT: hypothesis (with .confidence), plan (with .id), ledger
OUTPUT: { eig: number, breakdown: EIGBreakdown }

BEGIN
  p ← hypothesis.confidence
  
  // Clamp to avoid log(0)
  p ← clamp(p, 0.001, 0.999)
  
  // Current entropy
  priorEntropy ← binaryEntropy(p)
  
  // Get plan hit rates from evidence history
  stats ← computePlanStats(plan.id, ledger)
  α ← stats.confirmRate      // P(confirm | plan)
  β ← stats.falsifyRate      // P(falsify | plan)
  γ ← stats.inconclusiveRate // P(inconclusive | plan)
  
  // Posterior after each outcome
  Δ_confirm ← 0.2 * (1 - p)
  Δ_falsify ← 0.2 * p
  
  posteriorIfConfirm ← clamp(p + Δ_confirm, 0.001, 0.999)
  posteriorIfFalsify ← clamp(p - Δ_falsify, 0.001, 0.999)
  posteriorIfInconclusive ← p
  
  // Expected posterior entropy
  expectedPosteriorEntropy ← 
    α * binaryEntropy(posteriorIfConfirm) +
    β * binaryEntropy(posteriorIfFalsify) +
    γ * binaryEntropy(posteriorIfInconclusive)
  
  eig ← priorEntropy - expectedPosteriorEntropy
  
  // EIG should never be negative (information can't decrease in expectation)
  eig ← max(eig, 0)
  
  RETURN {
    eig,
    breakdown: {
      priorEntropy,
      expectedPosteriorEntropy,
      confirmProb: α,
      falsifyProb: β,
      inconclusiveProb: γ,
      posteriorIfConfirm,
      posteriorIfFalsify,
    }
  }
END

FUNCTION binaryEntropy(p)
  IF p ≤ 0 OR p ≥ 1 THEN RETURN 0
  RETURN -(p * log2(p) + (1-p) * log2(1-p))
END
```

### Plan Stats Aggregation

```
ALGORITHM computePlanStats(planId, ledger)
INPUT: planId (string), ledger (SharedLedger)
OUTPUT: { confirmRate, falsifyRate, inconclusiveRate, sampleCount }

BEGIN
  confirms ← 0
  falsifies ← 0
  inconclusives ← 0
  
  FOR EACH ev IN ledger.evidenceLog DO
    // Match by plan tool name (same as current scheduler logic)
    plan ← findToolPlan(planId)
    IF plan = null THEN CONTINUE
    IF ev.toolName ≠ plan.tool THEN CONTINUE
    
    SWITCH ev.verdict
      CASE 'confirms': confirms += 1
      CASE 'falsifies': falsifies += 1
      DEFAULT: inconclusives += 1
    END SWITCH
  END FOR
  
  total ← confirms + falsifies + inconclusives
  
  IF total < 3 THEN
    // Laplace smoothing with pseudocounts
    confirms += 1
    falsifies += 1
    inconclusives += 0.5
    total ← confirms + falsifies + inconclusives
  END IF
  
  IF total = 0 THEN
    // Uniform prior (no data at all)
    RETURN { confirmRate: 0.4, falsifyRate: 0.4, inconclusiveRate: 0.2, sampleCount: 0 }
  END IF
  
  RETURN {
    confirmRate: confirms / total,
    falsifyRate: falsifies / total,
    inconclusiveRate: inconclusives / total,
    sampleCount: total,
  }
END
```

### EIG-aware Schedule Algorithm

```
ALGORITHM scheduleWithEIG(ledger, agents, currentRound, budget, opts)
INPUT: same as current schedule() + opts.strategy
OUTPUT: same as current schedule()

BEGIN
  IF opts.strategy = 'greedy-confidence' THEN
    RETURN schedule_legacy(ledger, agents, currentRound, budget)
  END IF
  
  perAgentDirective ← Map()
  observerCount ← 0
  
  FOR EACH agent IN agents DO
    activeOwn ← agent's active hypotheses (same filter as before)
    
    IF activeOwn is empty THEN
      perAgentDirective.set(agent.id, { hint: 'no own active H, observe' })
      observerCount += 1
      CONTINUE
    END IF
    
    // Build all candidate (H, plan) pairs
    candidates ← []
    FOR EACH h IN activeOwn WHERE h.lastTouchedRound < currentRound DO
      plans ← getToolPlansForKind(h.kind)
      FOR EACH plan IN plans DO
        IF NOT alreadyTested(h.id, plan.tool, ledger) THEN
          eigResult ← computeEIG(h, plan, ledger)
          bonus ← computeExplorationBonus(h, plan, ledger, opts.explorationWeight)
          candidates.push({ h, plan, eig: eigResult.eig, bonus, total: eigResult.eig + bonus, breakdown: eigResult.breakdown })
        END IF
      END FOR
    END FOR
    
    IF candidates is empty THEN
      perAgentDirective.set(agent.id, { hint: 'all plans exhausted' })
      CONTINUE
    END IF
    
    // Sort by total (EIG + bonus) descending, tie-break by cost then id
    candidates.sort(by total DESC, then cost_estimate ASC, then h.id ASC)
    
    best ← candidates[0]
    
    IF best.total < 0.01 THEN
      perAgentDirective.set(agent.id, { hint: 'low-information: consider declare_done', eig: best.eig })
      observerCount += 1
      CONTINUE
    END IF
    
    perAgentDirective.set(agent.id, {
      suggestedHypothesisId: best.h.id,
      suggestedToolPlanId: best.plan.id,
      eig: best.eig,
      eigBreakdown: best.breakdown,
    })
  END FOR
  
  stallGuardWarning ← (observerCount = agents.length)
  RETURN { perAgentDirective, stallGuardWarning }
END
```

## Data Models

### EIGBreakdown

```typescript
export type EIGBreakdown = {
  readonly priorEntropy: number        // H(p) in bits
  readonly expectedPosteriorEntropy: number  // E[H(p|result)]
  readonly confirmProb: number         // α
  readonly falsifyProb: number         // β
  readonly inconclusiveProb: number    // γ
  readonly posteriorIfConfirm: number  // p after confirm
  readonly posteriorIfFalsify: number  // p after falsify
}
```

### PlanStats

```typescript
export type PlanStats = {
  readonly confirmRate: number         // [0, 1]
  readonly falsifyRate: number         // [0, 1]
  readonly inconclusiveRate: number    // [0, 1]
  readonly sampleCount: number         // raw count before smoothing
}
```

### Extended ScheduleDirective

```typescript
// 新增可选字段（向后兼容）
export type ScheduleDirective = {
  readonly suggestedHypothesisId?: string
  readonly suggestedToolPlanId?: string
  readonly hint?: string
  readonly eig?: number                // NEW: bits
  readonly eigBreakdown?: EIGBreakdown // NEW: for UI/debug
}
```

## Component Layout

| File | Responsibility | Pure? |
|------|---------------|-------|
| `src/services/cav/pev/eigEngine.ts` (NEW) | `computeEIG`, `binaryEntropy`, `rankCandidates` | ✓ |
| `src/services/cav/pev/planStats.ts` (NEW) | `computePlanStats`, Laplace smoothing | ✓ |
| `src/services/cav/pev/scheduler.ts` (MODIFY) | 内部调用 eigEngine，保持签名不变 | ✓ |
| `src/commands/ccb-pev/PevSession.tsx` (MODIFY) | 显示 EIG 值 | — |

## Properties (Testable Invariants)

1. **EIG ∈ [0, 1]** — 二元假设的 EIG 永远不超过 1 bit。
2. **EIG(p=0.5) ≥ EIG(p=0.9)** — 最不确定的假设信息增益最大（当 plan 的 confirm/falsify 概率对称时）。
3. **EIG 单调性** — 对固定 plan stats，EIG 关于 |p - 0.5| 单调递减（越接近 0.5 越大）。
4. **Exploration bonus 衰减** — 随着 (H, plan) 被测试次数增加，bonus 趋向 0。
5. **向后兼容** — `strategy='greedy-confidence'` 时输出与旧 scheduler 完全一致。
6. **Laplace smoothing** — sampleCount < 3 时，stats 不会出现 0 或 1 的极端概率。

## Migration

- v1: 新增 `eigEngine.ts` + `planStats.ts`，修改 `scheduler.ts` 内部实现（不改签名）
- 默认 strategy 改为 `'eig'`
- `PevRunOpts` 新增可选字段 `schedulerStrategy?: 'greedy-confidence' | 'eig'`
- 现有测试通过 `strategy: 'greedy-confidence'` 保持绿色
- 新增 EIG 专项测试

## Open Questions (已收敛)

1. **Δ_confirm / Δ_falsify 的步长 0.2 是否合适？** → v1 用 0.2 作为经验值；后续可通过 meta-learning (R5) 从历史 session 学习最优步长。
2. **是否需要考虑 plan 之间的相关性？** → v1 不考虑（假设 plan 独立）；v2 可引入 plan 相关矩阵。
3. **EIG 是否应该考虑 stale cascade 的连锁效应？** → v1 不考虑（只看单个 H 的熵变）；v2 可引入"如果 H 被 falsify，其子树的总熵减少量"作为 bonus。
