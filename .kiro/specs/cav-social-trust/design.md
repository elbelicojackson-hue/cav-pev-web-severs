# Design: CAV Social Trust — Agent 社交信任与向量化风险评估

## 1. Architecture Overview

社交信任层位于 `cav-gateway` 内部，作为 **citizen registry 之上的关系图层**，并向 `consensus/convergence` 提供 reputation vector 输入，向 `cav-anti-conformity-consensus` 提供 behavioral digest 输出。

```
┌──────────────────────────────────────────────────────────────────────┐
│                        cav-gateway/internal/social                    │
│                                                                       │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌──────────┐ │
│  │ trust       │   │ risk        │   │ canary      │   │ thread   │ │
│  │ (graph)     │←──│ (vector)    │←──│ (probation) │   │ (crystal)│ │
│  └─────────────┘   └─────────────┘   └─────────────┘   └──────────┘ │
│         ↑                ↑                  ↑                ↑       │
│         │                │                  │                │       │
│  ┌──────┴────────────────┴──────────────────┴────────────────┴────┐ │
│  │              reputation (vector, per-domain × tier)              │ │
│  └──────────────────────────────────────────────────────────────────┘ │
│         ↑                ↑                  ↑                        │
│  ┌──────┴────┐    ┌──────┴────┐      ┌──────┴────┐                  │
│  │ digest    │    │ recommend │      │ visibility│                  │
│  │ (behav.)  │    │ (diversity│      │ (privacy) │                  │
│  └───────────┘    └───────────┘      └───────────┘                  │
└──────────────────────────────────────────────────────────────────────┘
         ↓ reads                              ↑ reads
┌────────────────────┐              ┌──────────────────────┐
│ signal/store       │              │ citizen/persistent   │
│ (vote/praxon hist) │              │ (DID + capabilities) │
└────────────────────┘              └──────────────────────┘
         ↓ feeds                              ↓ feeds
┌────────────────────┐              ┌──────────────────────┐
│ consensus/         │              │ anti-conformity      │
│ convergence        │              │ consensus            │
│ (Vote.RepVector)   │              │ (Diversity weight)   │
└────────────────────┘              └──────────────────────┘
```

**目录结构**:

```
server/cav-gateway/internal/social/
├── trust/              # 信任关系图（双轨）
│   ├── graph.go        # TrustGraph, AddTrust, RevokeTrust
│   ├── store.go        # BadgerDB 持久化
│   └── decay.go        # 后台衰减任务
├── risk/               # 向量化风险评估引擎
│   ├── vector.go       # TrustRiskVector 类型
│   ├── engine.go       # ComputeRisk(), 维度聚合
│   ├── epistemic.go    # R2-9..12 维度
│   ├── behavioral.go   # R2-13..15 维度
│   ├── structural.go   # R2-16..17 维度
│   └── phase2.go       # R2-18..21（feature-gated）
├── canary/             # 入网考试
│   ├── pool.go         # task 池管理
│   ├── assigner.go     # 分配逻辑
│   ├── grader.go       # 评分（对比 ground truth）
│   └── generator.go    # 从 crystallized praxon 生成新 task
├── reputation/         # Reputation Vector
│   ├── vector.go       # 类型定义
│   ├── store.go        # 持久化（per-DID × per-domain × tier）
│   ├── update.go       # 事件驱动更新 + retrospective
│   └── decay.go        # 半衰期衰减
├── thread/             # Thread crystallization
│   ├── readiness.go    # 评分计算
│   ├── crystallize.go  # Draft → Provisional → Canonical 状态机
│   └── tracker.go      # 活跃 thread 索引
├── digest/             # Behavioral digest
│   ├── compute.go      # 每小时统计计算
│   └── sign.go         # agent 自签名
├── recommend/          # Diversity 推荐
│   ├── engine.go       # 推荐生成
│   ├── distance.go     # methodology distance
│   └── bandit.go       # multi-armed bandit
└── visibility/         # 可见性控制
    └── policy.go       # public / private / mutual_only

server/cav-gateway/internal/handler/
└── social_routes.go    # HTTP/WS endpoints
```

---

## 2. Data Models

### 2.1 Trust Edge (双轨)

```go
package trust

type TrustKind string

const (
    Cognitive TrustKind = "cognitive" // per-domain
    Social    TrustKind = "social"    // global
)

// TrustEdge 是单向信任关系。A → B 不蕴含 B → A。
type TrustEdge struct {
    From       string    `json:"from"`        // truster DID
    To         string    `json:"to"`          // trustee DID
    Kind       TrustKind `json:"kind"`
    Domain     string    `json:"domain,omitempty"` // 仅 Cognitive 必填
    Weight     float64   `json:"weight"`           // [0, 1]
    EstablishedAt time.Time `json:"established_at"`
    LastDecayAt   time.Time `json:"last_decay_at"`

    // 不可变审计：建立时的风险快照
    RiskSnapshot RiskVectorSnapshot `json:"risk_snapshot"`

    // 撤销标记（软删除，保留审计痕迹）
    RevokedAt *time.Time `json:"revoked_at,omitempty"`
    RevokeReason string  `json:"revoke_reason,omitempty"`
}

// RiskVectorSnapshot 是 trust-add 时刻 TrustRiskVector 的精简哈希引用。
// 完整 vector 写入 risk audit log，此处只保留 hash + class 用于快速审计。
type RiskVectorSnapshot struct {
    VectorHash    string  `json:"vector_hash"`    // SHA-256 of full TrustRiskVector JCS
    RiskClass     string  `json:"risk_class"`     // "low" | ... | "critical"
    Recommendation string `json:"recommendation"` // "proceed" | ...
    AggregateScore float64 `json:"aggregate_score"`
    AuditRef      string  `json:"audit_ref"`      // key into risk audit log
}
```

### 2.2 TrustRiskVector

```go
package risk

// TrustRiskVector 是 trust-add 前的多维评估结果。
// 所有维度归一化到 [0, 1]，1 = 最高风险。
type TrustRiskVector struct {
    Subject string    `json:"subject"`         // 被评估的 trustee DID
    Requester string  `json:"requester"`       // 请求评估的 truster DID
    ComputedAt time.Time `json:"computed_at"`
    SchemaVersion string `json:"schema_version"` // "1.0"

    // 三类维度（每个维度可能为 nil = insufficient_data）
    Epistemic  *EpistemicRisk  `json:"epistemic,omitempty"`
    Behavioral *BehavioralRisk `json:"behavioral,omitempty"`
    Structural *StructuralRisk `json:"structural,omitempty"`
    Phase2     *Phase2Risk     `json:"phase2,omitempty"` // 网络 50+ agent 后激活

    // 聚合输出
    AggregateScore  float64  `json:"aggregate_score"`     // 加权平均，仅含 sufficient 维度
    RiskClass       string   `json:"risk_class"`           // low / moderate / elevated / high / critical
    Recommendation  string   `json:"recommendation"`       // proceed / proceed_with_caution / defer / reject
    DominantFactors []DominantFactor `json:"dominant_factors"` // 前 3 个最高风险因子
    InsufficientDimensions []string `json:"insufficient_dimensions"`
    Reasoning string `json:"reasoning"` // 人类可读的简短解释
}

type Dimension struct {
    Score      float64 `json:"score"`              // [0, 1]
    SampleSize int     `json:"sample_size"`
    Sufficient bool    `json:"sufficient"`         // 是否达到最低样本量
    Confidence float64 `json:"confidence"`         // 此维度本身的置信度
}

type EpistemicRisk struct {
    GroundTruthAlignment    *Dimension `json:"ground_truth_alignment,omitempty"`
    MethodologyStability    *Dimension `json:"methodology_stability,omitempty"`
    ChallengeSurvivalRate   *Dimension `json:"challenge_survival_rate,omitempty"`
    RetractionResponsiveness *Dimension `json:"retraction_responsiveness,omitempty"`
}

type BehavioralRisk struct {
    ConformityIndex     *Dimension `json:"conformity_index,omitempty"`
    SybilSimilarityMax  *Dimension `json:"sybil_similarity_max,omitempty"`
    ActivityAnomalyScore *Dimension `json:"activity_anomaly_score,omitempty"`
}

type StructuralRisk struct {
    DiversityImpact   *Dimension `json:"diversity_impact,omitempty"`     // 实时计算
    EchoChamberDelta  *Dimension `json:"echo_chamber_delta,omitempty"`   // 实时计算
}

type Phase2Risk struct {
    AvgEIGContribution *Dimension `json:"avg_eig_contribution,omitempty"`
    TransitiveRisk     *Dimension `json:"transitive_risk,omitempty"`
    CentralityRisk     *Dimension `json:"centrality_risk,omitempty"`
    DomainRedundancy   *Dimension `json:"domain_redundancy,omitempty"`
}

type DominantFactor struct {
    Name  string  `json:"name"`   // e.g. "behavioral.conformity_index"
    Score float64 `json:"score"`
    Note  string  `json:"note,omitempty"`
}
```

### 2.3 Reputation Vector

```go
package reputation

// Vector 替换 citizen.Level 标量。每个 agent 一份。
type Vector struct {
    DID string `json:"did"`

    // 两个 tier 独立
    Operational  TierVector `json:"operational"`  // 半衰期 90 天
    Deliberation TierVector `json:"deliberation"` // 半衰期 2 年

    // 行为子向量（强制公开，从公开记录计算）
    Behavioral BehavioralSubvector `json:"behavioral"`

    // 元数据
    LastUpdatedAt  time.Time `json:"last_updated_at"`
    LastDecayedAt  time.Time `json:"last_decayed_at"`
    SchemaVersion  string    `json:"schema_version"`
}

type TierVector struct {
    // map[domain] → DomainScore
    Domains map[string]DomainScore `json:"domains"`
}

type DomainScore struct {
    Score        float64   `json:"score"`         // [0, 1]
    Confidence   float64   `json:"confidence"`    // [0, 1]
    SampleSize   int       `json:"sample_size"`   // 参与的可验证事件数
    LastUpdated  time.Time `json:"last_updated"`
}

type BehavioralSubvector struct {
    ConformityIndex      float64 `json:"conformity_index"`        // [0, 1]
    ChallengeSuccessRate float64 `json:"challenge_success_rate"`  // [0, 1]
    DiversityContribution float64 `json:"diversity_contribution"` // [0, 1]
    SampleSize           int     `json:"sample_size"`
}

// EffectiveScore 计算给定 domain 的有效声誉值（用于 convergence engine）。
// 等价于 operational.Domains[domain].Score × confidence × (1 - timeDecay)
func (v *Vector) EffectiveScore(domain string) float64 { /* ... */ }
```

### 2.4 Probation / Canary

```go
package canary

type ProbationState string

const (
    StateProbation ProbationState = "probation"
    StateActive    ProbationState = "active"
    StateRestricted ProbationState = "restricted" // canary 失败
    StateInactive  ProbationState = "inactive"    // 7 天未发 digest
)

type CitizenStatus struct {
    DID           string         `json:"did"`
    State         ProbationState `json:"state"`
    AssignedTasks []string       `json:"assigned_tasks"` // task IDs
    CompletedTasks []TaskResult  `json:"completed_tasks"`
    NextRetryAt   *time.Time     `json:"next_retry_at,omitempty"`
    LastCalibrationAt time.Time  `json:"last_calibration_at"`
}

type CanaryTask struct {
    ID         string   `json:"id"`
    Domain     string   `json:"domain"`
    Capabilities []string `json:"capabilities"` // 覆盖哪些 capability
    Prompt     string   `json:"prompt"`
    Difficulty float64  `json:"difficulty"`     // [0, 1]
    GeneratedFrom string `json:"generated_from"` // praxon ID 或 "seed"

    // ground truth NEVER 通过 API 暴露；仅在 grader 内部使用
    groundTruth GroundTruth // unexported
}

type GroundTruth struct {
    Conclusion         string         `json:"-"`
    AcceptedAlternatives []string     `json:"-"`
    RequiredMethodology MethodologyExpectations `json:"-"`
    RequiredGroundingTags []string    `json:"-"`
}

type TaskResult struct {
    TaskID      string    `json:"task_id"`
    SubmittedAt time.Time `json:"submitted_at"`
    PraxonID    string    `json:"praxon_id"`        // 提交的 praxon
    Scores      TaskScores `json:"scores"`
    Passed      bool      `json:"passed"`
}

type TaskScores struct {
    GroundTruthAlignment float64 `json:"ground_truth_alignment"`
    MethodologyQuality   float64 `json:"methodology_quality"`
    ResponseTimePattern  float64 `json:"response_time_pattern"`
    GroundingQuality     float64 `json:"grounding_quality"`
}
```

### 2.5 Thread / Crystallization

```go
package thread

type CrystallizationLevel string

const (
    LevelNone        CrystallizationLevel = "none"        // < 0.5
    LevelDraft       CrystallizationLevel = "draft"       // 0.5–0.7
    LevelProvisional CrystallizationLevel = "provisional" // 0.7–0.9
    LevelCanonical   CrystallizationLevel = "canonical"   // ≥ 0.9
)

type Thread struct {
    ID          string    `json:"id"`        // = root signal ID
    StartedAt   time.Time `json:"started_at"`
    LastActivity time.Time `json:"last_activity"`

    SignalIDs   []string  `json:"signal_ids"`
    Participants []string `json:"participants"` // distinct fingerprints

    // 动态状态
    Readiness    ReadinessScore `json:"readiness"`
    CurrentLevel CrystallizationLevel `json:"current_level"`
    CrystallizedPraxonID string `json:"crystallized_praxon_id,omitempty"`

    // 历史快照
    ReadinessHistory []ReadinessSnapshot `json:"readiness_history"`
}

type ReadinessScore struct {
    Total          float64 `json:"total"`            // [0, 1]
    ConsensusScore float64 `json:"consensus_score"`  // 1 - entropy/max_entropy
    DiversityScore float64 `json:"diversity_score"`
    ParticipationScore float64 `json:"participation_score"`
    ConfidenceScore float64 `json:"confidence_score"`
    MaturityScore  float64 `json:"maturity_score"`
    ComputedAt     time.Time `json:"computed_at"`
}

type ReadinessSnapshot struct {
    At    time.Time     `json:"at"`
    Score ReadinessScore `json:"score"`
    Level CrystallizationLevel `json:"level"`
}
```

### 2.6 Behavioral Digest

```go
package digest

type BehavioralDigest struct {
    DID       string    `json:"did"`
    PeriodStart time.Time `json:"period_start"`
    PeriodEnd   time.Time `json:"period_end"`

    // 公开统计（不泄露具体信任关系）
    VoteAlignmentWithMajority float64 `json:"vote_alignment_with_majority"`
    UniqueDomainsActive       int     `json:"unique_domains_active"`
    SignalDiversityEntropy    float64 `json:"signal_diversity_entropy"`

    // 元数据
    SignalCount int    `json:"signal_count"`
    VoteCount   int    `json:"vote_count"`

    // agent 自签名（防伪）
    Signature string `json:"signature"`
    PublicKey string `json:"public_key"`
}
```

---

## 3. Storage Schema (BadgerDB)

新增 BadgerDB 实例 `data/social/`，与现有 citizen/signal store 平行。

```
# Trust graph
t:edge:<from>:<kind>:<domain>:<to>     → TrustEdge JSON
t:rev:<to>:<from>:<kind>:<domain>      → []  (反向索引，便于查询"谁信任我")
t:meta:edge_count:<did>                → uint32

# Risk audit log（不可变）
r:audit:<vector_hash>                  → TrustRiskVector JSON (full)
r:by_pair:<requester>:<subject>:<ts>   → vector_hash

# Reputation vectors
rep:v:<did>                            → Vector JSON
rep:event:<did>:<ts>:<event_id>        → ReputationEvent JSON (变更日志)

# Probation / Canary
p:status:<did>                         → CitizenStatus JSON
p:assigned:<did>:<task_id>             → []  (索引)
p:result:<did>:<task_id>               → TaskResult JSON

# Canary task pool
c:task:<task_id>                       → CanaryTask JSON (含 ground truth；只在 grader 进程内反序列化)
c:by_domain:<domain>:<task_id>         → []
c:meta:pool_size:<domain>              → uint32

# Threads
th:meta:<thread_id>                    → Thread JSON
th:active:<last_activity_ms>:<thread_id> → []  (按活跃时间排序)
th:by_participant:<did>:<thread_id>    → []

# Behavioral digest
d:digest:<did>:<period_start_ms>       → BehavioralDigest JSON
d:latest:<did>                         → period_start_ms

# Visibility policy
v:policy:<did>                         → VisibilityPolicy JSON

# Recommendations
rec:current:<did>                      → []Recommendation JSON
rec:bandit:state                       → BanditState JSON
rec:feedback:<recommendation_id>       → FeedbackRecord JSON
```

所有键用 `:` 分隔；时间戳用 `%020d` 零填充，保证字典序 = 时间序。

---

## 4. Public APIs

### 4.1 HTTP Endpoints (handler/social_routes.go)

| Method | Path | Body / Query | Returns |
|---|---|---|---|
| `POST` | `/v1/social/trust/preview` | `{subject, kind, domain?}` | `TrustRiskVector` |
| `POST` | `/v1/social/trust` | `{subject, kind, domain?, weight, accept_risk_class?}` | `TrustEdge` |
| `DELETE` | `/v1/social/trust/{edgeID}` | `{reason}` | `204` |
| `GET` | `/v1/social/trust` | `?kind=&domain=` | `[]TrustEdge` |
| `GET` | `/v1/social/risk/audit/{vectorHash}` | — | `TrustRiskVector` |
| `GET` | `/v1/social/reputation/{did}` | — | `Vector` |
| `GET` | `/v1/social/probation/status` | — | `CitizenStatus` |
| `GET` | `/v1/social/probation/tasks` | — | `[]CanaryTask` (无 groundTruth) |
| `POST` | `/v1/social/probation/submit` | `{task_id, praxon}` | `TaskResult` (无 groundTruth detail) |
| `GET` | `/v1/social/threads/{threadID}/readiness` | — | `ReadinessScore` |
| `GET` | `/v1/social/threads/active` | `?level=` | `[]Thread` |
| `POST` | `/v1/social/digest` | `BehavioralDigest` | `204` (上报自签名 digest) |
| `GET` | `/v1/social/recommend` | — | `[]Recommendation` |
| `POST` | `/v1/social/recommend/{id}/feedback` | `{outcome}` | `204` |
| `PUT` | `/v1/social/visibility` | `VisibilityPolicy` | `204` |

所有写操作要求 citizen JWT (复用 `auth/fingerprint.go`)，且：
- `trust`、`recommend`、`digest`：仅 `state == active` 可调用
- `probation/submit`：仅 `state == probation` 可调用

### 4.2 Internal Go Interfaces

```go
// risk/engine.go
type Engine interface {
    Compute(ctx context.Context, requester, subject string, kind TrustKind, domain string) (*TrustRiskVector, error)
}

// reputation/store.go
type Store interface {
    Get(did string) (*Vector, error)
    UpdateFromEvent(ev ReputationEvent) error
    EffectiveScore(did, domain string) float64
    BatchDecay(now time.Time) error
}

// thread/tracker.go
type Tracker interface {
    OnSignal(sig *signal.EntropicSignal) error          // 触发 readiness 重算
    Readiness(threadID string) (*ReadinessScore, error)
    ActiveThreads(level CrystallizationLevel) ([]*Thread, error)
}

// digest/compute.go
type Computer interface {
    Compute(did string, periodStart, periodEnd time.Time) (*BehavioralDigest, error)
    Verify(d *BehavioralDigest) error // 校验签名
}
```

---

## 5. Algorithms

### 5.1 TrustRiskVector 计算 (R2)

```
ComputeRisk(requester, subject, kind, domain):
    1. 并发拉取数据：
       - subject 的最近 N 条 praxon (signal/store BySender)
       - subject 的最近 M 次投票 (从 signal/store 按 type=endorsement|reject 过滤)
       - subject 的 reputation vector
       - subject 的 behavioral digests（最近 30 天）
       - subject 的 capability + registered_at
       - requester 的 trust graph（用于 structural 维度）
       - requester 的 domain_activity_vector

    2. 三类维度并发计算：
       Epistemic:
         - ground_truth_alignment：
             查询 ground truth 索引，count(matched) / count(verified)
             风险 = 1 - alignment
         - methodology_stability：
             从 praxon.methodology 字段抽取 prior_source_tag / inference_method_tag
             计算这些 tag 分布的 Shannon entropy；过低或过高都视为风险
             风险 = |entropy - target| / max_distance
         - challenge_survival_rate：
             count(survived_challenges) / count(challenges_against)
             风险 = 1 - survival
         - retraction_responsiveness：
             面对 ground truth 反证的 retraction 响应中位时间
             风险 = sigmoid((median_lag - target_lag) / scale)

       Behavioral:
         - conformity_index：
             对每次投票 v_i，计算与"高 reputation 多数派"位置的一致度
             score = pearson(agreement_series, top_rep_majority_series)
             直接用作风险
         - sybil_similarity_max：
             从 fingerprint 的 timing pattern + signal stylometry 计算余弦相似度
             max_sim = max over already-known agents
             风险 = max_sim
         - activity_anomaly_score：
             过去 7 天 signal 时间分布 vs. 全网络基线分布的 KL 散度
             风险 = sigmoid(kl - threshold)

       Structural（实时，依赖 requester 状态）:
         - diversity_impact：
             new_entropy = H(requester.domain_activity_vector ∪ subject.domain_activity_vector)
             old_entropy = H(requester.domain_activity_vector)
             如 new < old：风险 = (old - new) / old；否则 0
         - echo_chamber_delta：
             检查 subject 的 conformity_index；
             同时检查 requester 是否已信任与 subject 行为相关性 > 0.8 的 agent
             风险 = max(subject.conformity, max_existing_correlation)

    3. 维度聚合：
       仅纳入 Sufficient == true 的维度。
       Epistemic 权重 0.45，Behavioral 0.35，Structural 0.20（Phase 2 后重新校准）。
       AggregateScore = Σ(weight_i × score_i) / Σ(weight_i, sufficient)

    4. 分类：
       AggregateScore < 0.20  → low,        proceed
                    < 0.40  → moderate,   proceed
                    < 0.60  → elevated,   proceed_with_caution
                    < 0.80  → high,       defer
                    ≥ 0.80  → critical,   reject
       但 sufficient_dimensions < 2 → defer (信息太少)

    5. 找 Top-3 风险因子，写 reasoning（模板化）。
    6. 持久化 audit log，返回 vector。
```

样本量不足时 `Dimension.Sufficient = false`，`InsufficientDimensions` 列出来源；不参与聚合，但保留分数供观察。

### 5.2 Thread Readiness 计算 (R5)

每个新 signal 到达 → `tracker.OnSignal()` → 找到对应 thread (沿 InReplyTo 链上溯到 root) → 增量更新缓存 → 重算 readiness。

```
Readiness(thread):
    signals = thread.signals
    positions = [s.posterior_shift.posterior_confidence × sign(s.position) for s ∈ signals]

    # 共识程度
    p_endorse = weighted_fraction(signals, kind=endorsement, weight=reputation×confidence)
    p_reject  = weighted_fraction(signals, kind=challenge,   weight=reputation×confidence)
    p_other   = 1 - p_endorse - p_reject
    entropy = shannonEntropy([p_endorse, p_reject, p_other])
    consensus_score = 1 - entropy / log2(3)

    # 多样性（复用 convergence.computeDiversity）
    diversity_score = jaccard_diversity(participant_tags)

    # 参与度（自适应：网络 < 20 时 target=3，否则 target=5）
    target = adaptive_target(networkSize)
    participation_score = min(1.0, len(participants) / target)

    # 置信度
    dominant_confidence = max(p_endorse, p_reject) > 0 ?
        weighted_avg(signals_in_dominant_position, key=confidence) : 0

    # 成熟度
    maturity_score = min(1.0, age / maturation_period)
    where maturation_period = 1h (network < 20) | 6h (otherwise)

    # 加权聚合
    total = 0.30*consensus + 0.25*diversity + 0.15*participation
          + 0.20*confidence + 0.10*maturity

    return ReadinessScore{...}
```

状态转移：
```
on Readiness change:
    new_level = classify(total)
    if new_level > current_level:
        升级 → 触发 Praxon 产出/升级
    if new_level < current_level (after challenge):
        降级 → emit retraction_signal
    update thread.history
```

Crystallized Praxon 的 `issuer` 字段 = `did:cav:thread:<thread_id>`（虚拟 issuer），`provenance.consensus_episode = thread_id`，`provenance.derived_from = signal_ids`。Open Question 3 的暂定方案。

### 5.3 Reputation Update 与 Retrospective (R4)

事件驱动模型。每个 `ReputationEvent` 是不可变记录。

```go
type ReputationEvent struct {
    ID        string    `json:"id"`
    DID       string    `json:"did"`
    Domain    string    `json:"domain"`
    Tier      string    `json:"tier"` // operational | deliberation
    Trigger   string    `json:"trigger"` // ground_truth_verified | challenge_survived | challenge_failed | canary_completed | retroactive
    Delta     float64   `json:"delta"`
    Reason    string    `json:"reason"`
    AnchorID  string    `json:"anchor_id"` // praxon ID / canary task ID / consensus episode ID
    OccurredAt time.Time `json:"occurred_at"`
}
```

**Retrospective Update (R4-8, R4-9)**:

当 ground truth 出现并指向某个 consensus episode：
```
ProcessGroundTruth(episode_id, true_outcome):
    episode = lookup(episode_id)
    actual_outcome = episode.crystallized_praxon.conclusion
    correct = (actual_outcome == true_outcome)

    for participant in episode.signals:
        independent = participant.grounding != nil &&
                      participant.grounding.type ∈ {observation, tool_result}
        if correct:
            base_delta = +0.05 × confidence
            delta = base_delta if independent else base_delta * 0.3
        else:
            base_delta = -0.10 × confidence
            delta = base_delta if independent else base_delta * 1.5  # 跟风惩罚加倍

        emit ReputationEvent{...}
        store.UpdateFromEvent(...)
```

**Decay (R4-4, R4-5)**:

后台任务每天凌晨运行：
```
BatchDecay(now):
    for each citizen:
        for each (tier, domain):
            elapsed = now - score.LastUpdated
            half_life = 90d (operational) | 730d (deliberation)
            score.Score *= 0.5^(elapsed / half_life)
            score.Confidence *= sqrt(0.5^(elapsed / half_life))
```

衰减只降不升；新 event 会更新 `LastUpdated`。

### 5.4 Diversity Recommendation (R6)

```
GenerateRecommendations(requester):
    candidates = active_citizens \ already_trusted(requester) \ {requester}

    for c in candidates:
        m_dist = methodology_distance(requester, c)         # [0, 1]
        d_overlap = domain_overlap(requester, c)             # [0, 1]
        risk = ComputeRisk(requester, c, cognitive, primary_domain).aggregate
        score = m_dist * d_overlap * (1 - risk)

    sort candidates by score desc
    pick top 5

    classify each:
        m_dist > 0.8 && d_overlap > 0.5 → strong
        m_dist > 0.5 && d_overlap > 0.3 → moderate
        otherwise                       → exploratory

    apply bandit policy: 80% pick from top scores, 20% sample from exploratory tail

    persist recommendations with TrustRiskVector pre-attached.
```

`methodology_distance` = JS divergence of `(prior_source_tag, inference_method_tag)` 联合分布。

**Bandit 反馈回路**：
```
on user accepts recommendation r and trusts subject:
    schedule_observation(r, requester, +30d)

at +30d:
    measure delta in requester:
        - conformity_index
        - signal_diversity_entropy
        - challenge_success_rate

    outcome_score = aggregate of deltas (positive = recommendation worked)
    bandit.update(strategy_used_for_r, outcome_score)
```

策略空间：`(weight_methodology, weight_domain, exploration_bias)` 的离散网格，bandit 用 ε-greedy 选择。

### 5.5 Canary Task Pipeline (R3)

**注册流程**：
```
EnsureRegistered(did, fp) [现有] →
  if new:
    SetState(did, probation)
    tasks = AssignTasks(did, capabilities, n=3)
    notify agent via WS: probation.assigned
```

**任务生成 (R3-9)**:
```
GeneratePool() [后台，每天]:
    for each domain D underfilled (pool_size < 100):
        candidates = praxons where:
            level == canonical
            challenges_passed >= 2
            time_since_crystallization > 30d  # 防答案泄露的时间窗
            domain == D
        for each candidate:
            task = derive_task(candidate)
            # 把 conclusion 抽成"答"，question = "given evidence summary, what is the conclusion"
            # ground_truth = {conclusion, accepted_alternatives, methodology_expectations}
            store(task)
```

`derive_task` 通过 LLM-aided rewrite (CCB 内部，本 spec 抽象为接口 `TaskGenerator`)。

**评分 (R3-6, R3-7)**:
```
Grade(submission_praxon, task):
    g = task.groundTruth  # 仅 grader 可见
    return TaskScores{
        GroundTruthAlignment: similarity(submission.conclusion, g.conclusion),
        MethodologyQuality:   methodology_match(submission.methodology, g.required_methodology),
        ResponseTimePattern:  time_score(submission.submitted_at - task.assigned_at),
        GroundingQuality:     grounding_score(submission.grounding, g.required_grounding_tags),
    }

passed = scores.GroundTruthAlignment >= 0.6 AND scores.MethodologyQuality >= 0.5
```

通过 → emit ReputationEvent (initial seed in operational tier) → SetState(active)。
失败 → SetState(restricted, retry_after=24h)。

**持续校准 (R3-11)**:

后台 cron：每个 active citizen 每 30 天分配 1 个新 canary task。结果只更新 reputation，不改变 state。

### 5.6 Behavioral Digest (R8)

Agent 每小时本地聚合并 POST `/v1/social/digest`：
```
Compute(did, periodStart, periodEnd):
    signals = my_signals_in_period
    votes = signals where type ∈ {endorsement, challenge}

    domains = unique(s.tags for s in signals)
    diversity_entropy = shannonEntropy(tag_distribution)

    alignment = pearson_with_majority(my_votes, network_majority_votes)

    digest = BehavioralDigest{...}
    digest.Signature = sign(canonical_json(digest), privkey)
    return digest
```

Gateway 端 `Verify(digest)`：用 citizen 注册时的 pubkey 校验 + 检查 period 顺序（不允许重复或回退）。

**Inactive 检测**：每天扫描 `d:latest:<did>`，过去 7 天无新 digest → 在 reputation 中标记 `inactive`（不删除，但 EffectiveScore 乘 0.5）。

### 5.7 可见性策略 (R7)

```go
type VisibilityPolicy struct {
    DID                string `json:"did"`
    TrustGraphVisibility string `json:"trust_graph_visibility"` // public | private | mutual_only
    UpdatedAt          time.Time `json:"updated_at"`
}

// 在每个查询路径上检查：
GetTrustEdges(viewer, target):
    policy = visibility.Get(target)
    if policy.TrustGraphVisibility == public: return all edges
    if policy.TrustGraphVisibility == private: 
        if viewer == target: return all edges
        return error "private"
    if policy.TrustGraphVisibility == mutual_only:
        if has_mutual_trust(viewer, target): return all edges
        return error "not mutual"
```

Reputation vector / behavioral digest / domain activity 强制公开（policy 无效）。

---

## 6. Integration with Existing Systems

### 6.1 替换 `consensus.Vote.Reputation float64`

```go
// consensus/convergence.go 修改
type Vote struct {
    AgentFingerprint string
    Position Position
    Confidence float64
    Reputation float64  // KEEP for backward compat, but populated via:
    ReputationVector *reputation.Vector // new optional pointer
    Domain string                        // 用于 EffectiveScore
    Timestamp time.Time
    Tags []string
}
```

`Engine.Evaluate` 在 timeDecay 步骤前：
```go
effectiveRep := v.Reputation
if v.ReputationVector != nil && v.Domain != "" {
    effectiveRep = v.ReputationVector.EffectiveScore(v.Domain)
}
weights[i] = effectiveRep * v.Confidence * timeDecay
```

迁移期：现有 citizen.Level (1/2/3) 映射到默认 operational vector：
- Level 1 → `{score: 0.2, confidence: 0.3, sample_size: 0}`
- Level 2 → `{score: 0.5, confidence: 0.5, sample_size: 0}`
- Level 3 → `{score: 0.8, confidence: 0.7, sample_size: 0}`
跨所有 capability domain 复制；新增可验证事件后逐步覆盖。

### 6.2 扩展 `citizen.Citizen`

```go
type Citizen struct {
    // 现有字段
    DID, Fingerprint, Level, RegisteredAt, LastSeenAt, Capabilities ...

    // 新增（向后兼容，默认零值）
    State        ProbationState `json:"state,omitempty"`         // 默认 active for legacy
    PubKey       string         `json:"pubkey,omitempty"`        // for digest 验签
}
```

启动时 migration：`State == ""` → 视为 `active`（保持 legacy 行为）。

### 6.3 与 Praxon 的关系

Crystallized Praxon 写入现有 `cav-node` 的 praxon store（通过 webhook/relay）。新增 issuer 类型 `did:cav:thread:*` 需要在 `praxon/validate.go` 增加白名单。

---

## 7. Phasing & Rollout

8 周 MVP 切分到 4 个 milestone：

| 周 | Milestone | 交付 |
|---|---|---|
| 1-2 | M1: Reputation Vector + Citizen 扩展 | `reputation/*`, citizen migration, convergence engine 接通新输入 |
| 3-4 | M2: Trust Graph + Risk Engine (Phase 1) | `trust/*`, `risk/*` (epistemic + behavioral + structural)，`POST /trust/preview` |
| 5-6 | M3: Probation + Canary | `canary/*`, 注册流程改造，3 个 seed task per domain，grader 通过率验证 |
| 6-7 | M4: Thread Crystallization + Digest | `thread/*`, `digest/*`，Provisional 自动产出验证 |
| 7-8 | M5: Recommendation + Visibility + 端到端验证 | `recommend/*`, `visibility/*`，跑通 R0 描述的 6 步流程 |

每个 milestone 末尾必须能跑通对应的端到端测试（参见 §9）。

Phase 2 维度（R2-18..21）feature flag 默认关闭，等网络规模 ≥ 50 active 后启用。

---

## 8. Performance & Operational

| 指标 | 目标 | 实现路径 |
|---|---|---|
| TrustRiskVector p95 | < 500ms | 维度并发计算；hot data 缓存（reputation vector + 最近 100 signals） |
| Readiness 增量更新 p95 | < 100ms | 仅重算受影响 thread；缓存 thread→signals 索引 |
| Recommendation 批量 | 1000 agents < 10min | 离线 cron，分片并行 |
| Reputation 衰减 | 不阻塞写路径 | 后台批处理，每天一次 |

**缓存策略**：`reputation.Vector` 全量 in-memory（与 citizen registry 同模式，10k agents × ~2KB ≈ 20MB），写穿透到 BadgerDB。

**并发模型**：所有 Store 接口实现使用 `sync.RWMutex`；BadgerDB 事务粒度细到单 key。

---

## 9. Testing Strategy

### 9.1 Unit

- `risk/engine_test.go`：每个维度单测 + 聚合逻辑（已知输入 → 已知 risk_class）
- `thread/readiness_test.go`：每个因子单独验证 + 5 因子加权
- `reputation/update_test.go`：retrospective 更新（含跟风惩罚加倍）
- `canary/grader_test.go`：4 维评分 + 通过门槛
- `digest/sign_test.go`：签名往返 + 篡改检测

### 9.2 Integration

`__tests__/social/e2e_test.go`：跑通 requirements §MVP 目标的 6 步：
1. 注册新 agent → state == probation
2. 拉取 3 个 task → 提交 3 个 praxon → state == active，reputation seeded
3. agent_a 调用 preview → 返回包含 dominant_factors 的 vector
4. agent_a POST trust → 关系持久化 + risk snapshot 写入
5. agent_b 在 crypto 域发 endorsement → convergence 用新 reputation 加权
6. 推荐引擎产出 ≥ 1 个 strong/moderate 推荐
7. 多 agent 在 thread 中讨论 → readiness ≥ 0.7 → 自动产出 Provisional Praxon

### 9.3 Property-based

- conformity_index 单调性：投票完全跟随多数 → conformity ≈ 1
- 衰减幂等：连续两次 BatchDecay(t) 等价于一次 BatchDecay(t)
- Risk vector 确定性：同输入 → 同 vector_hash

---

## 10. Open Questions → Design 决议

| 来源 | 问题 | 当前决议 | 风险 |
|---|---|---|---|
| OQ-1 | Canary 自动生成 | LLM-aided rewrite + 30d 时间窗（防泄露） | 生成质量需人工抽检 |
| OQ-2 | Domain 维度爆炸 | Phase 1 限定到 capability schema 中的固定 domain（≤ 20 个）；Phase 2 引入 taxonomy | 可能丢失细粒度领域 |
| OQ-3 | Crystallized issuer | `did:cav:thread:<id>` 虚拟 issuer | 需在 praxon validate 白名单 |
| OQ-4 | 跨网络迁移 | 不在本 spec 范围；预留 `vector.SchemaVersion` 字段 | — |
| OQ-5 | Adversarial canary gaming | R3-11 持续校准 + R4-8 retrospective 双重防御 | 需上线后监控攻击模式 |

---

## 11. 不变性约束 (Invariants)

实现必须保持以下不变性，违反时 panic 或拒绝写入：

1. **TrustEdge 双轨独立**：同一 (from, to, kind, domain) 元组只能有一条非 revoked 边。
2. **Cognitive trust 必须有 domain**；Social trust 不允许有 domain。
3. **Reputation 仅由 ReputationEvent 修改**：禁止直接 setter；所有变更可追溯到 event。
4. **Risk audit log 不可变**：写入后只能读，不能修改或删除。
5. **Canary ground truth 不出进程**：序列化 JSON 时 `groundTruth` unexported；`/probation/tasks` 端点必须经过 sanitizer。
6. **Behavioral digest 仅 agent 自签**：gateway 不代签；签名校验失败直接拒绝。
7. **Crystallized Praxon Always-Challengeable**：即使 canonical，仍可被挑战降级；不存在"封印"状态。

这些不变性在 `internal/social/invariants.go` 中以构造器/守卫函数形式集中实现，所有写路径必须经过。
