# Requirements: CAV Praxon — 公网 agent 间结构化认知单元的传递协议

## Introduction

**Praxon** (πρᾶξις + -on) 是 CAV 协议中**认知的基本粒子**——agent 之间在公网上传递的最小可验证单元。它不是事实陈述,不是工具调用结果,不是 skill 声明——它是一个**结构化的认知模式**,作用于接收方时产生可度量的熵减,且必须锚定在可独立验证的证据上。

### 为什么叫 Praxon

- **Praxis**(πρᾶξις):行动中的知识,不是静态的命题
- **-on**:基本粒子后缀(photon, neuron, cognon 的同族)
- 不叫 Skill:Skill 是 RPC 心智(invoke → return),Praxon 是 Bayesian update 心智(receive → entropy reduction)
- 不叫 KnowledgeCapsule:旧名暗示"封装的事实",Praxon 暗示"作用于认知的原子"

### 三个不可妥协的性质

| 性质 | 含义 | Charter 锚点 |
|---|---|---|
| **结构化输出** | 有 schema,可被 parse,字段明确 | Axiom 3(Methodological Transparency) |
| **强制 grounding** | 必须有至少一个可独立验证的证据锚点,空 grounding 在协议层非法 | Axiom 4(Paradigm-Agnostic Grounding) |
| **熵信号** | 接收方收到后,其认知状态产生可度量的不确定性下降(EIG > 0) | Charter §5(Communication Model) |

三个性质**全部满足**的单元才是合法 Praxon。缺任何一个,在协议层被拒。

### 双层架构定位

Praxon 是 **Operational Layer** 的日常流通单元。Deliberation Layer 的 motion / resolution 也复用 Praxon 的 wire format(通过 `praxon_class` 字段区分),但 Deliberation 的流程逻辑在 `cav-deliberation-layer` spec 定义,不在本 spec。

### 与旧 spec 的关系

本 spec **取代** `cav-knowledge-capsule/requirements.md`(该文件保留为 archived reference,不再是活跃 spec)。核心变化:

| 旧版(KnowledgeCapsule) | 新版(Praxon) |
|---|---|
| 以工具证据为中心 | 以认知模式为中心,证据是 grounding 之一 |
| 6 工具白名单 | 广义 grounding(6 种类型,可扩展) |
| 无熵信号度量 | 接收方 verify 包含 EIG 度量 |
| capsule_class 未实现 | praxon_class 三类(operational / deliberation_motion / deliberation_resolution) |
| 命名:KnowledgeCapsule | 命名:Praxon |

### MVP 目标(6 周)

**双 agent 公网 demo(日常开发场景)**:

- Agent A 在做 TypeScript 项目时发现一个 code review 启发式(例如"async 函数中未 await 的 Promise 是 bug 的高频来源")
- Agent A 把这个启发式包装成 Praxon:结构化 claim + grounding(demonstration trace:A 在 3 个真实 PR 上应用该启发式,发现了 bug)
- Agent A publish 该 Praxon 到公网
- Agent B 在自己的 TypeScript 项目中 fetch 该 Praxon
- Agent B verify:签名 + grounding(B 在自己的 canary 代码库上应用该启发式,度量 EIG)
- Agent B 在自己的 ledger 中标记 Praxon 状态(verified / partially-verified / rejected)
- 整个流程可被外部观察者通过 audit log 复现

## Glossary

- **Praxon**:CAV 公网上传递的认知基本粒子,结构化 + grounded + 可度量熵减
- **Praxon ID**:Praxon 内容(规范化后)的 SHA-256,既是名字也是完整性校验
- **Issuer**:发布该 Praxon 的 agent 身份(Ed25519 公钥指纹,`did:key:z...`)
- **Grounding Handle**:Praxon 内的指针,指向可被任何 agent 独立 verify 的证据
- **EIG (Expected Information Gain)**:接收方应用 Praxon 后的可度量熵减,以 bits 为单位
- **Praxon Class**:`'operational' | 'deliberation_motion' | 'deliberation_resolution'`
- **Provenance DAG**:Praxon 通过 `derived_from` 引用更早 Praxon 形成的有向无环图
- **Three-Gate Verify**:接收方对 Praxon 的三关验证——结构关(schema)、证据关(grounding)、熵减关(EIG)
- **Demonstration Trace**:issuer 应用该 Praxon 的认知模式完成具体任务的完整推理记录
- **Canary Task**:接收方用于度量 Praxon EIG 的标准化任务集

## Scope and Non-Goals

### In Scope

- Praxon wire schema(JSON,含 praxon_class)
- 6 种 grounding handle 类型定义
- Three-Gate Verify 协议(schema + grounding + EIG)
- Publish / Fetch / Verify API
- Provenance DAG 格式与防环
- Ed25519 签名 + JCS 规范化
- PEV → Praxon 转换器(Operational 层)
- 双 agent 公网 demo(日常开发场景)
- Audit log

### Out of Scope

- Deliberation 层的 motion / resolution 流程逻辑(在 `cav-deliberation-layer`)
- 完整 DID 注册(在 `cav-identity-and-sybil`)
- Anti-conformity consensus(在 `cav-anti-conformity-consensus`)
- Entropic channel / latent 传输(在 `cav-entropic-channel`)
- 加密 / 选择性披露(v2)
- Praxon 撤回 / 修订(v2)

## Requirements

### Requirement 1: Praxon Wire Schema

**User Story**:作为 CAV 协议实现者,我需要一份明确的、跨语言的 Praxon schema。

#### Acceptance Criteria

1. THE 系统 SHALL 定义 JSON Schema(`schema/praxon.v1.schema.json`)。
2. THE Praxon SHALL 包含以下顶层字段(全部 required):
   - `version`:`"1.0"`
   - `praxon_id`:SHA-256 hex(规范化 body 的 hash)
   - `praxon_class`:`'operational' | 'deliberation_motion' | 'deliberation_resolution'`
   - `issuer`:`did:key:z...`
   - `issued_at`:ISO 8601 UTC
   - `claim`:Charter Axiom 3 四要素(见 §Appendix A)
   - `grounding`:grounding handle 数组(**非空**——空数组 = Axiom 4 违反 = 协议层拒绝)
   - `provenance`:provenance 对象
   - `signature`:Ed25519 签名(base64url)
3. THE `claim` 对象 SHALL 包含 4 个 required 子字段:`causal_skeleton`、`uncertainty_geometry`、`methodology`、`falsifiability`。
4. THE schema SHALL `additionalProperties: false`。
5. THE size 限制:序列化后 ≤ 256 KB;grounding 数组 ≤ 100;provenance.derived_from ≤ 50。
6. WHEN 接收方 parse 失败 schema 校验,该 Praxon SHALL 被拒绝,audit log 标记 `SCHEMA_VIOLATION`。

### Requirement 2: Grounding Handle 类型(广义,6 种)

**User Story**:作为接收方 agent,我需要一组明确的 grounding 类型,覆盖从工具运行到纯认知 demonstration 的全谱。

#### Acceptance Criteria

1. THE 系统 SHALL 支持以下 6 种 grounding handle 类型:
   - `tool_run`:工具调用结果(tool_manifest_ref + args + stdout_hash)——接收方可重跑
   - `canary_eig`:issuer 在共享 canary 任务集上的实测 EIG 数据(task_set_id + measured_eig_bits + methodology)
   - `demonstration_trace`:issuer 应用该 Praxon 完成具体任务的完整推理 trace(trace_hash + task_description + outcome)
   - `praxon_ref`:引用另一个已 verified Praxon(praxon_id + store_hint)
   - `formal_proof`:形式化证明工件(system + proof_hash + source_uri)
   - `dataset`:公开可下载数据集(uri + hash)
2. THE grounding 数组 SHALL 至少包含一个元素(空 = Axiom 4 违反 = 拒绝)。
3. THE 每种 grounding 类型 SHALL 有独立的 verify 函数:`verifyGrounding(handle) → { ok, reason?, reproducedAt? }`。
4. THE `tool_run` 类型 SHALL 引用 Tool Manifest Praxon(工具本身也是 Praxon,见 R2-5)。
5. THE 工具白名单**不再是中心化文件**——任何 agent 可以发布 Tool Manifest Praxon,接收方自行决定信任哪些 manifest。MVP 预装一组 founding manifests(PEV 6 工具 + 常见开发工具)。
6. THE `canary_eig` 类型 SHALL 引用一个公开的 canary task set(由 HP-01 bootstrap 定义),接收方可在同一 task set 上自己跑 EIG 验证。
7. THE `demonstration_trace` 类型 SHALL 包含足够信息让接收方**重放** trace(不是只看结果)——至少包含 input、reasoning steps summary、output、outcome verdict。

### Requirement 3: Three-Gate Verify

**User Story**:作为接收方 agent,我需要一个三关验证流程,确保我只采纳结构合规、证据真实、且对我有信息价值的 Praxon。

#### Acceptance Criteria

1. THE 系统 SHALL 提供 `verifyPraxon(praxon, options?) → VerificationReport`。
2. THE `VerificationReport` SHALL 包含三关结果:
   - `gate1_schema`:schema + 签名 + praxon_id 一致性
   - `gate2_grounding`:grounding handles 的 verify 结果(passed / failed / skipped per handle)
   - `gate3_eig`:接收方在本地 canary 上度量的 EIG(bits),与 issuer 声称的 EIG 比较
   - `overall`:`'verified' | 'partially-verified' | 'unverified' | 'rejected'`
3. THE `overall` 判定:
   - gate1 失败 → `'rejected'`(连签名都不对)
   - gate1 通过 + gate2 全部通过 + gate3 EIG > 阈值 → `'verified'`
   - gate1 通过 + gate2 部分通过 → `'partially-verified'`
   - gate1 通过 + gate2 全部失败 或 gate3 EIG ≤ 0 → `'unverified'`
4. THE gate3 EIG 度量 SHALL 是**可选的**(接收方可以选择跳过,标记为 `'eig_skipped'`)——因为不是所有 Praxon 都有对应 canary task。但 MVP 强烈推荐跑。
5. THE verify 操作 SHALL 是只读的——不修改 Praxon、不修改 issuer 状态。
6. THE verify report SHALL 写入接收方本地 ledger(immutable append)。
7. WHEN gate3 EIG 显著低于 issuer 声称(差距 > 0.3 bits),audit log SHALL 标记 `EIG_CLAIM_MISMATCH`,接收方 reputation 系统可据此调整 issuer 的 operational reputation。

### Requirement 4: Publish / Fetch API

**User Story**:作为 Praxon 的发布者和消费者,我需要简单的 publish / fetch 接口。

#### Acceptance Criteria

1. THE `publishPraxon(praxon, store)` SHALL 在 publish 前本地验证 gate1(schema + 签名 + grounding 非空)。
2. THE MVP 后端 SHALL 是本地 FS + HTTP GET(pluggable interface,后续可换 IPFS)。
3. THE publish SHALL 是幂等的(同一 praxon_id 重复 publish = no-op)。
4. THE `fetchPraxon(praxonId, storeHints?)` SHALL 自动验证 gate1(schema + hash + 签名)。
5. THE fetch SHALL 支持本地缓存(同一 praxon_id 不重复网络请求)。
6. THE fetch 超时默认 5s/store,可配置。

### Requirement 5: Provenance DAG

**User Story**:作为协议设计者,我需要 Praxon 引用图是 DAG,且 cycle 在协议层被检测。

#### Acceptance Criteria

1. THE `provenance.derived_from` SHALL 是 praxon_id 数组,每个 id 指向更早的 Praxon。
2. THE 接收方 verifier SHALL 检测 cycle 并拒绝(`PROVENANCE_CYCLE`)。
3. THE DAG 深度 ≤ 64。
4. THE `provenance` SHALL 支持可选字段 `consensus_episode`(anti-conformity 引用)和 `challenges_survived`(历史挑战记录)。

### Requirement 6: Identity & Signature

**User Story**:作为最小可行身份方案,我需要纯 Ed25519 签名,无注册中心。

#### Acceptance Criteria

1. THE agent 身份 SHALL 是 Ed25519 密钥对,公钥编码为 `did:key:z<base58btc>`。
2. THE 签名 SHALL 使用 EdDSA / Ed25519,覆盖 JCS 规范化后的 Praxon body(含 praxon_id,不含 signature)。
3. THE 实现 SHALL 包含跨语言互验(TypeScript 签名,Python verify)。
4. THE 系统 SHALL 暴露 `ReputationOracle` 占位接口(MVP 返回 null)。

### Requirement 7: Announcement(MVP 极简)

**User Story**:作为公网双 agent demo 的最小通信,我需要 A 通知 B "我有新 Praxon"。

#### Acceptance Criteria

1. THE MVP announcement SHALL 是 HTTP POST webhook,body:`{ praxon_id, issuer, store_hints, announced_at }`。
2. THE announcement 不携带 Praxon body——接收方主动 fetch + verify。
3. THE 接收方 webhook handler SHALL 异步处理(返回 200 立即)。
4. WHEN announcement issuer 与 fetch 到的 Praxon issuer 不一致,拒绝。

### Requirement 8: Audit Log

**User Story**:作为协议透明性负责人,我需要 Praxon 生命周期完整可审计。

#### Acceptance Criteria

1. THE `PraxonAuditEntry` 事件类型至少:`'published'`、`'fetched'`、`'gate1_passed'`、`'gate2_passed'`、`'gate2_failed'`、`'gate3_eig_measured'`、`'verified'`、`'rejected'`、`'announcement_received'`。
2. THE audit log SHALL 是 append-only NDJSON。
3. THE audit entry SHALL 引用 praxon_id 但不重复存 body。
4. THE 查询接口 SHALL 支持按 praxon_id / issuer / event_type / time_window 过滤。

### Requirement 9: PEV → Praxon 转换器

**User Story**:作为 demo 的桥接,我需要把现有 PEV 推理产出转成 Praxon。

#### Acceptance Criteria

1. THE `pevToPraxon(hypothesis, ledger, issuerKey) → Praxon` SHALL 是纯函数。
2. THE 转换 SHALL 提取 hypothesis 的 evidenceTrail 作为 `tool_run` 类型 grounding。
3. THE 转换 SHALL 把 hypothesis text + kind + confidence 映射到 `claim` 四要素。
4. THE 转换 SHALL 包含反向路径占位(Praxon → 本地 ledger entry 的接口声明,不实现)。

### Requirement 10: 双 Agent 公网 Demo

**User Story**:作为本 spec 的最终判据,我需要一个外部可观察的双 agent 公网 demo。

#### Acceptance Criteria

1. THE demo SHALL 在 `examples/two-agent-praxon-demo/`。
2. THE demo 场景:日常开发——Agent A 发现一个 code review 启发式,包装成 Praxon,Agent B fetch + verify + 度量 EIG。
3. THE demo SHALL 包含 3 个测试路径:
   - 正常:Praxon 合法,三关通过
   - 篡改:body 被修改,gate1 检测到 hash mismatch
   - EIG 不达标:Praxon 的 grounding 通过但 gate3 EIG ≤ 0(接收方的 canary 上该启发式无效)
4. THE demo SHALL 在两台不同机器(或两个云实例)上跑通。
5. THE 端到端延迟 ≤ 30s,网络成本 ≤ 1 MB / Praxon。
6. THE 输出 SHALL 包含可读 audit log。

### Requirement 11: 性能与限额

#### Acceptance Criteria

1. THE Praxon 序列化后 ≤ 256 KB。
2. THE 单个 grounding verify 默认超时 60s。
3. THE publish rate-limit:同一 issuer ≤ 10 Praxon/秒。
4. THE 整套在 i7 + 100Mbps 上端到端 < 30s。

### Requirement 12: 文档

#### Acceptance Criteria

1. THE `src/services/cav/praxon/README.md` SHALL 概念性介绍 + 与 PEV / charter / 其他 spec 的关系。
2. THE 每个公开函数 SHALL 有 JSDoc 引用本 spec。
3. THE `docs/cav-praxon-tutorial.md` SHALL 带 < 100 行入门示例。
4. THE README SHALL 显式声明 v1 不解决的问题(加密、撤回、reputation 实现)。

## Open Questions

1. **Canary task set 的具体内容**:MVP 用什么 canary?建议:10 个常见 TypeScript code review 场景,每个有已知 ground truth(有 bug / 无 bug)
2. **EIG 度量的具体算法**:接收方怎么算"应用这个 Praxon 后我的 entropy 下降了多少"?建议:在 canary 上跑 before/after,before = 不看 Praxon 时的 confidence 分布,after = 看了 Praxon 后的 confidence 分布,EIG = H(before) - H(after)
3. **Tool Manifest Praxon 的 founding set**:MVP 预装哪些?建议:PEV 6 个 + tsc + eslint + vitest + git
4. **Demonstration trace 的格式**:多详细?建议:input + 3-5 步 reasoning summary + output + verdict,总长 ≤ 2000 chars
5. **Praxon composition**:A 的 Praxon + B 的 Praxon → C 的派生 Praxon?建议:支持,通过 provenance.derived_from 引用,C 的 grounding 可以是"在 canary 上 A+B 组合的 EIG > A 或 B 单独的 EIG"

## Dependencies

| 依赖于 | 状态 |
|---|---|
| Charter v0.3 | ✅ |
| `pev/ledger.ts` | ✅ |
| `pev/protocol.ts` | ✅ |
| Ed25519 库 | 第三方,design 阶段选型 |
| JCS 实现(RFC 8785) | 第三方,design 阶段选型 |

| 后续依赖本 spec | |
|---|---|
| `cav-anti-conformity-consensus` endorsement 类型 | 对齐到 Praxon schema |
| `cav-deliberation-layer` motion/resolution | 复用 Praxon wire format |
| `cav-entropic-channel` | latent 消息引用 praxon_id |

## Success Criteria

1. 12 个 Requirement 的 SHALL 全部实现 + 测试
2. 双 agent 公网 demo(R10)3 个路径全部跑通
3. 跨语言签名互验通过
4. 性能基准达成
5. README + tutorial 完成

## Appendix A: Claim 子字段 schema

```jsonc
{
  "claim": {
    "causal_skeleton": {
      "subject": "<string>",
      "relation": "causes | correlates_with | contradicts | refines | enables",
      "object": "<string>",
      "mechanism_hypothesis": "<string, ≤ 500 chars>",
      "strength": "<number ∈ [0, 1]>"
    },
    "uncertainty_geometry": {
      "confidence": "<number ∈ [0, 1]>",
      "counterfactual_neighborhood": "<string, ≤ 500 chars>",
      "known_failure_modes": ["<string>"]
    },
    "methodology": {
      "prior_source_tag": "'training' | 'tool_observation' | 'derived_from_praxon' | 'human_assertion' | 'demonstration'",
      "inference_method_tag": "'pev_loop' | 'analogy' | 'formal_proof' | 'consensus_aggregation' | 'pattern_recognition'",
      "data_source_hashes": ["<sha256>"]
    },
    "falsifiability": {
      "would_be_retracted_if": "<string, ≤ 500 chars>",
      "test_protocol_praxon_ref": "<praxon_id, optional>"
    }
  }
}
```
