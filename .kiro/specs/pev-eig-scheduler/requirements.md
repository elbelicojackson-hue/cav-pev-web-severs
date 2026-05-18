# Requirements: PEV EIG Scheduler — 信息论最优实验设计

## Introduction

当前 PEV scheduler (`scheduler.ts`) 使用**贪心策略**：选 confidence 最高的假设 + 第一个未测试的 tool plan。这在假设数量少时够用，但存在根本缺陷：

- **高 confidence 假设不一定是最值得测试的** — 一个 confidence=0.9 的假设，无论 confirm 还是 falsify，对整体知识增量都很小（因为我们已经很确定了）
- **低 confidence 假设可能信息量更大** — 一个 confidence=0.5 的假设，测试结果无论哪个方向都会大幅改变我们的认知
- **plan 之间的信息量差异被忽略** — `packer::diec` 和 `packer::upx-test` 对同一个假设的信息贡献完全不同

本特性引入 **Expected Information Gain (EIG)** 作为调度的核心指标，将 scheduler 从"贪心选最确定的"升级为"选能最大化知识增量的实验"。

核心公式：

```
EIG(H, plan) = H_before - E[H_after | plan_result]
             = -Σ p(outcome) · log p(outcome)  [当前熵]
             - E[ -Σ p(outcome|result) · log p(outcome|result) ]  [期望后验熵]
```

简化为二元假设（confirm/falsify）的情况：

```
EIG(H, plan) = H(p) - [ P(confirm|plan) · H(p_after_confirm) + P(falsify|plan) · H(p_after_falsify) ]

其中:
  p = H.confidence (当前先验)
  H(p) = -p·log(p) - (1-p)·log(1-p)  [二元熵]
  P(confirm|plan) = plan 的历史 confirm 率 (从 evidenceLog 统计)
  P(falsify|plan) = plan 的历史 falsify 率
  p_after_confirm = Bayesian update: p · P(confirm|H_true) / P(confirm)
  p_after_falsify = Bayesian update
```

## Glossary

- **EIG**: Expected Information Gain — 一次实验（tool call）预期能减少的假设集总熵（bits）
- **Prior**: 假设的当前 confidence，视为 P(H=true)
- **Likelihood**: P(plan_result | H_status) — plan 在假设为真/假时的输出概率
- **Posterior**: Bayesian update 后的 confidence
- **Binary Entropy**: H(p) = -p·log₂(p) - (1-p)·log₂(1-p)
- **Plan Hit Rate**: 某 plan 在历史 evidence 中的 confirm/falsify/inconclusive 比例
- **Information Budget**: 每轮可分配的 EIG 总量上限（防止所有 agent 都去测同一个 H）
- **Exploration Bonus**: 对从未被测试过的 (H, plan) 对的 EIG 加成（鼓励探索）

## Requirements

### Requirement 1: EIG 计算引擎

**User Story:** 作为 scheduler 的实现者，我希望有一个纯函数能计算任意 (hypothesis, plan) 对的 Expected Information Gain，这样我能用它替代当前的 confidence 排序。

#### Acceptance Criteria

1. THE 系统 SHALL 提供纯函数 `computeEIG(hypothesis, plan, ledger, priors?) → { eig: number; breakdown: EIGBreakdown }`。
2. THE EIG 值 SHALL 以 bits 为单位（log₂），范围 [0, 1]（二元假设的最大熵为 1 bit）。
3. WHEN hypothesis.confidence === 0.5 且 plan 的 confirm/falsify 概率均为 0.5，THE EIG SHALL 等于 1.0 bit（最大信息增益）。
4. WHEN hypothesis.confidence === 0.99 或 0.01（极端确定），THE EIG SHALL 接近 0（已经很确定，测试价值低）。
5. THE `priors` 参数 SHALL 允许注入自定义的 P(confirm|plan) 和 P(falsify|plan)；缺省时从 `ledger.evidenceLog` 统计该 plan 的历史命中率。
6. WHEN 某 plan 在 ledger 中无历史记录（首次使用），THE 系统 SHALL 使用均匀先验 P(confirm) = P(falsify) = 0.4, P(inconclusive) = 0.2。
7. THE `EIGBreakdown` SHALL 包含字段 `{ priorEntropy, expectedPosteriorEntropy, confirmProb, falsifyProb, inconclusiveProb, posteriorIfConfirm, posteriorIfFalsify }`，用于 UI 展示和调试。
8. THE 计算 SHALL 在 ≤ 1ms 内完成（对 100 个 (H, plan) 对的批量计算 ≤ 100ms）。

### Requirement 2: EIG-aware Scheduler

**User Story:** 作为 PEV 主循环的驱动者，我希望 scheduler 按 EIG 降序选择 (H, plan) 对，而不是按 confidence 降序，这样每次 tool call 都能最大化知识增量。

#### Acceptance Criteria

1. THE scheduler SHALL 对每个 agent 的所有候选 (H, plan) 对计算 EIG，选 EIG 最大的作为 directive。
2. WHEN 多个 (H, plan) 对的 EIG 相等（tie），THE scheduler SHALL 按以下优先级 tie-break：(a) plan.cost_estimate 升序（便宜优先）；(b) hypothesis.id 升序（确定性）。
3. THE scheduler SHALL 支持 `strategy: 'greedy-confidence' | 'eig'` 配置，默认 `'eig'`；设为 `'greedy-confidence'` 时回退到当前行为（向后兼容）。
4. THE scheduler SHALL 在 directive 中附加 `eig: number` 字段，供 UI 和 promptBuilder 展示。
5. WHEN 所有候选 (H, plan) 对的 EIG < 0.01 bit，THE scheduler SHALL 触发 `low-information` 提示（类似 stall-guard 但不停机），建议 agent 考虑 `declare_done` 或 `mutate`。
6. THE scheduler SHALL 不修改 `schedule()` 的函数签名（向后兼容）；EIG 策略通过 `PevBudget` 或新增 `SchedulerOpts` 参数注入。

### Requirement 3: Exploration Bonus

**User Story:** 作为防止 scheduler 陷入局部最优的机制设计者，我希望从未被测试过的 (H, plan) 对获得额外的 EIG 加成，这样系统不会永远只测"看起来最有信息量"的那一个。

#### Acceptance Criteria

1. THE 系统 SHALL 对从未在 `ledger.evidenceLog` 中出现过的 (H.id, plan.tool) 组合施加 exploration bonus。
2. THE exploration bonus SHALL 为 `0.1 * (1 - timesTestedRatio)` bits，其中 `timesTestedRatio = 该 H 被测试的次数 / 该 kind 的总 plan 数`。
3. THE bonus SHALL 加在 EIG 之上（不替代），最终排序值 = `EIG + explorationBonus`。
4. WHEN 某 (H, plan) 已被测试过（evidence 存在），THE bonus SHALL 为 0。
5. THE exploration bonus 的系数 SHALL 可通过 `PevRunOpts.explorationWeight` 配置（默认 0.1）。

### Requirement 4: 历史统计聚合

**User Story:** 作为 EIG 计算的数据源，我希望系统能从当前 session 的 evidenceLog 中实时统计每个 plan 的 confirm/falsify/inconclusive 比例，作为 likelihood 估计。

#### Acceptance Criteria

1. THE 系统 SHALL 提供 `computePlanStats(planId, ledger) → { confirmRate, falsifyRate, inconclusiveRate, sampleCount }`。
2. WHEN `sampleCount === 0`（该 plan 从未被使用），THE 系统 SHALL 返回均匀先验 `{ confirmRate: 0.4, falsifyRate: 0.4, inconclusiveRate: 0.2, sampleCount: 0 }`。
3. THE 统计 SHALL 仅基于当前 session 的 `ledger.evidenceLog`（不跨 session，除非 R5 的 meta-learning 扩展启用）。
4. THE 统计 SHALL 是纯函数（同一 ledger 输入 → 同一输出）。
5. WHEN `sampleCount < 3`，THE 系统 SHALL 对统计值做 Laplace smoothing（+1 伪计数），防止极端概率（0 或 1）导致 EIG 计算退化。

### Requirement 5: 跨 Session Meta-Learning（可选扩展）

**User Story:** 作为长期使用者，我希望系统能记住"哪些 plan 对哪些 kind 的假设最有效"，这样新 session 的前几轮不需要从均匀先验开始。

#### Acceptance Criteria

1. THE 系统 MAY 在 PEV 结束时将 plan stats 写入 `<sessionDir>/pev-meta.json`。
2. THE 系统 MAY 在 PEV 启动时读取 `pev-meta.json` 作为 prior（如果文件存在）。
3. THE meta prior SHALL 与当前 session 的 evidence 做 Bayesian 混合（meta 权重随 session 内 sample count 增加而衰减）。
4. THIS requirement is OPTIONAL for v1 — 可以在 v2 实现。v1 仅使用 session-local 统计。

### Requirement 6: UI 展示

**User Story:** 作为 RE 工程师，我希望在 PevSession UI 中看到每个 directive 的 EIG 值和 breakdown，这样我能理解 scheduler 为什么选了这个实验。

#### Acceptance Criteria

1. THE `ScheduleDirective` SHALL 新增可选字段 `eig?: number` 和 `eigBreakdown?: EIGBreakdown`。
2. THE PevSession UI SHALL 在 AgentStatusBar 或 directive 区域显示 EIG 值（如 `EIG=0.72 bits`）。
3. THE final summary markdown SHALL 包含一个 "Information Efficiency" 段，统计：总 EIG 消耗 / 总 tool calls = 平均每次 tool call 的信息增益。
4. WHEN `strategy === 'greedy-confidence'`（回退模式），THE UI SHALL 不显示 EIG 相关信息。

### Requirement 7: 零回归

**User Story:** 作为现有 PEV 用户，我希望 EIG 增强不破坏任何现有行为。

#### Acceptance Criteria

1. THE 改造 SHALL 不修改 `scheduler.ts` 的现有 `schedule()` 函数签名。
2. THE 改造 SHALL 通过新增文件实现（`eigEngine.ts`、`planStats.ts`），不修改 `ledger.ts`、`protocol.ts`、`validator.ts`、`parser.ts`。
3. WHEN `strategy === 'greedy-confidence'`，THE scheduler 行为 SHALL 与改造前完全一致。
4. THE 现有测试 SHALL 全部通过（`bun test src/services/cav/pev/__tests__/`）。
5. THE 新增测试 SHALL 覆盖 EIG 计算的边界条件（confidence=0, 0.5, 1; empty evidence; single evidence; Laplace smoothing）。
