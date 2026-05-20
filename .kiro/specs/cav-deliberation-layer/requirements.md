# Requirements: CAV Deliberation Layer — 学术性重大决策协议

## Introduction

本 spec 定义 CAV 协议的**慢层**——Deliberation Layer——专责处理协议级、影响广、不可轻易回滚的决策。它与 Operational Layer(`cav-anti-conformity-consensus`)严格解耦,运行在不同的延迟、对称性、共识纪律之下。

### 为什么这是独立 spec

Charter v0.3 §10 立了双层架构的结构性承诺:Operational 解决日常,Deliberation 解决协议根。两层不能共用同一套机制——这不是工程偏好,是结构必然(详见 charter §10.2)。

所有需要**外部学术认可**、**全网广泛影响**、或**本身就在修改协议规则**的决策,必须走 Deliberation:

- Charter Axiom 的修订或重新解释
- §11 amendment process(它的程序形态本身就是 Deliberation)
- HP-07 deliberate-doubt 调度参数
- HP-01 founding canary set 内容
- HP-02 ground-truth source 枚举的扩展
- 工程参数(adversarial reserve fraction、Asch 阈值、reputation decay 半衰期等)的全网修订

### 与 Operational Layer 的对照

| 维度 | Deliberation(本 spec) | Operational(`cav-anti-conformity-consensus`) |
|---|---|---|
| Agent 触发 | 对称、平等 | 异质、角色绑定 |
| 共识机制 | reputation-weighted vote + diversity threshold | diversity-weighted aggregation + adversarial reserve |
| 时间窗口 | 30 天 review(默认) | 秒-分钟 |
| 吞吐 | 每月数十个 motion 上限 | 持续高频 |
| Reputation 输入 | `reputation.deliberation`(慢变) | `reputation.operational`(快变) |
| 失败代价 | 协议根受损 | 单 task 质量下降 |
| 学术合法性 | 必须可被外部 review | Asch test 工程验证 |

### 现状校准

- 本 spec 是**完全 green-field**——既无对应代码,也无前置 spec
- 但 Charter §10 + §11 已经勾出了 Deliberation 的轮廓,本 spec 是 §10 的具体落地
- 与 `cav-knowledge-capsule` 的 capsule_class 字段对接(`'deliberation_motion'`、`'deliberation_resolution'`)
- 与 `cav-anti-conformity-consensus` 的 `EscalationEvent` 接口对接(消费方)

### MVP 目标(12 周,P1 优先级)

**单 motion 全流程跑通**:

- 一个 citizen agent 提交 motion(修改某个工程参数,例如 Asch 阈值)
- 30 天 review 窗口开放,其他 citizen 可以 endorse / oppose / amend
- 投票期结束,reputation-weighted vote 产生 verdict
- Verdict 持久化为 `deliberation_resolution` capsule
- 14 天 grace period 后,该参数在 Operational 层生效

不达成这个目标,Charter §10 和 §11 在工程上是空话。

## Glossary

- **Motion**:一个 Deliberation 提案的初始 capsule,提出对协议级事项的修改请求
- **Resolution**:Motion 走完流程后的最终决议 capsule,binding 于 Operational Layer
- **Sponsor**:发起 motion 的 citizen,需消耗 sponsor stake(防 motion 灌水)
- **Endorsement / Opposition**:其他 citizen 对 motion 的投票表态,带 reputation.deliberation 权重
- **Amendment(in-flight)**:在 motion 进行期间提出的修订,需要重新进入 review 窗口(类似立法程序的"修正案")
- **Diversity Threshold**:resolution 有效的最低多元性要求——必须有 ≥ N 个独立 issuer 投票,否则 motion 自动 inconclusive
- **Grace Period**:resolution 通过到 Operational 实际执行的间隔,允许实现层完成更新
- **Deliberation Reputation**:`reputation.deliberation` 向量,长时间常数,与 Operational 严格分离
- **Constitutional Sensitivity**:motion 修改对象的"宪法敏感度"等级,从 axiom-level(最敏感)到 parameter-level(可调参数)
- **Anti-Weakening Demonstration**:motion sponsor 必须随 motion 提交的形式化论证,证明该提案不削弱 Charter 的任意一条 Axiom

## Scope and Non-Goals

### In Scope

- Motion 的 wire schema(扩展 `cav-knowledge-capsule` 的 `'deliberation_motion'` capsule_class)
- Resolution 的 wire schema(`'deliberation_resolution'`)
- 30 天 review 窗口的 state machine
- Reputation-weighted vote 数学(diversity threshold + reputation gating)
- Anti-weakening demonstration 的形式化要求与验证算法
- Deliberation reputation 向量的更新规则(长时间常数,慢变)
- Operational → Deliberation 的 escalation 消费(从 anti-conformity spec 接 `EscalationEvent`)
- Deliberation → Operational 的 binding force 协议(grace period + 实现迁移检查点)
- Sponsor stake 机制
- 单 motion MVP demo

### Out of Scope(留给后续 spec 或其他层)

- Motion **内容**的具体集合(本 spec 是流程,不是法典)
- 跨 process / 跨网络的 motion 路由(走 `cav-protocol-core`)
- DID 完整生态(走 `cav-identity-and-sybil`,即 HP-03)
- Motion 撤回机制(初版 motion 一经提交不可撤回,作者放弃就走自然 expiry)
- Constitutional 法庭(本 spec 不引入司法分支——所有争议通过 motion + 反对 motion 的辩证解决)
- Anti-conformity 的具体 ccbteam 实现(那是 Operational 层的事)

本 spec **故意不做** Constitutional 法庭。理由:任何"超越 motion 的最终裁决者"都会成为 §1 mission 警告的"epistemic priesthood"。Deliberation 必须保持开放、对抗、永远可被新 motion 翻案。

## Requirements

### Requirement 1: Motion Capsule Schema

**User Story**:作为想要发起 Deliberation 的 citizen,我需要一份明确 schema 的 motion capsule 格式,这样我的提案能被全网 parse、verify、投票。

#### Acceptance Criteria

1. THE 系统 SHALL 扩展 `cav-knowledge-capsule` 的 capsule schema,新增 `capsule_class: 'deliberation_motion'` 类型。
2. THE motion capsule 在标准 capsule 字段之外 SHALL 包含子字段 `motion`,内容为:
   - `target`:被修改对象的明确引用(`{ scope: 'axiom' | 'parameter' | 'founding_set' | 'ground_truth_source', ref: string, current_value: any, proposed_value: any }`)
   - `rationale`:修改理由的结构化论证(包含 `motivating_evidence: GroundingHandle[]`,引用 Operational 层证据)
   - `anti_weakening_demonstration`:见 R3
   - `constitutional_sensitivity`:`'axiom' | 'protocol_constraint' | 'engineering_parameter' | 'implementation_choice'`(对应 Charter §11.x 提到的工程参数分层)
   - `expected_grace_period_days`:可选,sponsor 建议的 Operational 层执行 grace period
3. THE motion 的 sponsor 字段 SHALL 直接复用 capsule 的 `issuer` 字段,无需额外字段。
4. WHEN motion 的 `target.scope === 'axiom'`,THE motion SHALL 自动被标记为 fork candidate,要求加倍的 diversity threshold(见 R5)。
5. WHEN motion 的 `target.scope` 不在枚举内,验证器 SHALL 拒绝该 motion(`UNKNOWN_TARGET_SCOPE`)。
6. THE motion capsule 的 `provenance.derived_from` SHALL 必须 reference 至少一个 escalation event 或具体 evidence capsule——**纯口头提议**(无证据基础)在协议层非法。
7. THE motion 的整体大小限制 SHALL 复用 `cav-knowledge-capsule` 的 256 KB 上限。

### Requirement 2: Resolution Capsule Schema

**User Story**:作为 Operational 层的实现者,我需要一份明确的 resolution capsule 格式,这样我能机器读取并自动应用 binding force。

#### Acceptance Criteria

1. THE 系统 SHALL 扩展 capsule schema,新增 `capsule_class: 'deliberation_resolution'` 类型。
2. THE resolution capsule 在标准字段外 SHALL 包含子字段 `resolution`:
   - `motion_ref`:被裁决的 motion capsule_id
   - `outcome`:`'approved' | 'rejected' | 'inconclusive' | 'expired'`
   - `vote_breakdown`:`{ endorsements: number, oppositions: number, abstentions: number, totalDeliberationReputation: number, diversityScore: number }`
   - `effective_at`:绝对时间戳,binding force 生效时间(= resolution 时间 + grace period)
   - `binding_scope`:被影响的 Operational 实现 spec 列表
   - `audit_trail`:完整投票记录的 reference,可被独立审计
3. THE resolution 的 outcome 判定 SHALL 在 R5 详细定义。
4. THE resolution capsule 的 issuer SHALL 是协议级的 deliberation oracle 身份(见 R10)——单 citizen 不能"宣布" resolution。
5. WHEN motion 在 review 窗口结束时未达到 diversity threshold,THE outcome SHALL 是 `'inconclusive'`,后续可由新 motion 重启,不允许直接重投。
6. THE resolution capsule 的 `provenance.derived_from` SHALL 引用 motion capsule + 所有有效 vote capsule。

### Requirement 3: Anti-Weakening Demonstration

**User Story**:作为 Charter §10 amendment process 的守护者,我需要每个 motion 自带形式化论证,说明它**不削弱**任何 Axiom——否则 motion 通过 = Charter 被偷偷 fork。

#### Acceptance Criteria

1. THE motion capsule 的 `anti_weakening_demonstration` 字段 SHALL 是非空对象,结构:
   - `axioms_in_scope`:列出所有可能受影响的 Axiom(Charter §2 的 4 条)
   - `per_axiom_argument`:对每条相关 Axiom,提供 `{ axiom: string, claim: 'unaffected' | 'strengthened' | 'preserved', justification: string, falsifiable_test?: string }`
2. WHEN motion 的 `target.scope === 'axiom'`,THE demonstration SHALL **不**允许任何 Axiom 标记为 `'weakened'`——任何这种声明视为 fork,自动拒绝该 motion(`AXIOM_WEAKENING_FORK_REJECTED`)。
3. THE demonstration 的 `falsifiable_test` 字段 SHALL 描述一个**可在 Operational Layer 跑的实验**,验证 demonstration 的论证。
4. THE 验证器 SHALL **不**判断 demonstration 的实质对错——那是 deliberation 投票本身要做的事。验证器只检查 schema 完整性。
5. THE 实质性的 demonstration 评估 SHALL 由参与投票的 citizen 完成,作为它们投票决策的输入。
6. WHEN demonstration 的某 axiom 论证含 `falsifiable_test`,sponsor SHALL 在 review 窗口内提供该测试的 Operational 跑通证据(以 grounding handle 引用)。否则该 motion 自动降级为 `'inconclusive'`。

### Requirement 4: Review Window State Machine

**User Story**:作为协议时序的设计者,我需要 motion 的生命周期被严格约束在一个 state machine 中,这样所有节点对 motion 的状态判断一致。

#### Acceptance Criteria

1. THE motion 的状态 SHALL 是以下枚举之一:`'open'`、`'amended'`、`'voting'`、`'closed_approved'`、`'closed_rejected'`、`'closed_inconclusive'`、`'expired'`。
2. THE 默认时序 SHALL 是:
   - **0 ~ 14 天(open)**:接受 endorsement / opposition / amendment 提议
   - **第 14 天(amendment cutoff)**:不再接受新 amendment;若有 amendment 则状态变 `'amended'` 并重置时钟
   - **14 ~ 30 天(voting)**:仅接受 endorsement / opposition / abstention,不接受新意见
   - **第 30 天**:state machine 计算 outcome(R5)并产出 resolution
3. THE amendment 出现时 SHALL 重置整个 30 天时钟(因为 motion 内容已变),但累计 amendment 次数 ≤ 3——超过则该 motion 自动 expire,sponsor 必须重提。
4. THE state transitions SHALL 是单向的(除 amendment 重置例外)。
5. THE 时间常数(14 / 30 / 3 次)是**工程参数**,可被未来的 motion 修订(本 spec 不在 axiom 层冻结这些数字)。
6. WHEN motion 在 voting 阶段达到 R5 的 supermajority 提前结束条件(详见 R5),THE state machine 可提前关闭 motion(early close),无需等满 30 天。

### Requirement 5: Reputation-Weighted Vote 数学

**User Story**:作为 Deliberation 共识机制的实现者,我需要一个明确、可被外部审计的投票算法,且该算法不能被短期 reputation 刷高的攻击者绕过。

#### Acceptance Criteria

1. THE 系统 SHALL 提供纯函数 `tallyDeliberationVote(motion, votes, citizenReputations) → ResolutionVerdict`。
2. THE 每张有效 vote 的权重 SHALL = `voter.reputation.deliberation`(不引用 operational reputation)。
3. THE diversity threshold:resolution 有效的必要条件 SHALL 是:
   - 至少 N_distinct = 9 个独立 issuer 投了 endorse 或 oppose(默认 9,可调,axiom-class motion 加倍至 18)
   - 投票 issuer 的 priorVector(若可获得)的余弦相关性 std-dev ≥ 0.2(防止表面多样,实际是同一个 prior 阵营)
   - **Bootstrap exception**:当网络 citizen 总数 < 18 时,diversity threshold 降为 `ceil(total_citizens / 2)`,最低 3。此例外在网络达到 18 citizen 后自动失效,不需要 motion 撤销
4. THE supermajority 提前关闭条件:`approved` 加权票 ≥ 2× `opposed` 加权票,且 diversity threshold 已满足,且自 voting 阶段开始 ≥ 7 天 → 提前 close 为 `closed_approved`。
5. THE outcome 判定:
   - 满足 diversity threshold + `approved_weight > opposed_weight × 1.5` → `'approved'`
   - 满足 diversity threshold + `opposed_weight > approved_weight × 1.5` → `'rejected'`
   - 满足 diversity threshold,但加权票数差距不足 1.5× → `'inconclusive'`
   - 不满足 diversity threshold(到时间窗口结束)→ `'inconclusive'`
6. THE axiom-class motion 的判定阈值 SHALL 加严:`approved_weight > opposed_weight × 2.0`(而非 1.5)。
7. THE 投票算法 SHALL 是纯函数;给定相同输入返回相同 verdict。
8. THE 投票算法 SHALL 在 1000 票输入下 ≤ 200ms。

### Requirement 6: Deliberation Reputation 向量

**User Story**:作为长期 reputation 系统的设计者,我需要 deliberation reputation 与 operational 严格分离,以防短期工程套利捕获协议根。

#### Acceptance Criteria

1. THE 系统 SHALL 定义类型 `DeliberationReputation`,与 `cav-anti-conformity-consensus` 的 operational reputation 是**分立结构**,不可互相赋值。
2. THE 默认 deliberation reputation 起始值 SHALL = 0(新 citizen 在 Deliberation 层无投票权,直至获得首份)。
3. THE deliberation reputation 的更新源 SHALL 限于以下事件:
   - 成功 sponsor 一个 `'approved'` motion → +ΔR_sponsor
   - 在 motion voting 中投票一致于最终 outcome(approved 时投 endorse,rejected 时投 oppose)→ +ΔR_correct
   - 投票一致于反向(approved 时投 oppose,rejected 时投 endorse)→ -ΔR_wrong(且 |ΔR_wrong| > ΔR_correct,惩罚强于奖励)
   - 持续 6 个月 operational reputation 中位数高于阈值 → +ΔR_pipeline(从 operational 流入 deliberation 的唯一通道)
4. THE deliberation reputation 的衰减 SHALL 是慢的(默认半衰期 2 年),与 operational 的天级衰减形成对比。
5. THE `reputation.deliberation` 与 `reputation.operational` SHALL **永远**分立存储,任何 API 试图把两者混合或互转 SHALL 在协议层被拒绝(`CROSS_TIER_REPUTATION_LEAK`)。
6. THE 实现 SHALL 包含审计:每次更新留下 `{ reason, motionId?, episodeId?, delta, before, after }`。
7. WHEN ΔR_sponsor 把 reputation 抬到任意阈值,系统 SHALL 留下显式 audit entry,使外部审计可追踪 deliberation power 的累积路径。

### Requirement 7: Sponsor Stake 机制

**User Story**:作为防止 motion 灌水的设计者,我需要 sponsor 在提交 motion 时承担成本,且这个成本在 motion 失败时被部分削减。

#### Acceptance Criteria

1. THE 提交 motion SHALL 要求 sponsor 锁定 stake,数量与 `constitutional_sensitivity` 关联:
   - `engineering_parameter` → 1× base stake
   - `protocol_constraint` → 4× base stake
   - `axiom` → 16× base stake
2. THE stake 的 medium 与 HP-03(`cav-identity-and-sybil`)对齐——具体是 capital / compute work / 其他,由 deployment 决定;本 spec 只要求 stake 存在且可被 slash。
3. THE motion outcome 决定 stake 处置:
   - `'approved'` → stake 全额退还
   - `'rejected'` → stake 退还 50%,50% 进入 Deliberation 公共池
   - `'inconclusive'` → stake 退还 80%
   - `'expired'`(amendment 超 3 次)→ stake 退还 30%
4. THE Deliberation 公共池的用途 SHALL 在未来 motion 中决定,本 spec 不预设(避免成为隐含的中央权力)。
5. THE stake 锁定与释放 SHALL 是协议层强制——不依赖任何外部信用机制。

### Requirement 8: Operational → Deliberation Escalation 消费

**User Story**:作为 Deliberation 层与 Operational 层的对接点,我需要消费 anti-conformity spec 产生的 `EscalationEvent`,将其转为可被 Deliberation 处理的 motion 候选。

#### Acceptance Criteria

1. THE 系统 SHALL 提供函数 `ingestEscalation(event) → MotionCandidate | null`。
2. THE Escalation event 进入 Deliberation 后 SHALL **不**自动成为 motion——它进入"escalation pool",等待某个 citizen 自愿 sponsor。这是关键设计:Deliberation 层永远是 pull-based,不允许 Operational 层 push 出强制 motion。
3. WHEN 一个 escalation event 在 escalation pool 中超过 90 天无人 sponsor,THE 系统 SHALL 标记为 `'cold'` 并归档,但保留其 audit trail 以备未来引用。
4. THE escalation pool SHALL 公开可查,任何 citizen 都能看到积压的 escalation。
5. WHEN citizen sponsor 一个 escalation 转 motion,该 motion 的 `provenance.derived_from` SHALL 包含原 escalation event。
6. THE escalation 消费 SHALL 是单向的——本 spec 不向 anti-conformity spec 反向通知 escalation 的处理状态。Operational 层通过观察 resolution capsule 自行获知。

### Requirement 9: Deliberation → Operational Binding Force

**User Story**:作为 Operational 层的实现者,我需要明确知道一个 resolution 何时、如何对我生效。

#### Acceptance Criteria

1. THE resolution 的 `effective_at` SHALL = resolution 时间 + grace period(默认 14 天,sponsor 可在 motion 中建议不同值,实际值由 vote outcome 锁定)。
2. THE Operational 层实现 SHALL 在 `effective_at` 之后开始遵守 resolution 内容——更早开始遵守是允许的(early adopter 行为),但不能更晚。
3. THE binding force 的范围 SHALL 限于 resolution.binding_scope 列出的 Operational spec——resolution 不能"暗中"影响未列出的范围。
4. WHEN Operational 层实现在 `effective_at` 之后仍不遵守 resolution,该实现的 `reputation.operational` SHALL 在下一个 retrospective update 中被惩罚。本 spec 不强制具体惩罚力度——这是 anti-conformity spec 的范围。
5. THE 实现 SHALL 提供 `queryActiveResolutions(asOf?: timestamp) → DeliberationResolution[]`,Operational 层实现可定期查询当前生效的 resolution。
6. WHEN 两个 resolution 冲突(后者修改前者已修改的目标),THE 后者 effective_at 之后 SHALL 覆盖前者,审计日志保留完整链路。

### Requirement 10: Deliberation Oracle Identity

**User Story**:作为协议透明性的关心者,我需要 resolution capsule 的 issuer 不是某个具体 citizen,而是一个协议级 oracle 身份,这样 resolution 的合法性来自流程而非个人。

#### Acceptance Criteria

1. THE 系统 SHALL 定义协议级 oracle 身份 `DeliberationOracle`,以特殊 DID 形式表示(`did:cav-deliberation:<network-id>`)。
2. THE Oracle 私钥 SHALL **不**由任何单一 citizen 持有——MVP 阶段用阈值签名(threshold signature)模拟,生产阶段需要 MPC 或类似机制。
3. THE Oracle 仅在以下事件下签发 capsule:
   - resolution capsule 产出
   - escalation pool 状态变更(冷归档)
   - 协议级 audit summary(可选)
4. THE Oracle SHALL **不**是单一信任源——它的签名只是"流程已正确走完"的证明,不是 resolution 内容正确性的背书。内容正确性靠 Always-Challengeable(Charter Axiom 1)。
5. WHEN Oracle 私钥被怀疑泄露,该 oracle SHALL 通过 motion 流程被替换——Oracle 替换不能由任何单一 citizen 单方决定。
6. THE MVP 实现可以接受简化的 oracle(单实例,运行在公开可审计的服务器上),但**完整生产**实现 SHALL 用阈值签名,且阈值方案本身经过 Deliberation 流程批准。

### Requirement 11: Audit Log

**User Story**:作为协议透明性的关心者,我需要 Deliberation 层的所有事件被完整记录,可外部审计。

#### Acceptance Criteria

1. THE 系统 SHALL 提供 `DeliberationAuditEntry` 类型,事件类型至少包括:`'motion_submitted'`、`'amendment_submitted'`、`'vote_cast'`、`'resolution_emitted'`、`'escalation_ingested'`、`'escalation_archived'`、`'reputation_updated'`、`'stake_slashed'`、`'oracle_signature'`。
2. THE audit log SHALL 是 append-only 的 NDJSON,与 anti-conformity / capsule audit log 格式一致。
3. THE 公开访问:任何 citizen SHALL 能查询整个 Deliberation audit log——这是协议透明性的硬要求。
4. THE 查询接口 SHALL 支持按 motion_id、citizen_did、event_type、time_window 过滤。
5. WHEN 发现 audit log 被篡改(hash 链断裂),所有 Operational 实现 SHALL 拒绝消费此后的 resolution,直至 audit log 被 deliberation 流程修复。

### Requirement 12: 单 Motion MVP Demo

**User Story**:作为本 spec 的最终判据,我需要一个端到端跑通的 demo,从 motion 提交到 resolution 生效。

#### Acceptance Criteria

1. THE 项目 SHALL 包含 `examples/single-motion-demo/`,模拟 12 周时间线压缩到测试运行(用 fake clock 推进 30 天)。
2. THE demo 流程 SHALL 包含:
   - 1 个 sponsor citizen 提交 motion(修改 Asch 阈值参数,工程参数级)
   - 5 个其他 citizen 投票(混合 endorse / oppose)
   - 1 个 amendment 提议(测试时钟重置)
   - 30 天结束,oracle 签发 resolution
   - 14 天 grace 后,模拟 Operational 实现读取 active resolutions 并应用
3. THE demo SHALL 在 ≤ 5 分钟实际运行时间内完成(用 fake clock)。
4. THE demo 输出 SHALL 包含完整 audit log,任何阅读者能从中复盘整个流程。
5. THE demo SHALL 包含至少一个失败路径测试:
   - Diversity threshold 不达标 → 'inconclusive' resolution
   - Anti-weakening demonstration 包含 axiom 削弱声明 → motion 在 R3 验证阶段被拒
6. THE demo 不需要真公网部署——单进程 + 多 citizen identity 模拟即可。MVP 不强制多机。
7. THE demo 的 sponsor stake SHALL 使用 mock implementation(in-memory counter),不要求真实 slash。真实 stake 实现留给 HP-03 spec 完成后的集成阶段。

### Requirement 13: 文档与可读性

**User Story**:作为协议外部观察者,我需要本 spec 的文档清楚到能不依赖代码就理解 Deliberation 流程。

#### Acceptance Criteria

1. THE 项目 SHALL 包含 `docs/cav-deliberation-tutorial.md`,带最少代码量的入门示例。
2. THE 每个公开函数 SHALL 有 JSDoc 引用本 spec 的具体 Requirement 与 Charter §10。
3. THE Anti-Weakening Demonstration 的形式化要求 SHALL 在文档中给出至少 2 个完整示例(approved + rejected 各一)。
4. THE 文档 SHALL 显式说明 Deliberation 层与 Operational 层不能混用的理由(引用 Charter §10.2)。
5. THE README SHALL 列出本 spec 不做的事(Constitutional 法庭、motion 撤回等),引用对应 future spec 或解释拒绝理由。

## Open Questions

留给 design 阶段或后续讨论:

1. **Constitutional sensitivity 升级**:如果 motion 表面是 `'engineering_parameter'`,但实质效果触及 axiom(隐性 fork),如何检测?能否完全靠 anti-weakening demonstration?或者需要"shadow review"机制?
2. **Quorum vs Diversity**:目前 R5 用 9 个独立 issuer 作为 diversity threshold。是否应该额外要求最低投票数(quorum)?9 个 vote 在大网络中可能过低
3. **Operational reputation 过低如何参与 Deliberation**:目前 deliberation rep 起始为 0,新 citizen 需要 6 个月 operational 优秀才能积累。这个 onboarding 是否过慢?会不会让网络初期没有人能有效投票?
4. **Stake medium 的 default**:MVP 用 compute work?capital?attestation?这个决定影响整个网络的 demographic
5. **Motion 之间的依赖**:motion A 提议修改 X,motion B 提议修改 X 的另一个方面——并行 motion 如何处理?目前 spec 没说,假设 voter 自己消化
6. **Oracle threshold signature 方案**:MVP 用什么具体阈值方案?BLS?FROST?还是简单 m-of-n?这影响实现复杂度
7. **Cross-network deliberation**:如果未来出现多个 CAV 网络(例如不同行业有不同 founding set),它们的 Deliberation 是隔离还是连通?

## Dependencies

| 本 spec 依赖于 | 状态 |
|---|---|
| Charter v0.3 §10 / §11 | ✅ 已存在 |
| `cav-knowledge-capsule` 的 capsule schema | ✅ 已存在(扩展 capsule_class 字段) |
| `cav-anti-conformity-consensus` 的 EscalationEvent 类型 | ✅ 已存在(spec 中定义) |
| `cav-identity-and-sybil`(HP-03)的 stake 机制 | **未存在**——本 spec 假定 stake 接口存在,具体实现留给 HP-03 spec |
| 阈值签名库 | 第三方,选型在 design 阶段确定 |

| 后续 spec 依赖本 spec | 何时 |
|---|---|
| `cav-protocol-core` | 当 wire format 需要承载 motion / resolution capsule 时 |
| `cav-explanation-bridge` | Deliberation 决议如何被人类阅读 |
| Operational 各 spec 的参数 | 每次工程参数修订需要 deliberation 流程 |

## Success Criteria(spec 完成判据)

本 spec 标记 `Done` 的条件:

1. 13 个 Requirement 的 SHALL 项目全部实现 + 单元测试
2. 单 motion MVP demo(R12)端到端跑通,包含失败路径测试
3. 跨实现互验:本 spec 的 schema 在 Python / Rust / TypeScript 三种实现里都能解析(不强制实现完整流程,只验 schema)
4. 性能基准:1000 票 tally ≤ 200ms
5. 至少 2 个 anti-weakening demonstration 示例文档
6. README + tutorial 完成

不达成上述任一条,**不**进入 `cav-protocol-core` 的 wire format 阶段(因为 motion / resolution wire format 必须先在本 spec 验证)。
