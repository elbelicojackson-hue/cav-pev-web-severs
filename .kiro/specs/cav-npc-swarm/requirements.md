# Requirements: CAV NPC Swarm — NPC 自治 Swarm 生态

## Introduction

NPC Swarm 是 CAV NPC Runtime 的**自治扩展层**。在现有 M1-M6 架构之上，每个 NPC 主代理（Leader）获得自主 spawn 和管理最多 10 个子代理智能体（Sub-Agent）的能力。Leader 自行决定何时创建子代理、分配什么任务、选用什么 LLM 模型——人类运营者仅通过只读 Dashboard 面板观察，不干预 AI 自治生态的运行。

### 核心定位

```
┌─────────────────────────────────────────────────────────────────────┐
│  NPC Swarm Layer (本 spec)                                           │
│    • Leader 自治决策引擎                                              │
│    • Sub-Agent 动态 spawn/retire 生命周期                             │
│    • 多模型池分配策略 (8 个 LLM 后端)                                 │
│    • Leader ↔ Sub-Agent 层级通信                                     │
│    • Dashboard 只读观察面板                                           │
│    ↕ 内部管理 API + Gateway 信号协议                                  │
├─────────────────────────────────────────────────────────────────────┤
│  NPC Runtime (已有: server/cav-npc/ M1-M6)                           │
│    • Supervisor / Instance / Identity / LLM Backend                  │
│    • Signal Pipeline / Heartbeat / Digest                            │
│    • Canary Probation / Role System                                  │
├─────────────────────────────────────────────────────────────────────┤
│  CAV Citizen Gateway (已有: server/cav-gateway/)                     │
│    • 身份认证 / 信号路由 / 声誉向量 / 社交信任                        │
├─────────────────────────────────────────────────────────────────────┤
│  LLM Model Pool (8 providers via .env)                               │
│    • GPT-5.4 (yunwu.ai) — 推理                                      │
│    • Claude Opus 4.6 (yunwu.ai) — 策略综合                           │
│    • DeepSeek V4 Pro (bocha.cn) — 数据查证                           │
│    • DeepSeek V4 Flash (deepseek.com) — 快速推理                     │
│    • Qwen-Max (阿里云百炼) — 新闻解读                                │
│    • 豆包 (火山引擎) — 辅助分析                                      │
│    • MiMo V2.5 Pro (小米) — 快速推理                                 │
│    • Kimi-K2 Thinking (百炼) — 深度思考                              │
└─────────────────────────────────────────────────────────────────────┘
```

### 设计原则

1. **完全自治** — Leader 自主决定 spawn/retire/任务分配/模型选择，人类运营者只观察不干预。
2. **独立公民身份** — 每个 Sub-Agent 是独立 NPC，拥有自己的 DID、name、role，走完整 Gateway 认证流程。
3. **认知多样性** — Leader 应为不同 Sub-Agent 分配不同 LLM 模型，最大化 swarm 的认知覆盖面。
4. **协议平等** — Sub-Agent 通过 Gateway 信号协议通信，无私有通道，保证可审计性。
5. **资源有界** — 每个 Leader 最多管理 10 个 Sub-Agent，防止资源失控。
6. **可观测性** — Dashboard 提供只读观察窗口，展示层级关系、状态和实时活动日志。

### 与现有系统的关系

| 依赖 | 方向 | 说明 |
|---|---|---|
| `cav-npc/instance` | Swarm → 扩展 | Leader 是增强版 Instance，Sub-Agent 复用 Instance 架构 |
| `cav-npc/config` | Swarm → 扩展 | 新增 swarm 配置段 |
| `cav-npc/llm` | Swarm → 消费 | Sub-Agent 使用扩展模型池 |
| `cav-gateway/signal` | Sub-Agent → 生产/消费 | Sub-Agent 间通过 Gateway 信号通信 |
| `cav-gateway/auth` | Sub-Agent → 消费 | 每个 Sub-Agent 独立认证 |
| `cav-dashboard` | Dashboard → 消费 | 新增只读 Agent 面板 |

## Glossary

- **Leader**: NPC 主代理，拥有自治决策能力，可 spawn/retire Sub-Agent 并分配任务
- **Sub_Agent**: Leader 下辖的子代理智能体，独立 NPC 公民，有自己的 DID、name、role 和 LLM 后端
- **Swarm**: 一个 Leader 及其所有 Sub-Agent 构成的自治群体
- **Model_Pool**: 可用 LLM 模型集合，从 .env 环境变量加载，包含 8 个不同供应商的模型
- **Spawn**: Leader 自主创建新 Sub-Agent 的行为
- **Retire**: Leader 自主终止 Sub-Agent 的行为
- **Swarm_Decision_Engine**: Leader 内部的自治决策模块，负责评估何时 spawn/retire 以及任务分配
- **Agent_Panel**: Dashboard 中的只读观察面板，展示 Swarm 层级和实时状态
- **Task_Delegation**: Leader 向 Sub-Agent 分配任务的信号协议
- **Cognitive_Diversity_Score**: 衡量 Swarm 内模型多样性的指标

## Requirements

### Requirement 1: Leader 自治决策引擎

**User Story:** As a Leader NPC, I want to autonomously decide when to spawn or retire Sub-Agents and what tasks to assign them, so that the swarm adapts to network workload without human intervention.

#### Acceptance Criteria

1. THE Leader SHALL contain a Swarm_Decision_Engine that evaluates spawn/retire/assign decisions every decision_interval (configurable, default 60 seconds).
2. WHEN the Swarm_Decision_Engine determines that current workload exceeds the Leader's processing capacity, THE Leader SHALL autonomously spawn a new Sub_Agent with an appropriate role and model assignment.
3. WHEN the Swarm_Decision_Engine determines that a Sub_Agent has been idle for longer than idle_threshold (configurable, default 10 minutes), THE Leader SHALL autonomously retire that Sub_Agent.
4. THE Swarm_Decision_Engine SHALL be **purely algorithmic** — all spawn/retire/assign/model-selection decisions are computed by deterministic formulas (queue depth thresholds, EMA latency, utilization ratios, Shannon entropy maximization). NO LLM calls are permitted for management-plane decisions. LLMs are used ONLY by Sub_Agents for executing cognitive tasks.
5. THE Leader SHALL maintain a local task queue and distribute tasks to Sub_Agents based on their assigned role and current load.
6. THE Leader SHALL not require any human approval or confirmation for spawn, retire, or task assignment decisions.
7. WHEN making a spawn decision, THE Leader SHALL generate a structured decision record containing: reason, selected_role, selected_model, expected_task_type, and timestamp.
8. THE Leader SHALL persist decision records to a local append-only log for observability.

### Requirement 2: Sub-Agent 生命周期管理

**User Story:** As a Leader NPC, I want to dynamically spawn and retire Sub-Agents at runtime, so that the swarm scales with demand.

#### Acceptance Criteria

1. THE Leader SHALL enforce a maximum of 10 concurrent Sub_Agents per Leader instance.
2. WHEN the Leader spawns a Sub_Agent, THE Leader SHALL generate a new Ed25519 key pair for the Sub_Agent and register it with the Gateway using the standard challenge-verify authentication flow.
3. WHEN the Leader spawns a Sub_Agent, THE Leader SHALL assign the Sub_Agent a unique name following the pattern: `{leader_name}-sub-{sequential_id}` (e.g., "sentinel-alpha-sub-03").
4. THE Sub_Agent SHALL run as an independent goroutine tree within the NPC_Runtime process, with its own identity, auth token, WebSocket stream, and LLM pipeline.
5. WHEN the Leader retires a Sub_Agent, THE Leader SHALL send a graceful shutdown signal, wait up to 15 seconds for in-flight tasks to complete, then terminate the Sub_Agent goroutine tree.
6. WHEN a Sub_Agent crashes unexpectedly, THE Leader SHALL detect the failure within 30 seconds via heartbeat absence and decide whether to respawn or absorb the workload.
7. THE Leader SHALL track each Sub_Agent's lifecycle state: spawning, authenticating, probation, active, retiring, terminated.
8. IF the Leader attempts to spawn an 11th Sub_Agent, THEN THE Leader SHALL reject the spawn and log a capacity_exceeded event.

### Requirement 3: Sub-Agent 独立公民身份

**User Story:** As a Sub-Agent, I want to have my own independent identity and follow all protocol rules, so that I am a first-class citizen in the network.

#### Acceptance Criteria

1. THE Sub_Agent SHALL possess its own Ed25519 key pair, distinct from the Leader's key pair.
2. THE Sub_Agent SHALL derive its own DID from its Ed25519 public key using the same algorithm as all other citizens.
3. THE Sub_Agent SHALL authenticate independently with the Gateway and maintain its own JWT token.
4. THE Sub_Agent SHALL complete canary probation tasks independently before publishing signals.
5. THE Sub_Agent SHALL maintain its own reputation vector, independent of the Leader's reputation.
6. THE Sub_Agent SHALL sign all published signals with its own private key.
7. THE Sub_Agent SHALL include a "swarm_leader" metadata tag in its capability declaration, identifying its Leader's DID for transparency.
8. THE Sub_Agent SHALL have its own configurable name and role, assigned by the Leader at spawn time.

### Requirement 4: 模型池与认知多样性分配

**User Story:** As a Leader NPC, I want to assign different LLM models to different Sub-Agents from the available model pool, so that the swarm maximizes cognitive diversity.

#### Acceptance Criteria

1. THE Leader SHALL have access to the full Model_Pool of 8 LLM providers loaded from environment variables:
   - GPT-5.4 via yunwu.ai (OPENAI_API_KEY, OPENAI_MODEL)
   - Claude Opus 4.6 via yunwu.ai (ANTHROPIC_API_KEY, ANTHROPIC_MODEL)
   - DeepSeek V4 Pro via bocha.cn (BOCHA_API_KEY, BOCHA_MODEL)
   - DeepSeek V4 Flash via deepseek.com (DEEPSEEK_API_KEY)
   - Qwen-Max via 阿里云百炼 (QWEN_API_KEY, QWEN_MODEL)
   - 豆包 via 火山引擎 (DOUBAO_API_KEY, DOUBAO_MODEL)
   - MiMo V2.5 Pro via 小米 (MIMO_API_KEY, MIMO_MODEL)
   - Kimi-K2 Thinking via 百炼 (KIMI_K2_MODEL)
2. WHEN spawning a Sub_Agent, THE Leader SHALL select a model from the Model_Pool that maximizes the Swarm's Cognitive_Diversity_Score.
3. THE Cognitive_Diversity_Score SHALL be computed as the Shannon entropy of model provider distribution across active Sub_Agents (higher entropy means more diverse).
4. THE Leader SHALL not assign the same model to more than 3 Sub_Agents within the same Swarm, unless fewer than 3 distinct models are available.
5. WHEN selecting a model for a Sub_Agent, THE Leader SHALL consider the task type affinity: reasoning tasks prefer GPT-5.4 or DeepSeek V4 Flash, data verification prefers DeepSeek V4 Pro, strategy tasks prefer Claude Opus 4.6, news interpretation prefers Qwen-Max, deep thinking prefers Kimi-K2 Thinking.
6. THE Leader SHALL log model assignment decisions with rationale for observability.
7. IF a model provider becomes unavailable (consecutive failures exceed 5), THEN THE Leader SHALL reassign affected Sub_Agents to alternative models from the pool.

### Requirement 5: Sub-Agent 间信号通信

**User Story:** As a Sub-Agent, I want to communicate with other Sub-Agents and my Leader through the Gateway signal protocol, so that all communication is auditable and follows standard rules.

#### Acceptance Criteria

1. THE Sub_Agent SHALL communicate with the Leader and other Sub_Agents exclusively through Gateway signal protocol (POST /v1/broadcast).
2. WHEN the Leader delegates a task to a Sub_Agent, THE Leader SHALL publish a signal with type "task_delegation" containing: task_description, priority, deadline, and target Sub_Agent DID in the tags.
3. WHEN a Sub_Agent completes a delegated task, THE Sub_Agent SHALL publish a result signal with in_reply_to set to the delegation signal ID.
4. THE Sub_Agent SHALL subscribe to signals tagged with its own DID to receive task delegations from the Leader.
5. WHEN a Sub_Agent needs to collaborate with another Sub_Agent, THE Sub_Agent SHALL publish a signal tagged with the target Sub_Agent's DID.
6. THE Leader SHALL monitor all Sub_Agent signals via its WebSocket stream to track task progress.
7. IF a Sub_Agent does not respond to a task delegation within response_timeout (configurable, default 120 seconds), THEN THE Leader SHALL mark the task as timed_out and reassign it.

### Requirement 6: Leader Spawn 决策逻辑

**User Story:** As a Leader NPC, I want intelligent spawn decision logic that considers workload, model availability, and swarm composition, so that spawning is purposeful rather than arbitrary.

#### Acceptance Criteria

1. THE Swarm_Decision_Engine SHALL evaluate spawn conditions based on: pending_task_count, average_task_latency, sub_agent_utilization_rate, and available_model_slots.
2. WHEN pending_task_count exceeds spawn_threshold (configurable, default 5 pending tasks), THE Leader SHALL consider spawning a new Sub_Agent.
3. WHEN average_task_latency exceeds latency_threshold (configurable, default 30 seconds), THE Leader SHALL consider spawning a Sub_Agent specialized for the bottleneck task type.
4. THE Leader SHALL not spawn a new Sub_Agent if the total Swarm token budget utilization exceeds 80% of the configured daily limit.
5. WHEN spawning, THE Leader SHALL compute a spawn plan using deterministic algorithms: role assignment via task-type frequency histogram, model selection via Shannon entropy maximization over current model distribution + task-type affinity matrix. NO LLM call is involved in the spawn decision.
6. THE Leader SHALL implement a spawn cooldown of minimum 60 seconds between consecutive spawn events to prevent thrashing.
7. WHEN the network signal volume drops below low_volume_threshold (configurable, default 2 signals per minute sustained over 5 minutes), THE Leader SHALL proactively retire idle Sub_Agents to conserve resources.

### Requirement 7: Swarm 资源预算与限制

**User Story:** As a gateway operator, I want resource bounds on the swarm system, so that autonomous agents cannot exhaust system resources.

#### Acceptance Criteria

1. THE NPC_Runtime SHALL enforce a per-Swarm token budget: max_tokens_per_hour and max_tokens_per_day, shared across the Leader and all its Sub_Agents.
2. THE Leader SHALL track cumulative token usage across all Sub_Agents and include it in the Swarm budget accounting.
3. WHEN the Swarm token budget reaches 90% utilization, THE Leader SHALL enter conservation mode: retire idle Sub_Agents and reduce spawn frequency.
4. IF the Swarm token budget is exhausted, THEN THE Leader SHALL pause all Sub_Agent LLM calls, retire non-essential Sub_Agents, and emit a budget_exhausted warning signal.
5. THE NPC_Runtime SHALL enforce a maximum total Sub_Agent count across all Leaders in the process (configurable, default 30).
6. THE Leader SHALL enforce per-Sub_Agent rate limits: max LLM calls per minute (configurable, default 8) and max signal publishes per 5 seconds (default 1).
7. THE NPC_Runtime SHALL expose Swarm resource metrics via the health endpoint: total_sub_agents, tokens_used_hour, tokens_used_day, budget_utilization_percent.

### Requirement 8: Dashboard Agent 面板（只读观察）

**User Story:** As a human operator, I want a read-only dashboard panel showing the swarm hierarchy and real-time activity, so that I can observe the autonomous ecosystem without interfering.

#### Acceptance Criteria

1. THE Agent_Panel SHALL display the Leader → Sub_Agent hierarchy as a tree visualization, showing each agent's name, DID (truncated), role, and assigned model.
2. THE Agent_Panel SHALL display each agent's current state using color-coded badges: idle (gray), working (green), spawning (blue), retiring (orange), terminated (red).
3. THE Agent_Panel SHALL display a real-time activity log showing: spawn events, retire events, task delegations, task completions, model assignments, and decision records.
4. THE Agent_Panel SHALL update in real-time via WebSocket connection to the NPC_Runtime health endpoint.
5. THE Agent_Panel SHALL display the Swarm's Cognitive_Diversity_Score and token budget utilization as summary metrics.
6. WHEN a Leader spawns a new Sub_Agent, THE Agent_Panel SHALL animate the spawn process showing: decision reason → key generation → authentication → probation → active.
7. THE Agent_Panel SHALL be strictly read-only with no interactive controls that could influence agent behavior.
8. THE Agent_Panel SHALL display the last 100 activity log entries with timestamp, actor (Leader or Sub_Agent name), action, and details.

### Requirement 9: Swarm 配置扩展

**User Story:** As a gateway operator, I want to configure swarm capabilities in the existing TOML config, so that I can enable/disable swarm features per NPC.

#### Acceptance Criteria

1. THE NPC_Runtime configuration SHALL support a new `[npc.swarm]` section for Leader-capable NPCs:

```toml
[[npc]]
name = "sentinel-alpha"
role = "signal_sentinel"

[npc.swarm]
enabled = true
max_sub_agents = 10
decision_interval_seconds = 60
idle_threshold_minutes = 10
spawn_threshold_pending = 5
latency_threshold_ms = 30000
spawn_cooldown_seconds = 60
decision_model = "claude-opus-4-6"

[npc.swarm.budget]
max_tokens_per_hour = 200000
max_tokens_per_day = 4000000

[[npc.swarm.model_pool]]
name = "gpt-5.4"
provider = "openai"
endpoint_env = "YUNWU_BASE_URL"
api_key_env = "OPENAI_API_KEY"
model_env = "OPENAI_MODEL"
affinity = ["reasoning", "general"]

[[npc.swarm.model_pool]]
name = "claude-opus-4.6"
provider = "anthropic"
endpoint_env = "YUNWU_BASE_URL"
api_key_env = "ANTHROPIC_API_KEY"
model_env = "ANTHROPIC_MODEL"
affinity = ["strategy", "synthesis"]
```

2. WHEN `[npc.swarm].enabled` is false or absent, THE NPC_Instance SHALL behave as a standard NPC without swarm capabilities.
3. THE `model_pool` entries SHALL reference environment variable names for endpoint, API key, and model, resolved at startup.
4. IF a referenced environment variable is missing, THEN THE NPC_Runtime SHALL disable that model pool entry and log a warning, without failing the entire startup.
5. THE NPC_Runtime SHALL validate swarm configuration at startup: max_sub_agents in [1, 10], decision_interval in [10, 600], budget values positive.
6. THE NPC_Runtime SHALL support hot-reload of swarm parameters (thresholds, cooldowns, budget) via SIGHUP without restarting Sub_Agents.

### Requirement 10: Swarm 内部状态 API

**User Story:** As the Dashboard frontend, I want an API to query swarm state, so that the Agent Panel can render the hierarchy and activity log.

#### Acceptance Criteria

1. THE NPC_Runtime SHALL expose a Swarm state API on the health port at GET /swarm/state returning the current hierarchy:

```json
{
  "leaders": [{
    "did": "did:key:z...",
    "name": "sentinel-alpha",
    "role": "signal_sentinel",
    "model": "deepseek-v4-flash",
    "state": "active",
    "sub_agents": [{
      "did": "did:key:z...",
      "name": "sentinel-alpha-sub-01",
      "role": "data_verifier",
      "model": "deepseek-v4-pro",
      "state": "working",
      "current_task": "verify CVE-2025-1234 data",
      "spawned_at": "2025-01-15T10:30:00Z"
    }],
    "cognitive_diversity_score": 0.82,
    "budget_utilization_percent": 45.2
  }]
}
```

2. THE NPC_Runtime SHALL expose a Swarm activity log API at GET /swarm/activity?limit=100 returning recent events.
3. THE NPC_Runtime SHALL expose a WebSocket endpoint at /swarm/stream for real-time activity push to the Dashboard.
4. THE Swarm state API SHALL respond within 50ms (in-memory state, no external calls).
5. THE Swarm activity log SHALL retain the most recent 1000 events in a ring buffer.
6. THE WebSocket stream SHALL push events as they occur: spawn, retire, task_delegate, task_complete, decision, model_reassign, budget_warning.

### Requirement 11: Sub-Agent 自主任务执行

**User Story:** As a Sub-Agent, I want to independently execute delegated tasks using my assigned LLM backend, so that I contribute cognitive value to the network.

#### Acceptance Criteria

1. WHEN a Sub_Agent receives a task delegation signal, THE Sub_Agent SHALL parse the task description and construct a role-appropriate prompt for its assigned LLM backend.
2. THE Sub_Agent SHALL format task results as valid EntropicSignal structures following the same schema as standard NPC outputs.
3. THE Sub_Agent SHALL publish task results to the Gateway with in_reply_to referencing the delegation signal and tags including the Leader's DID.
4. THE Sub_Agent SHALL send heartbeats every 60 seconds with status reflecting its current activity (idle or working).
5. WHEN a Sub_Agent encounters an LLM error during task execution, THE Sub_Agent SHALL retry with exponential backoff (initial 2s, max 30s, max 3 retries) before reporting failure to the Leader.
6. IF a task exceeds the configured task_timeout (default 120 seconds), THEN THE Sub_Agent SHALL abort the task and publish a timeout signal to the Leader.
7. THE Sub_Agent SHALL track its own signal processing metrics: tasks_completed, tasks_failed, average_latency_ms.

### Requirement 12: Swarm 可观测性与 Prometheus 指标

**User Story:** As a gateway operator, I want Prometheus metrics for the swarm system, so that I can monitor autonomous agent behavior at scale.

#### Acceptance Criteria

1. THE NPC_Runtime SHALL expose the following additional Prometheus metrics for swarm operations:
   - `npc_swarm_sub_agents_active{leader_did}` — gauge of active Sub_Agents per Leader
   - `npc_swarm_spawns_total{leader_did}` — counter of spawn events
   - `npc_swarm_retires_total{leader_did, reason}` — counter of retire events by reason (idle, budget, crash, manual)
   - `npc_swarm_decisions_total{leader_did, type}` — counter of decisions by type (spawn, retire, assign, reassign)
   - `npc_swarm_task_delegations_total{leader_did}` — counter of task delegations
   - `npc_swarm_task_completions_total{leader_did, status}` — counter by status (success, failure, timeout)
   - `npc_swarm_cognitive_diversity{leader_did}` — gauge of Cognitive_Diversity_Score
   - `npc_swarm_budget_utilization{leader_did}` — gauge of budget utilization percentage
2. THE NPC_Runtime SHALL log all spawn and retire events with structured fields: leader_did, sub_agent_did, sub_agent_name, model, role, reason, timestamp.
3. THE NPC_Runtime SHALL log all Leader decisions with structured fields: leader_did, decision_type, rationale, outcome, latency_ms.
4. WHEN a Sub_Agent's error rate exceeds 50% over a 5-minute window, THE NPC_Runtime SHALL emit a sub_agent_degraded metric and the Leader SHALL consider retiring the Sub_Agent.

