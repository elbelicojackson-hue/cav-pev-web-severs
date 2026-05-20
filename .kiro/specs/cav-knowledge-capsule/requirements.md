# Requirements: CAV Knowledge Capsule — 公网 agent 间可验证知识传递的 wire 层

## Introduction

本 spec 定义 CAV 协议在公网上**两个 agent 之间传递已学过、已认证知识结构**的最小可行 wire 协议。这是 Charter 之后第一个让 CAV "**走出单机进入公网**"的 sub-spec,也是后续所有 CAV 子系统(consensus、identity、entropic channel、explanation bridge)的**底层载体**。

### 为什么先做这个

CAV Charter 已经声明:agent 间传递的是**认知结构**(causal skeleton + uncertainty + methodology + falsifiability + grounding),不是领域内容。Charter 也声明每个 claim 必须**可挑战**、**可被独立 verify**。

但 charter 没有定义这些声明在公网上**长什么样**——具体的字节流、签名格式、寻址方式、传输协议。没有这一层,所有上层 spec(anti-conformity consensus 等)都只能在单进程内运行,**无法成为真正的"协议"**。

KnowledgeCapsule 是 Charter 概念的 wire 兑现。

### 与 anti-conformity consensus 的关系

`cav-anti-conformity-consensus` 的 endorsement 类型在最终形态会是 KnowledgeCapsule 的一种特化。本 spec 的产出会让该 sub-spec 在 design 阶段重新锚定到 wire 层。**两个 spec 并行进入 design 之前,本 spec 必须先于它落地**——否则 anti-conformity 会基于内存对象建模,后续公网化要重写。

### 现状校准

- `pev/ledger.ts` **存在**——hypothesis + evidence + grounding 的内存数据模型已实现
- `pev/protocol.ts` **存在**——`Verdict` / `HypothesisId` / `HypothesisKind` 等基础类型已实现
- 公网 wire format **不存在**——本 spec 是绿地
- Identity / signature / capsule store **均不存在**——本 spec 不实现 reputation oracle 与 anti-conformity consensus,但定义它们与 KnowledgeCapsule 的接口

### MVP 目标(6 周)

**双 agent 公网 demo**:

- 两台不同机器(或两个云实例)上各跑一个 CAV agent
- Agent A 完成一段 PEV 推理,产出一份 KnowledgeCapsule
- Agent A 通过 HTTPS publish 到 capsule store
- Agent B 通过 HTTPS fetch 该 capsule
- Agent B 独立运行 grounding 验证(自己跑同样的工具,比对 hash)
- Agent B 在自己的本地 PEV ledger 中标记该 capsule 为 `B-verified` 或 `B-rejected`
- 整个流程可被外部观察者通过 audit log 复现

不达成这个目标,本 spec 标记 `Failed`,Phase 1 路线图重新评估。

### 默认技术栈(已选定,后续可演化)

| 层 | 选择 | 理由 |
|---|---|---|
| 传输 | HTTPS + JSON | 调试友好,生态成熟,后续可换 IPFS |
| Identity | Ed25519 公钥指纹(`did:key` 风格) | 无注册中心,纯密码学,标准已存 |
| 签名 | EdDSA over JCS(RFC 8785) | 标准化的 JSON 规范化 + 标准化签名 |
| Hash | SHA-256 | 通用、所有语言都有 |
| Capsule store | 本地 FS + HTTP GET(MVP),IPFS 留接口 | 6 周内能跑通的最简后端 |

## Glossary

- **KnowledgeCapsule**:CAV 公网上传递的知识结构最小单元,内容寻址 + 签名后的不可变 JSON 文档
- **Capsule ID**:capsule 内容(规范化后)的 SHA-256,既是名字也是完整性校验
- **Issuer**:发布该 capsule 的 agent 身份,由 Ed25519 公钥指纹标识
- **Grounding Handle**:capsule 内的指针,指向可被任何 agent 独立 fetch + verify 的证据(数据集 / 工具运行 / 上游 capsule)
- **Provenance DAG**:capsule 通过 `derived_from` 字段引用更早 capsule 形成的有向无环图,描述知识的来源链
- **Verification**:接收方 agent 不依赖 issuer say-so,而是通过重新运行 grounding 步骤,自己得出"这个 capsule 的 claim 是否站得住"的判断
- **Capsule Store**:capsule 的对外 fetch 接口,MVP 是 HTTP GET,长期可演化为 IPFS / DHT / pluggable backend
- **Announcement**:agent 通知对方"我有新 capsule"的消息;MVP 是 HTTP webhook,长期可演化为 entropic channel
- **Reputation Oracle**(占位):capsule issuer 的可信度查询接口,本 spec 只定义接口,不实现
- **Canonical Form**:capsule JSON 在签名前的标准化表示(JCS / RFC 8785),保证 issuer 与 verifier 算出同样的 hash

## Scope and Non-Goals

### In Scope

- KnowledgeCapsule 完整 wire schema(JSON)
- 规范化、签名、hash 算法定义
- Publish / Fetch / Verify 三个核心 API
- Grounding handle 的类型枚举与 fetch 协议
- Provenance DAG 的格式与防环规则
- Reputation oracle 接口(只接口,不实现)
- 双 agent 公网 demo 实现 + 测试

### Out of Scope(留给后续 spec)

- IPFS / DHT / 去中心化存储后端(留给 `cav-storage-backends`)
- 完整 DID 注册与命名系统(留给 `cav-identity-and-sybil`,即 HP-03)
- 实时双向 sync / 增量更新(留给 `cav-entropic-channel`)
- Latent embedding 传输(留给 HP-04 cluster-内部协议)
- Anti-conformity consensus 实现(已有 spec)
- Reputation oracle 的实际实现(留给 HP-03 + 后续)
- Capsule 撤回 / 反悔 / 修订机制(留给 v2,初版 capsule 是不可变的)
- 加密 / 选择性披露(留给 `cav-selective-disclosure`,初版 capsule 是 public-readable)

本 spec **故意**不解决加密。初版假设公开可读,因为 CAV 的核心价值是**透明可挑战**——加密让挑战更难。选择性披露是后续 feature,不是基础。

## Requirements

### Requirement 1: KnowledgeCapsule Wire Schema

**User Story**:作为 CAV 协议实现者,我需要一份明确的、跨语言的 capsule schema,这样不同实现可以互操作。

#### Acceptance Criteria

1. THE 系统 SHALL 定义 JSON Schema(`schema/knowledge-capsule.v1.schema.json`),覆盖所有合法 capsule 的结构。
2. THE capsule SHALL 包含以下顶层字段(全部 required):
   - `version`:协议版本号字符串(初版 `"1.0"`)
   - `capsule_id`:SHA-256 hex 串,等于规范化 capsule body(去除 `capsule_id` 与 `signature` 后)的 hash
   - `issuer`:Ed25519 公钥指纹(`did:key:z...` 格式)
   - `issued_at`:ISO 8601 UTC 时间戳
   - `claim`:Charter Axiom 3 四要素对象
   - `grounding`:grounding handle 数组(空数组**禁止**——见 R2-3)
   - `provenance`:provenance 对象
   - `signature`:Ed25519 签名(base64url),覆盖规范化 capsule body
3. THE `claim` 对象 SHALL 包含 4 个 required 子字段:`causal_skeleton`、`uncertainty_geometry`、`methodology`、`falsifiability`。每个子字段的内部 schema 由本 spec §Appendix A 定义。
4. THE schema SHALL 拒绝任何未在 schema 中声明的额外顶层字段(`additionalProperties: false`),以防 forward-compat 引发的歧义签名。
5. THE schema SHALL 标注每个字段的 size / length 上限(防 DoS):
   - `claim` 序列化后 ≤ 64 KB
   - `grounding` 数组长度 ≤ 100
   - `provenance.derived_from` 数组长度 ≤ 50
6. WHEN 任何接收方解析 capsule 失败 schema 校验,该 capsule SHALL 被拒绝(不进入 verification 流程),并在 audit log 标记 `SCHEMA_VIOLATION`。
7. THE schema 文件 SHALL 同时存在于 `src/services/cav/capsule/schema/` 与 `.kiro/specs/cav-knowledge-capsule/schema/`(冗余存放,方便外部读者引用)。

### Requirement 2: Grounding Handle 类型与 fetch 协议

**User Story**:作为接收方 agent,我需要一组明确定义的 grounding handle 类型,这样我能用同一套代码独立 verify 不同来源的证据。

#### Acceptance Criteria

1. THE 系统 SHALL 支持以下 grounding handle 类型(MVP):
   - `dataset`:`{ type: 'dataset', uri: <https url>, hash: <sha256> }`——一个公开可下载的数据集,接收方下载后 hash 比对
   - `tool_run`:`{ type: 'tool_run', tool_id: <canonical name>, args_hash: <sha256>, stdout_hash: <sha256>, stderr_hash?: <sha256>, exit_code: number }`——一次工具调用的结果,接收方在自己机器上重跑,比对 stdout/stderr hash
   - `capsule_ref`:`{ type: 'capsule_ref', capsule_id: <sha256>, store_hint?: <url> }`——引用另一个 capsule 作为 grounding,接收方需要递归 fetch + verify
   - `formal_proof`:`{ type: 'formal_proof', system: <e.g. "lean4">, proof_hash: <sha256>, source_uri: <url> }`——形式化证明工件,接收方需要用对应证明系统 type-check
2. THE grounding 数组 SHALL 至少包含一个元素(空数组是 Axiom 4 违反,直接拒绝)。
3. THE 接收方 verifier SHALL 为每种 grounding 类型实现 `verifyGrounding(handle) → { ok: boolean; reason?: string; reproducedAt?: ISO8601 }`。
4. WHEN `tool_run` 的 `stdout_hash` 比对失败,THE verifier SHALL 返回 `{ ok: false, reason: 'TOOL_OUTPUT_HASH_MISMATCH' }`,并在 audit log 记录两侧 hash 供调试。
5. WHEN `capsule_ref` 形成循环依赖(A → B → A),THE verifier SHALL 检测到 cycle 并返回 `{ ok: false, reason: 'PROVENANCE_CYCLE' }`。
6. THE `tool_run` 类型 SHALL 包含 `tool_id` 必须从一个**封闭白名单**(`canonical-tools.v1.json`)选取——不允许任意 shell 命令作为 grounding(防止 RCE 类攻击)。MVP 白名单包含 `pev/canonicalTests.ts` 中已有的 6 个工具。
7. THE 工具白名单 SHALL 每个工具明确其 `args_schema` 与 `expected_runtime_ms`(供接收方决定是否值得重跑)。

### Requirement 3: Capsule 规范化(Canonicalization)

**User Story**:作为签名实现者,我需要保证 issuer 与 verifier 算出**完全相同**的字节序列用于签名,否则签名永远 mismatch。

#### Acceptance Criteria

1. THE 系统 SHALL 使用 RFC 8785(JCS)规范化 capsule 在签名前的字节表示。
2. THE 规范化 SHALL **从 capsule body 中移除** `capsule_id` 与 `signature` 字段后再计算。
3. THE `capsule_id` 计算流程:`sha256(canonicalize(capsule_body_without_id_and_signature))`。
4. THE 签名计算流程:`Ed25519.sign(issuer_private_key, canonicalize(capsule_body_without_signature))`。注意:签名时 `capsule_id` **已经被填充**——签名是覆盖含 `capsule_id` 但不含 `signature` 的版本。
5. THE 实现 SHALL 包含基于属性的测试:对随机 capsule(合法字段空间内)进行 N=1000 次序列化-解析-再序列化,所有中间步骤的规范化结果 byte-for-byte 一致。
6. WHEN 两个不同实现(MVP 内只有 TypeScript 一个,但测试用 Python 验证)对同一 capsule 算出不同 `capsule_id`,该 spec 标记为**实现违规**——RFC 8785 的实现质量是 spec 合规的硬要求。

### Requirement 4: Publish API

**User Story**:作为 issuer agent,我需要一个简单的 API 把已签名 capsule 发布到 capsule store,让其他 agent 可以 fetch。

#### Acceptance Criteria

1. THE 系统 SHALL 提供 `publishCapsule(capsule, store) → { ok: true; capsuleUri: string } | { ok: false; reason: string }`。
2. THE publish 操作 SHALL 在 publish 前本地验证:schema、签名、grounding 至少存在 1 个、capsule_id 与内容一致。任何一项失败立刻 abort。
3. THE MVP 后端 SHALL 是本地文件系统(`<store_root>/<capsule_id>.json`)+ HTTP GET 静态服务。
4. THE store backend SHALL 是 pluggable interface(`CapsuleStore` 类型),IPFS / S3 / DHT 都可以是后续实现,本 spec 只要求接口正确。
5. THE publish 操作 SHALL 是幂等的:重复 publish 同一 `capsule_id` 不应报错,只是 no-op(因为 capsule 不可变,内容相同则结果相同)。
6. WHEN 网络中断 / 后端不可用,THE publish SHALL 返回 `{ ok: false, reason: 'STORE_UNAVAILABLE' }`,不静默丢弃。
7. THE 实现 SHALL 不依赖任何中心化注册服务——capsule 一旦写入 store,就可被任何知道 capsule_id 的 agent fetch。

### Requirement 5: Fetch API

**User Story**:作为接收方 agent,我需要一个 API,通过 capsule_id 和可选的 store hint 获取 capsule 字节,并自动校验完整性。

#### Acceptance Criteria

1. THE 系统 SHALL 提供 `fetchCapsule(capsuleId, storeHints?) → { ok: true; capsule: KnowledgeCapsule } | { ok: false; reason: string }`。
2. THE fetch 操作 SHALL 自动验证:
   - schema 合规
   - 接收到的内容规范化后 hash 等于 capsuleId(完整性)
   - signature 在 issuer 公钥下验证通过(签名完整性)
3. WHEN 任意一项验证失败,THE fetch SHALL 返回明确 `reason`,**不**返回那份 capsule(防止上层错误使用未经校验的数据)。
4. THE fetch 操作 SHALL 支持多个 storeHints,按顺序尝试,首个成功者返回。MVP 只需 HTTP store hint。
5. THE fetch 操作 SHALL 包含可配置超时(默认 5s/store),超时不阻塞调用方。
6. THE 实现 SHALL 提供本地缓存接口(可选,默认开启):同一个 capsule_id 多次 fetch 不重复网络请求。
7. THE 缓存命中 / miss SHALL 在 audit log 中可观测。

### Requirement 6: Verify API(grounding 重新认证)

**User Story**:作为接收方 agent,我需要不依赖 issuer say-so,自己用 grounding handle 把 capsule 的 claim **重新认证一遍**。

#### Acceptance Criteria

1. THE 系统 SHALL 提供 `verifyCapsule(capsule, options?) → VerificationReport`。
2. THE `VerificationReport` SHALL 包含:
   - `signature_ok`:签名 + 完整性是否通过
   - `groundings_attempted`:已尝试 verify 的 grounding 数
   - `groundings_passed`:通过 verify 的数量
   - `groundings_failed`:每个失败 grounding 的 `{ index, reason }`
   - `overall`:`'verified' | 'partially-verified' | 'unverified' | 'rejected'`
   - `reproduced_at`:每个成功 grounding 的本地复现时间戳
3. THE `overall` 判定逻辑:
   - `signature_ok=false` → `rejected`(连签名都不对,放弃)
   - `signature_ok=true && groundings_failed.length=0 && groundings_passed.length=groundings.length` → `verified`
   - `signature_ok=true && groundings_passed.length≥1 && groundings_failed.length≥1` → `partially-verified`
   - `signature_ok=true && groundings_passed.length=0` → `unverified`
4. THE verify 操作 SHALL 可选超时上限(默认 5 分钟/capsule),长 grounding(如重跑大规模工具)可异步。
5. THE verify 操作 SHALL 是**只读**的——不修改 capsule、不修改 issuer 状态,只产出 report。
6. THE verify 操作 SHALL 把 report 写入接收方本地 ledger(类似 `pev/ledger.ts` 风格的 immutable append),供后续推理引用。
7. WHEN 接收方决定**部分信任** issuer,允许跳过部分 grounding(配置项 `trustLevel`):`'paranoid'`(全部重 verify)、`'normal'`(只 verify capsule_ref 链与 tool_run)、`'light'`(只验签名 + capsule_id)。MVP 强烈推荐 `paranoid`。

### Requirement 7: Provenance DAG 与防环

**User Story**:作为协议设计者,我需要保证 capsule 引用图是 DAG 而不是 cycle,且 cycle 检测在协议层强制。

#### Acceptance Criteria

1. THE `provenance.derived_from` SHALL 是 capsule_id 数组,每个 id 必须指向一个早于本 capsule 的 issued_at 的 capsule(时间顺序约束)。
2. WHEN issuer 发布的 capsule 包含 derived_from 但其引用的 capsule 自身又(直接或间接)依赖本 capsule,该 capsule SHALL 被发布工具拒绝(防止人为构造 cycle)。
3. THE 接收方 verifier 在递归 verify `capsule_ref` 类型 grounding 时 SHALL 维护已访问 capsule 集合,检测到 cycle 立即终止并标记 `PROVENANCE_CYCLE`。
4. THE provenance DAG 深度 SHALL 不超过 64(防止长链 DoS),超过的 capsule 在发布时拒绝。
5. THE `provenance` 对象 SHALL 同时支持可选字段 `consensus_episode`(指向 anti-conformity consensus 的 episode_id)与 `challenges_survived`(过往挑战记录),这些字段在本 spec MVP 是占位,实际填充由后续 spec 提供。

### Requirement 8: Identity 与签名(纯 Ed25519,无注册中心)

**User Story**:作为协议的最小可行身份方案,我需要一个不依赖任何注册中心的 agent 身份系统,后续可演化为完整 DID。

#### Acceptance Criteria

1. THE Agent 身份 SHALL 是一个 Ed25519 密钥对;公钥按 multibase + multicodec 编码为 `did:key:z<base58btc>` 格式(W3C did:key 标准的子集)。
2. THE 系统 SHALL 提供 `generateIdentity() → { did: string; privateKey: Uint8Array; publicKey: Uint8Array }`。
3. THE 私钥的存储与保护**不**在本 spec 范围内——本 spec 假设调用方负责安全存储。
4. THE 签名 SHALL 使用 RFC 8032 EdDSA / Ed25519,签名输出 64 字节,base64url 编码后写入 capsule。
5. WHEN issuer 字段格式不是合法 `did:key:z...`,接收方 SHALL 拒绝该 capsule(`MALFORMED_ISSUER`)。
6. THE 实现 SHALL 包含跨语言互验测试:TypeScript 实现签名的 capsule,Python 脚本 verify 通过(确保不是 TypeScript-internal 自洽幻觉)。
7. THE 本 spec 不实现 reputation lookup、不实现 issuer 撤销、不实现 multi-sig——这些都是后续 spec 工作。
8. THE 系统 SHALL 暴露 `ReputationOracle` 接口(占位实现):`lookup(did) → ReputationVector | null`。MVP 实现返回 `null`(无意见),供调用方 fallback 到完全自己 verify 模式。

### Requirement 9: Announcement 与发现机制(MVP 极简)

**User Story**:作为公网双 agent demo 的最小通信,我需要一种方式让 Agent A 通知 Agent B "我有新 capsule",让 B 知道去哪里 fetch。

#### Acceptance Criteria

1. THE MVP announcement SHALL 是 HTTP POST 到 receiver 提前注册的 webhook URL,body 是 `{ capsule_id, issuer, store_hints, announced_at }`。
2. THE webhook URL SHALL 是 receiver 在协议握手阶段告知 sender 的(MVP 通过配置文件,后续可演化为 DID 文档发现)。
3. THE announcement **不携带** capsule body——receiver 收到后用 capsule_id + store_hints 主动 fetch + verify。
4. THE 接收方 webhook handler SHALL 在收到 announcement 后异步处理(返回 200 立即,后续 fetch + verify 不阻塞 sender)。
5. WHEN announcement 的 issuer 与后续 fetch 到的 capsule 的 issuer 不一致,该 capsule SHALL 被拒绝(防止 announcement 替换攻击)。
6. THE 长期 announcement 机制(entropic channel、pubsub、push notification)是后续 spec 范畴,本 spec 只要求 webhook 接口稳定。

### Requirement 10: Audit Log

**User Story**:作为协议透明性负责人,我需要 capsule 的整个生命周期(publish / fetch / verify)在 audit log 中完整可见。

#### Acceptance Criteria

1. THE 系统 SHALL 提供 `CapsuleAuditEntry` 类型,事件类型至少包括:`'published'`、`'fetched'`、`'fetch_failed'`、`'verified'`、`'verify_failed'`、`'announcement_received'`、`'announcement_rejected'`。
2. THE audit log SHALL 是 append-only,采用 NDJSON 持久化(与 `cav-anti-conformity-consensus` audit log 格式一致)。
3. THE audit entry SHALL 引用 capsule_id,但**不**重复存 capsule body(只引用,通过 capsule_id 去 store fetch)。
4. THE audit entry SHALL 包含 timestamp、actor(本地 agent did)、event、relevant_ids、可选 metadata。
5. THE 查询接口 `queryCapsuleAudit(filter) → CapsuleAuditEntry[]` SHALL 支持按 capsule_id、issuer、event_type、time_window 过滤。
6. THE audit log 在双 agent demo 中 SHALL 让外部观察者能完整重建"capsule 从 A 到 B 的过程",这是 MVP 演示的关键判据。

### Requirement 11: 双 Agent 公网 Demo(MVP 验收)

**User Story**:作为本 spec 的最终判据,我需要一个外部可观察、可重现的双 agent 公网 demo 跑通。

#### Acceptance Criteria

1. THE 项目 SHALL 包含 `examples/two-agent-public-demo/` 目录,内含两个独立 agent 进程的完整可运行代码。
2. THE demo SHALL 在两台不同机器(或两个独立云实例)上跑通,通过 HTTPS 通信。
3. THE demo 流程 SHALL 包含:
   - Agent A 跑一段 PEV 推理(用现有 `pev/pevRunner.ts`)
   - Agent A 把推理产出的 hypothesis + evidence 包装成 KnowledgeCapsule(转换器在本 spec 范围内,见 R12)
   - Agent A publish 该 capsule
   - Agent A 通过 webhook announcement 通知 Agent B
   - Agent B 自动 fetch + verify
   - Agent B 在自己的本地 ledger 中标记 capsule 状态
4. THE demo SHALL 包含至少 3 个测试场景:
   - **正常路径**:capsule 合法,verification 全部通过
   - **篡改路径**:capsule body 被中间人修改,接收方检测到 hash mismatch
   - **失败路径**:capsule 中某 grounding 故意构造为不可重现,接收方报告 `partially-verified`
5. THE demo 完整运行时间 SHALL ≤ 5 分钟(包括 PEV 推理 + 公网传输 + verify)。
6. THE demo 的输出 SHALL 包含可读 audit log,任何阅读者都能从中复盘整个流程。
7. THE demo 的网络成本 SHALL ≤ 1 MB / capsule(防止 wire 层膨胀)。

### Requirement 12: PEV → Capsule 转换器

**User Story**:作为 demo 的工程实现者,我需要一个把现有 PEV ledger 内容转成 KnowledgeCapsule 的转换器,这是双 agent demo 的关键桥接。

#### Acceptance Criteria

1. THE 系统 SHALL 提供 `pevToCapsule(hypothesis, ledger, issuerKey) → KnowledgeCapsule`。
2. THE 转换 SHALL 提取 hypothesis 的 evidenceTrail 中所有 ToolEvidence 作为 grounding 的 `tool_run` 类型。
3. THE 转换 SHALL 把 hypothesis 的 `text` + `kind` + 关联 evidence 的 verdict 总结为 `claim.causal_skeleton`(初版用简单模板)。
4. THE 转换 SHALL 把 hypothesis 的 confidence 与历史 falsification 转成 `claim.uncertainty_geometry`。
5. THE 转换 SHALL 是**纯函数**——给定相同输入输出相同 capsule(便于测试)。
6. THE 转换 SHALL **不**实现复杂语义压缩——初版可以让 capsule 比较冗长。优化是 v2 工作。
7. THE 转换 SHALL 包含反向路径占位:接收方收到 capsule 后,如何把它转回本地 PEV ledger 的接口(本 spec 不实现,只声明类型)。

### Requirement 13: 性能与限额

**User Story**:作为运行时性能关心者,我需要保证 capsule 系统在公网部署时不会失控。

#### Acceptance Criteria

1. THE capsule 序列化后 size SHALL ≤ 256 KB(包括所有字段),超过的 capsule 直接拒绝。
2. THE 单个 grounding 重 verify SHALL 默认超时 60s,可配置上调到 5 分钟。
3. THE publish 端 SHALL rate-limit 同一 issuer 的发布频率(默认 ≤ 10 capsule / 秒),超过返回 `RATE_LIMITED`。
4. THE fetch 端 SHALL rate-limit 同一 capsule_id 的并发 fetch(默认 ≤ 5 并发),超过排队。
5. THE 整套实现 SHALL 在 i7-class 笔记本 + 100Mbps 网络上跑双 agent demo 的端到端延迟 < 30s。

### Requirement 14: 文档与可读性

**User Story**:作为 6 周后接手或外部读这个 spec 的人,我需要文档清楚到能独立实现。

#### Acceptance Criteria

1. THE 项目 SHALL 包含 `src/services/cav/capsule/README.md`,概念性介绍 + 与 PEV / charter / 其他 spec 的关系。
2. THE 每个公开函数 SHALL 有 JSDoc 引用本 spec 的具体 Requirement。
3. THE schema 文件 SHALL 包含字段级 description。
4. THE 项目 SHALL 包含 `docs/cav-knowledge-capsule-tutorial.md`,带最少代码量(< 100 行)能跑通的入门示例。
5. THE README SHALL 显式声明 v1 不解决的问题(加密、撤回、reputation 实现等),链接到对应 future spec。

## Open Questions

留给 design 阶段或后续讨论:

1. **Capsule 撤回与修订**:capsule 不可变是简洁的,但现实中 issuer 可能想撤回(发现错误时)。MVP 不做,但 v2 必须考虑——是用 superseding capsule 模式(发新 capsule 引用旧的并标记 retracted)?还是引入可变 status 字段?
2. **`tool_run` grounding 的非确定性问题**:有些工具(联网工具、时间敏感工具)同一输入输出会变化。是否允许声明工具是 non-deterministic,降低 hash 比对要求?
3. **Capsule 大小压缩**:JSON 是文本,256 KB 限额可能限制某些复杂 evidence。是否考虑 CBOR 作为 binary alternative?
4. **Grounding fetch 的去中心化**:MVP 用 HTTP,但 IPFS 等去中心化存储显然更合适——什么时候迁移?
5. **Reputation oracle 的具体形式**:本 spec 只有接口。reputation 是从 anti-conformity consensus 流出来的副产物,还是独立的服务?这个决定会影响后续多个 spec
6. **跨语言实现的合规测试基础设施**:R3-6 要求 TS+Python 互验。如何把这套互验做成 CI 自动化?

## Dependencies

| 本 spec 依赖于 | 状态 |
|---|---|
| Charter v0.2 | ✅ 已存在 |
| HP-05 brief(latent ledger anchor 概念) | ✅ 已存在 |
| `pev/ledger.ts`(转换器源数据) | ✅ 已存在 |
| `pev/canonicalTests.ts`(工具白名单) | ✅ 已存在 |
| Ed25519 库(`@noble/ed25519` 或类似) | 第三方,选型在 design 阶段确定 |
| JCS 实现(RFC 8785) | 第三方,选型在 design 阶段确定 |

| 后续 spec 依赖本 spec | 何时 |
|---|---|
| `cav-anti-conformity-consensus`(endorsement 类型) | design 阶段对齐到 capsule schema |
| `cav-identity-and-sybil`(HP-03) | 完整 DID + 撤销 + reputation 实现 |
| `cav-entropic-channel` | latent 消息必须引用 capsule_id |
| `cav-storage-backends` | IPFS / DHT 后端实现 |

## Success Criteria(spec 完成判据)

本 spec 标记 `Done` 的条件:

1. 14 个 Requirement 的 SHALL 项目全部实现 + 单元测试
2. 双 agent 公网 demo(R11)在两台不同机器上跑通,3 个场景全部产出预期 audit log
3. 跨语言签名互验(R8-6)通过
4. JCS 规范化的属性测试(R3-5)1000 次全部一致
5. 性能基准(R13)在指定硬件上达成
6. README + tutorial 完成
7. 所有 schema 文件、JSDoc、引用路径校验无空链接

不达成上述任一条,**不**进入 anti-conformity consensus 的 design 阶段——因为 endorsement 类型的最终形态依赖 capsule schema。

## Appendix A: Claim 子字段 schema(详细)

> 本附录在 design 阶段会被进一步细化。这里给出 v1 的最小可行 schema。

```jsonc
{
  "claim": {
    "causal_skeleton": {
      "subject": "<string, free-form 概念名>",
      "relation": "causes | correlates_with | contradicts | refines",
      "object": "<string, free-form 概念名>",
      "mechanism_hypothesis": "<string, ≤ 500 chars 描述>",
      "strength": <number ∈ [0, 1]>
    },
    "uncertainty_geometry": {
      "confidence": <number ∈ [0, 1]>,
      "counterfactual_neighborhood": "<string, ≤ 500 chars>",
      "known_failure_modes": ["<string>", ...]
    },
    "methodology": {
      "prior_source_tag": "<enum: 'training' | 'tool_observation' | 'derived_from_capsule' | 'human_assertion'>",
      "inference_method_tag": "<enum: 'pev_loop' | 'analogy' | 'formal_proof' | 'consensus_aggregation'>",
      "data_source_hashes": ["<sha256>", ...]
    },
    "falsifiability": {
      "would_be_retracted_if": "<string, ≤ 500 chars>",
      "test_protocol_capsule_ref": "<capsule_id, optional>"
    }
  }
}
```

子字段的更严格 schema(枚举值穷举、字段长度、命名规范)在 design.md 阶段细化。
