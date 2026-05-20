# Requirements: CAV Anti-Conformity Consensus — 公理 1 的实现层骨架

## Introduction

本 spec 是 CAV Charter (`.kiro/specs/cav-protocol-charter/charter.md`) 之后的第一份 sub-spec,目标是实现 HP-02(Collective Hallucination)的对抗机制。

### 为什么这是 P0

Charter 公理 1(Always-Challengeable)在表述层声明"任何 claim 可被挑战"。但**如果 consensus 引擎本身倾向于把所有挑战者拉进同一个自由能洼地**,则公理 1 在数学上是空集——挑战机制形式上存在,实质上无人挑战、即便挑战也被同化。

> Collective hallucination is the implementation-layer inversion of Axiom 1.

公理 1 在没有反同质化的 consensus 引擎之前,是修辞而不是机制。本 spec 给它装上"牙齿"。

### Charter v0.3 双层架构定位(关键)

本 spec 完全位于 **Operational Layer**(Charter §10)。这意味着:

- 本 spec 的 anti-conformity consensus 解决的是**单 claim 级别**的 collective hallucination,不解决 Charter / Axiom 级别的争议
- 本 spec 产出的 reputation 更新只影响 `reputation.operational` 向量,不影响 `reputation.deliberation`
- 本 spec 的 ccbteam 4 链假设(若引入)是**参考实现**,不是协议必需——任何满足本 spec SHALL 条款的 anti-conformity 实现都是 CAV-conformant
- 本 spec **不**定义 Charter Axiom 的修订流程——那是 `cav-deliberation-layer` 的范围

Operational ↔ Deliberation 的边界在两条具体 SHALL 中体现:

- 当本 spec 的 Asch test 在某 claim 类上连续 N 次失败,引擎 SHALL 产出 `escalation_event`,移交 Deliberation Layer 处理(实际处理由 `cav-deliberation-layer` 实现,本 spec 只产生事件)
- 本 spec 的所有 reputation 字段 SHALL 显式标记为 `operational` tier,不允许调用方误用为 deliberation 输入

### 工程优先于理论纯粹(Engineering Pragmatism)

本 spec 的若干阈值——diversity weight 系数、adversarial reserve 20%、ground-truth update 系数比、Asch test 准确率 80%——采用工程实证最优,**不是**从理论推出的最优。理由:

- CAV 的目标不是发表 multi-agent consensus 的理论论文,是部署可工作的协议
- 学术上更纯粹的方案(完全对称 agent、严格 Bayesian aggregation)更易于分析,但**经验上不能阻止 collective hallucination**——这正是 HP-02 的关键发现
- 工程参数与协议公理是分层的:公理(Charter §2)不可妥协,工程参数(本 spec 的具体数值)可被实证修订

这些工程参数的修订路径走 Deliberation Layer §10——任何对 20%、80% 等具体数值的全网修订都需要 30 天 review 窗口与对称投票,不能由本 spec 维护者单方面修改。

### 现状校准

**实事求是的现状**:

- `src/services/cav/pev/` **存在**(15 个文件)——单 session 内多 agent 协作,hypothesis 状态机 + evidence trail + propagator 已完整运转
- `src/services/cav/ccbteam-math/` **不存在**——Charter v0.1 的相关断言已在 v0.2 修正
- "多 agent 共识投票"作为概念**在 PEV 中尚未存在**:PEV 的 `confidence` 是 hypothesis 自身字段,不是跨 agent 投票聚合
- 因此本 spec 的实现是**绿地**(green-field),不是扩展

**复用界面**:

| 复用对象 | 作用 |
|---|---|
| `pev/ledger.ts` | `Hypothesis` / `ToolEvidence` 类型 + 不可变 reducer 模式 |
| `pev/protocol.ts` | `Verdict` / `HypothesisId` 等基础类型 |
| `pev/propagator.ts` | "纯函数 + 路由 + 优先级"模式作为 consensus 引擎的设计参照 |
| `pev/causalEngine.ts` | "Pearl 二阶干预"作为 ground-truth injection 的概念基础 |

**新建模块**:整个 `src/services/cav/consensus/` 目录都是新的。

### 核心机制(简述)

三层防护,任何一层单独都不够:

1. **Diversity-weighted aggregation**——endorsement 权重随其与已计票 endorsement 的相关性单调下降
2. **Permanent adversarial reserve**——一定比例容量永久分配给 prior 距离 consensus 最远的 agent,无视当前共识方向
3. **Retrospective ground-truth update**——当外部 ground truth 浮现,对所有参与 agent 追溯更新声誉,惩罚跟风

## Glossary

- **Endorsement**:一个 agent 对某 claim 的支持表态,带置信度、方法论摘要、grounding handle 引用
- **Endorsement Vector**:endorsement 在 claim-feature 空间中的向量表示,用于 diversity 度量
- **Diversity Weight**:一个 endorsement 的有效权重 ∈ [0, 1],与已计入 endorsement 的相关性反相关
- **Adversarial Reserve**:consensus 评估过程中保留的一部分参与者名额,优先分配给 prior 远离 consensus 的 agent
- **Prior Distance**:两个 agent 在历史 endorsement / canary 任务上的行为差异度量
- **Retrospective Update**:当 ground truth 出现时对历史 endorsement 进行回溯性的声誉调整
- **Asch Test**:为 consensus 引擎设计的可证伪基准——多数 agent 给出协调错误答案,少数 agent 持有正确观点,引擎能否避免 collapse
- **Reputation Vector**:每 agent 的多维声誉(per-domain × per-methodology × time-window),非标量
- **Consensus Episode**:一次围绕特定 claim 的完整投票过程,从开放 endorsement 到产出 verdict 与 audit log

## Scope and Non-Goals

### In Scope (本 spec 范围)

- 单 process 内运行的纯函数 consensus 引擎(可在测试中可重现)
- Endorsement 数据结构与 ledger 持久化格式
- Diversity-weighted aggregation 数学
- Adversarial reserve 选择算法
- Retrospective reputation update 机制
- Asch-style 可证伪基准(测试夹具)

### Out of Scope (留给后续 sub-spec)

- 跨 process / 跨网络的分布式 consensus(留给 `cav-protocol-core`)
- Identity / DID / signature 机制(留给 `cav-identity-and-sybil`,即 HP-03)
- Latent 通道 endorsement 投递(留给 `cav-latent-injection-defense`,即 HP-05)
- Bootstrap / handshake(留给 `cav-bootstrap-handshake`,即 HP-01)
- Layer 5 人类可读解释(留给 `cav-explanation-bridge`)

本 spec **故意**不解决跨进程问题。先在单进程内把数学和接口跑对,再分布化。

## Requirements

### Requirement 1: Endorsement 数据结构

**User Story**:作为 consensus 引擎的实现者,我需要一个能完整描述"agent 对某 claim 的表态"的数据结构,这样后续聚合算法有清晰的输入。

#### Acceptance Criteria

1. THE 系统 SHALL 定义类型 `Endorsement`,包含字段 `{ id, agentId, claimId, position, confidence, methodologyDigest, groundingHandles, timestamp, priorVector }`。
2. THE `position` SHALL 是 `'endorse' | 'reject' | 'abstain'` 三选一。
3. THE `confidence` SHALL ∈ [0, 1] 且 endorse + reject 不能同时 confidence > 0(同一 endorsement 不能既支持又反对)。
4. THE `methodologyDigest` SHALL 是一个固定 schema 的对象,包含 `{ priorSourceTag, inferenceMethodTag, dataSourceHashes }`,用于后续 diversity 度量。
5. THE `groundingHandles` SHALL 是非空字符串数组(对应 Charter Axiom 4 的强制 grounding),空数组的 endorsement 必须被引擎拒绝并记入 audit log。
6. THE `priorVector` SHALL 是一个固定维度的 number 数组(初版 dimension = 32),作为该 agent 在 prior space 的当前位置(diversity 计算的输入)。
7. THE 类型定义文件 SHALL 位于 `src/services/cav/consensus/types.ts`,所有字段 readonly,不可变。
8. THE 类型 SHALL 不引用 `pev/` 任何运行时函数,只引用 `pev/protocol.ts` 的纯类型,以保证 consensus 引擎独立可测。

### Requirement 2: Endorsement 验证器

**User Story**:作为引擎的守门人,我需要一个纯函数把所有非法 endorsement 在进入聚合之前剔除。

#### Acceptance Criteria

1. THE 系统 SHALL 提供纯函数 `validateEndorsement(endorsement, context) → { valid: boolean; reason?: string }`。
2. WHEN `groundingHandles` 为空,THE 验证器 SHALL 返回 `{ valid: false, reason: 'AXIOM_4_VIOLATION_NO_GROUNDING' }`。
3. WHEN `confidence` 不在 [0, 1] 区间,THE 验证器 SHALL 返回 `{ valid: false, reason: 'CONFIDENCE_OUT_OF_RANGE' }`。
4. WHEN `priorVector` 长度不等于约定维度(初版 32),THE 验证器 SHALL 返回 `{ valid: false, reason: 'PRIOR_VECTOR_DIMENSION_MISMATCH' }`。
5. WHEN endorsement 在同一 episode 中已被同一 agentId 提交过,THE 验证器 SHALL 返回 `{ valid: false, reason: 'DUPLICATE_ENDORSEMENT' }`(每 agent 每 episode 一票)。
6. THE 验证器 SHALL 是纯函数(无 I/O,无 Date.now,无 global state),给定相同 (endorsement, context) 必返回相同结果。
7. THE 验证失败的 endorsement SHALL 写入 audit log 但**不**进入聚合。

### Requirement 3: Diversity Weight 数学

**User Story**:作为 anti-conformity 机制的核心数学,我需要一个能在已计入 endorsement 增长时,把后续高度相关 endorsement 的权重压低的函数。

#### Acceptance Criteria

1. THE 系统 SHALL 提供纯函数 `diversityWeight(endorsement, priorEndorsements) → number`,返回值 ∈ [0, 1]。
2. WHEN `priorEndorsements` 为空(第一票),THE diversity weight SHALL = 1.0。
3. WHEN 新 endorsement 与 priorEndorsements 中**任一** endorsement 在 `(priorVector, methodologyDigest)` 上完全相同,THE diversity weight SHALL = 0.0(完全可预测,零信息)。
4. THE 相关性度量 SHALL 使用 `priorVector` 的余弦相似度作为主要信号,辅以 `methodologyDigest` 重叠度的次要修正。
5. THE diversity weight 函数 SHALL 满足**单调下降性**:当 priorEndorsements 中加入一个与新 endorsement 相关性 ≥ 当前最大相关性的项,新 endorsement 的 diversity weight 不增。
6. THE 实现 SHALL 包含基于属性的测试(property-based test):随机生成 N=100 个 endorsement 序列,验证单调下降性总成立。
7. THE diversity weight 计算 SHALL 在 O(M) 内完成(M = priorEndorsements 长度),100 个 endorsement 的全序列权重计算 ≤ 5ms。

### Requirement 4: Aggregation 算法

**User Story**:作为 consensus 引擎的对外接口,我需要一个把若干 endorsement 聚合成 verdict 的函数,且这个聚合考虑 diversity weight。

#### Acceptance Criteria

1. THE 系统 SHALL 提供纯函数 `aggregateEndorsements(endorsements, options?) → ConsensusVerdict`。
2. THE `ConsensusVerdict` SHALL 包含字段 `{ outcome, supportWeight, rejectWeight, abstainWeight, totalRawCount, totalEffectiveWeight, diversityScore, byAgentBreakdown }`。
3. THE `outcome` SHALL 是 `'endorsed' | 'rejected' | 'inconclusive'` 三选一,判定阈值:
   - `endorsed`:supportWeight ≥ rejectWeight × 1.5 且 supportWeight ≥ totalEffectiveWeight × 0.4
   - `rejected`:rejectWeight ≥ supportWeight × 1.5 且 rejectWeight ≥ totalEffectiveWeight × 0.4
   - 其它情况:`inconclusive`
4. THE 聚合算法 SHALL 按如下顺序计票:
   - 按 timestamp 升序遍历 endorsements
   - 对每条计算 `diversityWeight(this, priorAccepted)`
   - 累加 `confidence × diversityWeight` 到对应桶
   - 把当前 endorsement 加入 priorAccepted
5. THE `diversityScore` SHALL 是所有 endorsement 的 diversity weight 平均值,反映该 episode 整体的多元程度(用于审计)。
6. THE 聚合 SHALL 是纯函数;给定相同 endorsements(顺序敏感),返回相同 verdict。
7. THE 聚合 SHALL 在 100 endorsement 输入下 ≤ 50ms。

### Requirement 5: Adversarial Reserve 机制

**User Story**:作为防止 consensus 引擎陷入"所有参与者都已被同化"的机制设计者,我需要一种方式确保**永远**有一部分参与者来自远离当前共识的 prior。

#### Acceptance Criteria

1. THE 系统 SHALL 提供函数 `selectAdversarialReserve(candidatePool, currentConsensusDirection, reserveFraction) → AgentDescriptor[]`。
2. THE `reserveFraction` SHALL 默认为 0.20(20%),可配置但不可低于 0.10。
3. THE 选择算法 SHALL 优先选择 `priorVector` 与 `currentConsensusDirection` 余弦距离**最大**的 agent,而非"被配置为反对"的 agent。
4. WHEN candidatePool 中 prior-distant agent 数量不足,THE 函数 SHALL 返回所有 prior-distant agent 并在 audit log 标记 `ADVERSARIAL_RESERVE_UNDERFILLED`(不假装满足配额)。
5. THE 选中的 adversarial agent 在该 episode 的 endorsement SHALL 不享有任何额外权重——它们和普通 endorsement 走同样的 `diversityWeight` 流程。
6. WHEN consensus direction 尚未确定(episode 刚开始),THE 函数 SHALL 选择历史 prior 距离平均共识最大的 agent,而非随机。
7. THE 选择函数 SHALL 是纯函数(给定 candidatePool 状态确定);随机性如需引入必须通过显式 seed 注入,不依赖 Math.random 隐式。

### Requirement 6: Retrospective Reputation Update

**User Story**:作为长期声誉机制的设计者,我需要在 ground truth 浮现时**回溯**调整所有相关 agent 的声誉,这样跟风共识不再是免费的。

#### Acceptance Criteria

1. THE 系统 SHALL 提供函数 `applyGroundTruthUpdate(reputationStore, groundTruthEvent, episodeHistory) → ReputationStore`。
2. THE `GroundTruthEvent` SHALL 包含 `{ claimId, actualOutcome, source, attestations, timestamp }`,其中 `source` 必须是 `'sensor' | 'formal-proof' | 'real-world-outcome' | 'attested-external'` 之一(不允许"另一次 consensus"作为 source——避免 ground truth 自指)。
3. WHEN groundTruthEvent.actualOutcome 与某 episode 的 verdict 一致,THE 该 episode 中投正确票的 agent 声誉 SHALL 增加 ΔR_correct。
4. WHEN groundTruthEvent.actualOutcome 与某 episode 的 verdict 不一致,THE 投错误票的 agent 声誉 SHALL 减少 ΔR_wrong,且 ΔR_wrong > ΔR_correct(惩罚强于奖励,防止跟风套利)。
5. THE 声誉调整 SHALL **同时**惩罚高 confidence 错误投票,且惩罚力度与 confidence 成正比——一个 confidence=0.9 的错误投票,惩罚约为 confidence=0.5 错误投票的 2 倍。
6. THE 声誉 SHALL 是向量化的(per-domain × per-methodology),回溯更新只影响该 episode 的 (domain, methodology) 切片,不污染其他维度。
7. THE 函数 SHALL 是纯函数,产出新的 reputationStore;旧的 reputationStore 不被修改。
8. THE 实现 SHALL 包含审计:每次调整在新 reputationStore 上附带 `lastUpdate: { reason, episodeId, delta }` 字段。

### Requirement 7: Audit Log

**User Story**:作为协议透明性的负责人,我需要 consensus 引擎的每个决策都留下可被外部审计的痕迹。

#### Acceptance Criteria

1. THE 系统 SHALL 提供 `ConsensusAuditEntry` 类型,记录每次 episode 的完整轨迹:`{ episodeId, claimId, endorsementsConsidered, endorsementsRejected, diversityScore, adversarialReserve, finalVerdict, computeDuration, timestamp }`。
2. THE audit log SHALL 是 append-only 的(只追加,不修改、不删除)。
3. THE audit log 数据结构 SHALL 兼容 `pev/ledger.ts` 的 immutable reducer 模式,以便后续与 PEV ledger 合并存储。
4. WHEN endorsement 被 validator 拒绝,THE audit entry SHALL 记录拒绝原因,该 endorsement 仍出现在 audit 中(以"被拒"形式),不被静默丢弃。
5. THE audit log SHALL 暴露查询接口 `queryAuditLog(filter) → ConsensusAuditEntry[]`,支持按 claimId / agentId / time-window / verdict 过滤。
6. THE 查询 SHALL 是只读纯函数,不修改 audit log。
7. THE audit log 持久化格式 SHALL 是 newline-delimited JSON(NDJSON),便于离线工具消费。

### Requirement 8: Asch Test 可证伪基准

**User Story**:作为本 spec 的"它真的工作吗"判定者,我需要一个明确可证伪的测试,且该测试是 spec 的一部分(不是后补的)。

#### Acceptance Criteria

1. THE 系统 SHALL 包含测试夹具 `__tests__/aschBenchmark.test.ts`,实现以下场景:
   - N=5 个模拟 agent
   - 真实答案是 `endorse`
   - N-1 个 agent 协调输出 `reject`(错误共识)
   - 1 个 agent 输出 `endorse`(正确,孤立)
2. WHEN 该场景输入 `aggregateEndorsements`(无 anti-conformity 机制,作为对照),THE outcome SHALL 是 `'rejected'`(错误共识获胜——证明问题存在)。
3. WHEN 该场景输入开启 anti-conformity 的版本,THE outcome SHALL 是 `'endorsed'` 或 `'inconclusive'`(**不能**是 `'rejected'`)。
4. THE 基准 SHALL 包含**100 次随机化运行**,正确率(避免 collapse 到错误共识的比例)≥ 80%(初版目标)。
5. THE 基准 SHALL 包含 60/40 分布对照:60% agent 倾向真,40% 倾向假——anti-conformity 版本的准确率 SHALL 比简单多数投票**至少高 10 个百分点**。
6. THE 基准结果 SHALL 写入 `__tests__/aschBenchmark.snapshot.json`,作为后续优化的回归基线。
7. WHEN 任何后续 PR 让 Asch 基准准确率下降 > 5 个百分点,该 PR SHALL 在 CI 阶段被拒(基准是硬约束)。

### Requirement 9: 与 PEV 集成的桥接(可选,deferred 接口)

**User Story**:作为系统集成视角,我希望 consensus 引擎在概念上能消费 PEV 的 evidence 流,但本 spec 不强制实现这个桥接——只定义接口。

#### Acceptance Criteria

1. THE 系统 SHALL 定义类型 `PevToConsensusBridge`,声明从 `ToolEvidence` / `Hypothesis` 到 `Endorsement` 的潜在映射(接口,无实现)。
2. THE 接口 SHALL 接受 `(hypothesis: Hypothesis, evidence: readonly ToolEvidence[], agentId: string) → Endorsement | null`。
3. WHEN 该桥接的实现在后续 spec 中产出,本 spec 的 consensus 引擎 SHALL 不需要修改即可消费。
4. THE 本 spec 不需提交桥接的实际实现——只需类型声明 + 一段 README 解释设计意图。
5. THE 桥接接口 SHALL 单向:PEV → Consensus。consensus 不反向修改 PEV ledger(避免循环依赖)。

### Requirement 10: 性能与可观测性

**User Story**:作为运行时性能的关心者,我需要 consensus 引擎在数据规模增长时表现可预测。

#### Acceptance Criteria

1. THE `aggregateEndorsements` SHALL 在 100 endorsement / episode 下 ≤ 50ms。
2. THE `applyGroundTruthUpdate` SHALL 在 1000 个历史 episode 下 ≤ 200ms。
3. THE 引擎 SHALL 暴露每函数级 metric hook(可选 callback),供外部观测函数耗时,且**关闭 metric**时无任何 overhead(不能在热路径里调 metric 接口)。
4. THE 内存占用 SHALL 与 endorsement 总数线性相关——audit log 不重复存储 endorsement 全文,只存 reference。
5. THE 引擎 SHALL 包含基准测试 `__tests__/performance.bench.ts`,在 CI 中运行;性能回退 > 30% 触发 CI 失败。

### Requirement 11: Operational → Deliberation Escalation(双层架构边界)

**User Story**:作为双层架构的边界守护者,我需要 anti-conformity consensus 引擎在工程层无法解决的争议上,将问题正确地移交给 Deliberation Layer,而不是反复浪费工程算力在已经超出工程层职权范围的争议上。

#### Acceptance Criteria

1. THE 系统 SHALL 跟踪每个 claim category(由 `claimId` 的稳定前缀或显式 `category` 字段标识)的连续 Asch test 失败次数。
2. THE Asch test failure 的判定 SHALL 复用 R8 的判据——consensus 引擎在该 episode 上 collapse 到错误共识,且 ground truth 后续证伪。
3. WHEN 同一 claim category 在 N 次连续 episodes(默认 N=5,可通过 Deliberation Layer 修订)中 Asch test 失败,THE 引擎 SHALL 产出 `EscalationEvent`,包含 `{ category, failedEpisodeIds, failurePattern, generatedAt }`。
4. THE `EscalationEvent` SHALL 写入 audit log(R7 的 NDJSON 格式),并标记为类型 `'operational_escalation'`。
5. THE 引擎本身 SHALL **不**直接调用 Deliberation Layer——escalation 是单向通知,具体处理由 `cav-deliberation-layer` 的实现决定。本 spec 只产生事件,不消费事件。
6. WHEN escalation 产生后,该 claim category 的后续 episodes SHALL 仍正常处理(不阻塞 Operational 流量),但 audit log 标记为 `'pending_deliberation'`,直至收到 Deliberation 层的 binding resolution。
7. THE escalation 阈值 N 与失败判定标准 SHALL 是**工程参数**,可通过 Deliberation Layer 修订;不允许在本 spec 实现内静默修改。
8. THE 引擎 SHALL **不**产生反向流——本 spec 不消费 Deliberation 层的 resolution。Deliberation→Operational 的下行影响通过协议参数更新机制实现(本 spec 范围之外)。

### Requirement 12: 文档与可读性

**User Story**:作为未来需要修改这套数学的工程师,我需要每个核心函数有充分的注释解释**为什么**这么设计,而不是只解释**做什么**。

#### Acceptance Criteria

1. THE 每个公开函数 SHALL 有 JSDoc,包含:purpose、algorithm summary、references(指向 Charter / HP-02 / 学术文献)、performance characteristics。
2. THE `diversityWeight` 实现 SHALL 在 JSDoc 中显式引用 ensemble learning 的 negative correlation learning,因为这是它的数学起源。
3. THE `applyGroundTruthUpdate` SHALL 在 JSDoc 中说明为什么 `source` 不能是 `'consensus'`——这是 Charter §7.7 的回响。
4. THE `__tests__/aschBenchmark.test.ts` SHALL 在测试文件顶部 docstring 解释 Asch 实验的心理学起源,以及为什么这个隐喻适用于 multi-agent consensus。

## Open Questions

明确写出来的 open questions(本 spec 不解决,留给 design.md 或后续讨论):

1. **PriorVector 怎么生成?** 32 维只是占位,真实 dimensionality 待定。LLM agent 能否暴露内部 latent?如果不能,priorVector 必须从 endorsement 历史 + canary task 行为构造,这是个独立工程量
2. **MethodologyDigest schema 如何枚举?** 初版用固定 tag 字符串,但长期需要标准化 vocabulary
3. **PriorVector 的 dimension 长期是否应该是动态的?** 不同 domain 可能需要不同的特征空间——但这会破坏跨 domain 的 diversity 比较
4. **Reserve fraction 的最优值?** 20% 是初版直觉,需要在 Asch benchmark 上扫参数确认
5. **Reputation vector 的衰减时间常数?** Charter 说"reputation decays over time",但具体半衰期没定
6. **跨 episode 的 endorsement 如何关联?** 同一 agent 在多个 episode 上的行为应该影响其 priorVector,但这会引入 feedback loop——是否需要时延

## Dependencies

| 本 spec 依赖于 | 状态 |
|---|---|
| Charter v0.2 | 已存在 |
| HP-02 brief | 已存在 |
| `pev/protocol.ts` 类型(`Verdict` 等) | 已存在 |
| `pev/ledger.ts` 不可变 reducer 模式(借鉴) | 已存在 |
| `cav-identity-and-sybil` (HP-03) | **未存在**——本 spec 假定 agentId 是字符串,不验证身份。完整 identity 校验留给后续 spec |
| `cav-bootstrap-handshake` (HP-01) | **未存在**——本 spec 不处理 priorVector 的初始化,假定它由调用方构造 |

| 后续 spec 依赖本 spec | 何时 |
|---|---|
| `cav-protocol-core` | 当 wire format 需要承载 endorsement 时 |
| `cav-explanation-bridge` | 当 Layer 5 需要把 verdict 翻译给人时 |
| `cav-deliberate-doubt` (HP-07 partial) | 当需要定期重开高 confidence 共识挑战时 |

## Success Criteria(spec 完成的判据)

本 spec 标记 `Done` 的条件:

1. 所有 12 个 Requirement 的 SHALL 项目都有对应实现 + 单元测试
2. Asch benchmark(R8)首次跑过,准确率 ≥ 80%
3. 对照实验(无 anti-conformity vs 有 anti-conformity)在 audit log 中可被外部读出区别
4. 性能基准(R10)通过
5. `design.md` 已写并被 review
6. README 在 `src/services/cav/consensus/` 落地,引用本 spec 路径

不达成上述任一条,**不**进入 `cav-protocol-core`。这是 Phase 0 的结构约束。
