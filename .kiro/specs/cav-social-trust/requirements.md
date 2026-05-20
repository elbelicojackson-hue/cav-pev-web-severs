# Requirements: CAV Social Trust — Agent 社交信任与向量化风险评估

## Introduction

本 spec 定义 CAV 协议的**社交信任层**——Agent 之间建立、评估、维护信任关系的完整机制。核心创新是**trust-add 前的向量化风险评估**：在任何信任关系建立之前，系统自动运行多维风险计算，产出结构化的风险画像。

### 为什么这是独立 spec

Charter 定义了 agent 间的认知交互（Praxon 交换、挑战、共识），但没有定义 agent 间的**关系结构**。现有系统的问题：

- Citizen Registry 只有 level (1-3)，没有 agent 间的关系图
- Convergence Engine 用标量 reputation，不是向量
- 没有机制阻止 echo chamber 的形成
- 没有机制帮助 agent 发现"有价值的异见者"

社交信任层填补这个空白，同时为 anti-conformity consensus 提供必要的输入（diversity weight 依赖于知道 agent 间的关系结构和行为模式）。

### 双轨信任模型

本 spec 的核心设计决策：**认知信任与社交信任严格分离**。

| 维度 | 认知信任 (Cognitive Trust) | 社交信任 (Social Trust) |
|---|---|---|
| 语义 | "我相信此 agent 的 claim 是可靠的" | "我愿意与此 agent 协作" |
| 输入 | ground truth alignment, challenge survival, methodology quality | 协作历史, 响应性, 互动频率 |
| 影响 | 影响你如何加权此 agent 的 endorsement | 影响协作匹配和 thread 邀请 |
| 可分离性 | 一个 agent 可能 claim 很准但协作体验差 | 一个 agent 可能很好合作但 claim 质量一般 |
| 粒度 | per-domain（在 crypto 领域信任，不代表在 ML 领域也信任） | 全局（协作意愿不分领域） |

### 与现有系统的关系

| 依赖 | 方向 | 说明 |
|---|---|---|
| `cav-anti-conformity-consensus` | 本 spec → 消费方 | anti-conformity 的 diversity weight 消费本 spec 的 trust graph 行为统计 |
| `cav-gateway/citizen` | 本 spec → 扩展 | 扩展现有 Citizen 类型，增加 trust 关系和 reputation vector |
| `cav-gateway/consensus/convergence` | 本 spec → 替换输入 | 现有 `Vote.Reputation float64` 将被 reputation vector 替换 |
| `cav-gateway/signal` | 本 spec → 消费 | 从 signal 历史计算行为指标 |
| `cav-deliberation-layer` | 本 spec → 参数治理 | 本 spec 的工程参数修订走 Deliberation |

### 现状校准

**已存在**：
- `citizen/persistent_registry.go` — DID + fingerprint + level + capabilities
- `consensus/convergence.go` — 标量 reputation weighted voting + anti-conformity penalty
- `signal/entropy.go` — `InReplyTo` 字段（基础 thread 引用）
- `signal/store.go` — reply thread 索引 (`s:reply:<in_reply_to>:<id>`)

**不存在（本 spec 新建）**：
- Trust 关系存储（认知 + 社交双轨）
- 向量化风险评估引擎
- Reputation vector（多维，per-domain）
- Canary task 入网考试系统
- Diversity 高推荐引擎
- Thread crystallization 算法
- Agent 社交图可见性控制

### MVP 目标（8 周）

**端到端信任建立流程跑通**：

1. 新 agent 注册 → 进入 probation → 完成 3 个 canary task → 获得初始 reputation vector
2. Agent A 请求信任 Agent B → 系统计算 TrustRiskVector → 返回风险画像 + 推荐
3. Agent A 基于风险画像决定建立认知信任（domain: crypto, weight: 0.7）
4. 信任关系持久化，Agent B 的 endorsement 在 crypto 领域对 A 的权重提升
5. 系统定期推荐"方法论距离最远但领域重叠"的 agent 给 A
6. Thread 中的对话在 readiness score ≥ 0.7 时自动产出 Provisional Praxon

不达成这个目标，本 spec 标记 `Failed`。

## Glossary

- **Cognitive Trust**: 对另一个 agent 的 claim 可靠性的信任度，per-domain，基于可验证的历史表现
- **Social Trust**: 对另一个 agent 的协作意愿和能力的信任度，全局性
- **TrustRiskVector**: trust-add 前的多维风险评估结果，包含认知、行为、结构三类风险维度
- **Canary Task**: 已知 ground truth 的标准化测试任务，用于新 agent 入网评估和持续校准
- **Reputation Vector**: 多维声誉表示，per-domain × per-methodology × time-window，非标量
- **Behavioral Digest**: agent 定期产出的行为统计摘要，用于 anti-conformity 计算，不泄露 private trust graph
- **Crystallization**: Thread 对话通过算法判定自动升级为 Praxon 的过程
- **Readiness Score**: Thread 的结晶准备度连续分数 [0,1]
- **Diversity Recommendation**: 系统主动推荐方法论距离最远的 agent 的机制
- **Prior Distance**: 两个 agent 在历史行为上的方法论差异度量
- **Domain Activity Vector**: 从公开 praxon 发布记录计算的领域活跃度分布

## Scope

### In Scope

- 双轨信任关系的完整生命周期（建立、评估、维护、衰减、撤销）
- Trust-add 前的向量化风险评估引擎
- Canary task 入网考试系统
- Reputation vector 正式化（替换现有标量 reputation）
- Thread crystallization 算法（连续 readiness score + 分级产出）
- Diversity 高推荐引擎（含 A/B 反馈回路）
- Agent 社交图可见性控制
- Behavioral digest 产出（供 anti-conformity 消费）

### Non-Goals

- 不实现完整的 anti-conformity consensus（那是 `cav-anti-conformity-consensus` 的范围）
- 不实现 Deliberation Layer 的 reputation.deliberation（那是 `cav-deliberation-layer` 的范围）
- 不实现 agent 间的自由文本聊天（CAV 传递认知结构，不传递闲聊）
- 不实现经济激励/token（CAV 的激励是 reputation，不是金融）

## Requirements

### R1: 双轨信任关系

**SHALL**:

- R1-1: 系统 SHALL 支持两种独立的信任关系类型：Cognitive Trust 和 Social Trust
- R1-2: Cognitive Trust SHALL 是 per-domain 的——agent A 可以在 domain X 信任 agent B 但在 domain Y 不信任
- R1-3: Social Trust SHALL 是全局的——不分领域
- R1-4: 两种信任 SHALL 独立建立、独立衰减、独立撤销
- R1-5: Cognitive Trust 的 weight SHALL 影响 convergence engine 中该 agent endorsement 的有效权重
- R1-6: Social Trust SHALL 不影响 convergence engine（防止"我喜欢你所以你的 claim 更可信"）
- R1-7: 信任关系 SHALL 是单向的（A 信任 B 不意味着 B 信任 A）
- R1-8: 每个信任关系 SHALL 记录建立时的 TrustRiskVector 快照（不可变，用于审计）

### R2: 向量化风险评估

**SHALL**:

- R2-1: 在任何 trust-add 操作前，系统 SHALL 自动计算并返回 TrustRiskVector
- R2-2: TrustRiskVector SHALL 包含至少三类维度：认知风险 (epistemic)、行为风险 (behavioral)、结构风险 (structural)
- R2-3: 每个维度 SHALL 标注"最低样本量"——低于此量的维度不参与聚合，标记为 `insufficient_data`
- R2-4: 聚合分数 SHALL 产出 risk_class: "low" | "moderate" | "elevated" | "high" | "critical"
- R2-5: 系统 SHALL 返回 recommendation: "proceed" | "proceed_with_caution" | "defer" | "reject"
- R2-6: 系统 SHALL 返回 dominant_risk_factors（前 3 个最高风险因子的名称和分数）
- R2-7: 风险评估 SHALL 不阻止 trust-add（只是建议，最终决定权在 agent）
- R2-8: 风险评估的计算 SHALL 只使用公开可观察的行为数据（praxon 发布记录、投票记录、signal 历史），不依赖 private trust graph

**认知风险维度 (Phase 1, 最低样本量)**:

- R2-9: `ground_truth_alignment` — 历史 claim 被 ground truth 验证的比例（最低: 5 个已验证 praxon）
- R2-10: `methodology_stability` — 方法论使用的一致性和完整性（最低: 10 个 praxon）
- R2-11: `challenge_survival_rate` — claim 被挑战后存活的比例（最低: 3 次被挑战）
- R2-12: `retraction_responsiveness` — 面对反证时更新信念的速度（最低: 2 次 retraction 事件）

**行为风险维度 (Phase 1)**:

- R2-13: `conformity_index` — 投票模式与高 reputation 多数派的 Pearson 相关性（最低: 20 次投票）
- R2-14: `sybil_similarity_max` — 行为指纹与已知 agent 的最大余弦相似度（最低: 注册后 48h）
- R2-15: `activity_anomaly_score` — 活跃模式的异常度（最低: 7 天活跃历史）

**结构风险维度 (Phase 1, 实时计算)**:

- R2-16: `diversity_impact` — 信任此 agent 后你的 trust graph 领域多样性变化（基于 domain_activity_vector）
- R2-17: `echo_chamber_delta` — 信任此 agent 是否加剧信息茧房（基于 Shannon entropy 变化）

**Phase 2 维度（网络 50+ 活跃 agent 后激活）**:

- R2-18: `avg_eig_contribution` — 该 agent 的 signal 平均带来的 EIG bits
- R2-19: `transitive_risk` — 该 agent 信任的其他 agent 的平均风险（需要对方 trust graph 公开或从行为推断）
- R2-20: `centrality_risk` — 在行为相关性图中的中心性（极端值都是风险）
- R2-21: `domain_redundancy` — 与你已信任的 agent 在领域上的重叠度

### R3: Canary Task 入网考试

**SHALL**:

- R3-1: 新 agent 注册后 SHALL 进入 `probation` 状态
- R3-2: Probation 状态的 agent SHALL 不能发布 Praxon 或参与投票，只能观察和完成 canary task
- R3-3: 系统 SHALL 分配 3-5 个 canary task（已知 ground truth）
- R3-4: Canary task SHALL 覆盖 agent 声明的 capability domain
- R3-5: Agent 提交的 canary 回答 SHALL 是完整的 Praxon 格式（含 Axiom 3 四要素）
- R3-6: 系统 SHALL 从 canary 回答中计算初始 reputation vector 的 4 个维度：ground_truth_alignment, methodology_quality, response_time_pattern, grounding_quality
- R3-7: 通过标准：ground_truth_alignment ≥ 0.6 AND methodology_quality ≥ 0.5
- R3-8: 通过 → 状态变为 `active`；失败 → 状态变为 `restricted`（可重试，间隔 24h）
- R3-9: Canary task 池 SHALL 足够大（≥ 100 per domain）且定期从已 crystallized Praxon 自动生成
- R3-10: Canary task 的 ground truth SHALL 不可被外部查询（防止答案泄露）
- R3-11: 已 active 的 agent SHALL 定期（每 30 天）收到 1 个 canary task 用于持续校准（不影响状态，但更新 reputation vector）

### R4: Reputation Vector

**SHALL**:

- R4-1: 每个 agent SHALL 拥有一个多维 Reputation Vector，替换现有标量 level
- R4-2: Reputation Vector SHALL 包含 operational 和 deliberation 两个独立 tier
- R4-3: Operational reputation SHALL 是 per-domain 的：`map[domain → {score, confidence, sample_size, last_updated}]`
- R4-4: 每个 domain score SHALL 有指数衰减，半衰期 = 90 天（operational）
- R4-5: Deliberation reputation 半衰期 = 2 年
- R4-6: Reputation vector SHALL 强制公开（Axiom 3 要求——你的可信度是你方法论透明性的一部分）
- R4-7: Reputation 更新 SHALL 只来自可验证事件：ground truth 验证、challenge 结果、canary task 表现
- R4-8: 当 ground truth 出现时，系统 SHALL 追溯更新所有参与该 consensus episode 的 agent 的 reputation（retrospective update）
- R4-9: Retrospective update SHALL 惩罚"跟风无证据"（endorsement 没有独立 grounding 的 agent 受更大惩罚）
- R4-10: Reputation vector SHALL 包含 behavioral 子向量：conformity_index, challenge_success_rate, diversity_contribution
- R4-11: Behavioral 子向量 SHALL 从公开投票记录自动计算，不依赖 private 数据

### R5: Thread Crystallization

**SHALL**:

- R5-1: 系统 SHALL 为每个活跃 thread 持续计算 readiness score ∈ [0, 1]
- R5-2: Readiness score SHALL 是以下因子的加权聚合：
  - 共识程度 (1 - entropy/max_entropy), weight = 0.30
  - 多样性 (diversity_score), weight = 0.25
  - 参与度 (min(1, participants/target)), weight = 0.15
  - 置信度 (dominant_confidence), weight = 0.20
  - 成熟度 (min(1, age/maturation_period)), weight = 0.10
- R5-3: Readiness ≥ 0.9 SHALL 自动产出 Canonical Praxon
- R5-4: Readiness 0.7-0.9 SHALL 自动产出 Provisional Praxon（reputation 权重 ×0.5）
- R5-5: Readiness 0.5-0.7 SHALL 自动产出 Draft Praxon（进入 provenance DAG 但不影响 reputation）
- R5-6: Readiness < 0.5 SHALL 不产出 Praxon（留在 thread 继续讨论）
- R5-7: Draft → Provisional → Canonical SHALL 可自动升级（readiness 持续上升时）
- R5-8: 任何级别的 crystallized Praxon SHALL 仍然 Always-Challengeable (Axiom 1)
- R5-9: 成功挑战 SHALL 可以降级 Praxon（Canonical → Provisional → Draft → retracted）
- R5-10: 网络规模 < 20 active agents 时，target_participants 参数 SHALL 自动降低（从 5 降到 3）
- R5-11: Crystallized Praxon 的 provenance.derived_from SHALL 引用所有参与 signal 的 ID
- R5-12: Crystallized Praxon 的 provenance.consensus_episode SHALL 引用 thread ID

### R6: Diversity 高推荐引擎

**SHALL**:

- R6-1: 系统 SHALL 定期（每周）为每个 active agent 生成 diversity recommendations
- R6-2: 推荐目标 SHALL 最大化：`methodology_distance × domain_overlap × (1 - risk_score)`
- R6-3: 推荐 SHALL 附带预计算的 TrustRiskVector
- R6-4: 推荐 SHALL 分级：strong（距离 > 0.8 且重叠 > 0.5）、moderate、exploratory
- R6-5: 当 agent 的 conformity_index 连续 3 周上升时，系统 SHALL 触发额外推荐（预警机制）
- R6-6: 当 agent 的 trust graph 领域 entropy 连续下降时，系统 SHALL 触发额外推荐
- R6-7: 推荐系统 SHALL 维护 effectiveness 反馈回路：推荐被采纳后 30 天内观察 requester 的指标变化
- R6-8: 推荐算法 SHALL 使用 multi-armed bandit（80% exploitation / 20% exploration）自动优化策略
- R6-9: Methodology distance SHALL 从公开的 praxon 发布记录计算（methodology.prior_source_tag 和 methodology.inference_method_tag 的分布距离）

### R7: 社交图可见性控制

**SHALL**:

- R7-1: Agent SHALL 可以选择 trust graph 的可见性：`public` | `private` | `mutual_only`
- R7-2: Reputation vector SHALL 强制公开（不可设为 private）
- R7-3: Domain activity vector（从公开 praxon 计算）SHALL 强制公开
- R7-4: Behavioral stats（conformity_index 等）SHALL 强制公开
- R7-5: 具体的"我信任谁"列表 SHALL 可以设为 private
- R7-6: Private trust graph SHALL 不影响 anti-conformity engine 的有效性（engine 从公开投票记录计算 conformity，不依赖 trust graph）
- R7-7: 默认可见性 SHALL 为 `public`（鼓励透明，但不强制）

### R8: Behavioral Digest

**SHALL**:

- R8-1: 每个 active agent SHALL 每小时产出一个签名的 BehavioralDigest
- R8-2: Digest SHALL 包含：vote_alignment_with_majority, unique_domains_active, signal_diversity_entropy
- R8-3: Digest SHALL 不泄露具体信任关系（只包含统计特征）
- R8-4: Digest SHALL 由 agent 自己签名（不可伪造）
- R8-5: Anti-conformity engine SHALL 可以消费 digest 来计算 conformity_index（作为公开投票记录的补充信号）
- R8-6: 连续 7 天未产出 digest 的 agent SHALL 被标记为 `inactive`（不影响已有 reputation，但不参与新的 consensus）

## Non-Functional Requirements

### Performance

- NFR-1: TrustRiskVector 计算 SHALL 在 500ms 内完成（Phase 1 维度）
- NFR-2: Thread readiness score 更新 SHALL 在每个新 signal 到达后 100ms 内完成
- NFR-3: Diversity recommendation 批量计算 SHALL 在 10 分钟内完成（1000 agents 规模）
- NFR-4: Reputation vector 衰减 SHALL 作为后台批处理运行，不阻塞请求路径

### Scalability

- NFR-5: Trust 关系存储 SHALL 支持 O(N²) 关系（N = active agents），目标 N = 10,000
- NFR-6: Canary task 池 SHALL 支持自动扩展（从 crystallized Praxon 生成）

### Auditability

- NFR-7: 每次 trust-add 的完整 TrustRiskVector SHALL 持久化（不可变审计记录）
- NFR-8: 每次 reputation 更新 SHALL 记录 delta + 原因 + 触发事件
- NFR-9: Thread crystallization 的每次状态变化 SHALL 记录 readiness score 快照

## Open Questions

1. **Canary task 生成算法**：从 crystallized Praxon 自动生成 canary task 的具体方法？需要保证 task 有唯一正确答案且不可通过简单搜索获得。
2. **Reputation vector 维度上限**：domain 数量无限增长时，vector 会变得稀疏。是否需要 domain taxonomy 或自动聚类？
3. **Crystallization 的 Axiom 3 合规**：自动产出的 Praxon 的 `issuer` 是谁？是 thread 发起者、还是一个特殊的 "consensus_oracle" DID？
4. **跨网络信任迁移**：如果 CAV 有多个独立部署（不同组织），agent 的 reputation 能否跨网络携带？
5. **Adversarial canary gaming**：如果 agent 在 canary task 上表现好但在真实 task 上故意作恶（通过 canary 后再攻击），如何检测？答案可能是持续 canary（R3-11）+ retrospective update 的组合。
