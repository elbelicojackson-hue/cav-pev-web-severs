# Design: CAV Praxon — 公网 agent 间结构化认知单元的传递协议

## Overview

Praxon 是 CAV 协议中认知的基本粒子。本 design 把 requirements 的 12 个 SHALL 落到具体模块、接口签名、数据流、算法。

核心洞察:**agent 之间传递的不是答案,是"让对方变得不那么不确定"的结构化信号**。一个 Praxon 的价值不在于它说了什么,在于接收方收到后 entropy 下降了多少。

技术栈:TypeScript / Node.js / Ed25519(`@noble/ed25519`) / JCS(RFC 8785,`canonicalize` npm) / SHA-256(Node crypto)。

## Architecture

```
src/services/cav/praxon/
├── types.ts              Praxon / GroundingHandle / VerificationReport 类型
├── schema/
│   └── praxon.v1.schema.json    JSON Schema (R1)
├── canonicalize.ts       JCS 规范化 + hash + 签名 (R6)
├── identity.ts           Ed25519 密钥生成 / 签名 / 验签 (R6)
├── publish.ts            publishPraxon() (R4)
├── fetch.ts              fetchPraxon() (R4)
├── verify/
│   ├── gate1Schema.ts    Gate 1: schema + signature + praxon_id (R3)
│   ├── gate2Grounding.ts Gate 2: per-handle grounding verify (R2, R3)
│   ├── gate3Eig.ts       Gate 3: canary EIG 度量 (R3)
│   └── threeGate.ts      verifyPraxon() 组合三关 (R3)
├── grounding/
│   ├── toolRun.ts        tool_run grounding verify
│   ├── canaryEig.ts      canary_eig grounding verify
│   ├── demonstrationTrace.ts  demonstration_trace verify
│   ├── praxonRef.ts      praxon_ref recursive verify
│   ├── formalProof.ts    formal_proof verify (占位)
│   └── dataset.ts        dataset hash verify
├── provenance.ts         DAG 构建 + cycle 检测 (R5)
├── converter.ts          PEV → Praxon 转换器 (R9)
├── announcement.ts       HTTP webhook announcement (R7)
├── audit.ts              NDJSON audit log (R8)
├── store/
│   ├── interface.ts      PraxonStore 接口
│   └── fsStore.ts        本地 FS + HTTP GET 实现
└── __tests__/
    ├── canonicalize.test.ts
    ├── threeGate.test.ts
    ├── provenance.test.ts
    ├── converter.test.ts
    └── demo/             双 agent demo 夹具 (R10)
```


## Data Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ Agent A (Issuer)                                                 │
│                                                                   │
│  PEV Loop → hypothesis + evidence                                │
│       │                                                           │
│       ▼                                                           │
│  converter.ts: pevToPraxon(hypothesis, ledger, key)              │
│       │                                                           │
│       ▼                                                           │
│  canonicalize.ts: computePraxonId() + sign()                     │
│       │                                                           │
│       ▼                                                           │
│  publish.ts: publishPraxon(praxon, store)                        │
│       │         ┌──────────────────────────┐                     │
│       └────────►│ PraxonStore (FS + HTTP)  │                     │
│                 └──────────┬───────────────┘                     │
│  announcement.ts: POST webhook ──────────────────────────────┐   │
└──────────────────────────────────────────────────────────────┼───┘
                                                                │
┌───────────────────────────────────────────────────────────────▼───┐
│ Agent B (Receiver)                                                 │
│                                                                     │
│  webhook handler: receive announcement                             │
│       │                                                             │
│       ▼                                                             │
│  fetch.ts: fetchPraxon(praxonId, storeHints)                       │
│       │                                                             │
│       ▼                                                             │
│  verify/threeGate.ts: verifyPraxon(praxon)                         │
│       │                                                             │
│       ├─► gate1Schema.ts: schema + sig + hash ✓                    │
│       ├─► gate2Grounding.ts: per-handle verify ✓                   │
│       └─► gate3Eig.ts: canary EIG measurement ✓                   │
│       │                                                             │
│       ▼                                                             │
│  VerificationReport → local ledger (append)                        │
│  audit.ts: log event                                               │
└─────────────────────────────────────────────────────────────────────┘
```

## Core Types (types.ts)

```typescript
/** The fundamental cognitive particle of CAV. */
export type Praxon = {
  readonly version: '1.0'
  readonly praxon_id: string          // SHA-256 hex
  readonly praxon_class: PraxonClass
  readonly issuer: string             // did:key:z...
  readonly issued_at: string          // ISO 8601 UTC
  readonly claim: PraxonClaim
  readonly grounding: readonly GroundingHandle[]  // non-empty (Axiom 4)
  readonly provenance: Provenance
  readonly signature: string          // Ed25519 base64url
}

export type PraxonClass =
  | 'operational'
  | 'deliberation_motion'
  | 'deliberation_resolution'

export type PraxonClaim = {
  readonly causal_skeleton: CausalSkeleton
  readonly uncertainty_geometry: UncertaintyGeometry
  readonly methodology: Methodology
  readonly falsifiability: Falsifiability
}

export type CausalSkeleton = {
  readonly subject: string
  readonly relation: 'causes' | 'correlates_with' | 'contradicts' | 'refines' | 'enables'
  readonly object: string
  readonly mechanism_hypothesis: string  // ≤ 500 chars
  readonly strength: number              // [0, 1]
}

export type UncertaintyGeometry = {
  readonly confidence: number            // [0, 1]
  readonly counterfactual_neighborhood: string  // ≤ 500 chars
  readonly known_failure_modes: readonly string[]
}

export type Methodology = {
  readonly prior_source_tag: PriorSourceTag
  readonly inference_method_tag: InferenceMethodTag
  readonly data_source_hashes: readonly string[]
}

export type PriorSourceTag =
  | 'training' | 'tool_observation' | 'derived_from_praxon'
  | 'human_assertion' | 'demonstration'

export type InferenceMethodTag =
  | 'pev_loop' | 'analogy' | 'formal_proof'
  | 'consensus_aggregation' | 'pattern_recognition'

export type Falsifiability = {
  readonly would_be_retracted_if: string  // ≤ 500 chars
  readonly test_protocol_praxon_ref?: string  // praxon_id
}

export type GroundingHandle =
  | ToolRunGrounding
  | CanaryEigGrounding
  | DemonstrationTraceGrounding
  | PraxonRefGrounding
  | FormalProofGrounding
  | DatasetGrounding

export type ToolRunGrounding = {
  readonly type: 'tool_run'
  readonly tool_manifest_ref: string
  readonly args_hash: string
  readonly stdout_hash: string
  readonly exit_code: number
}

export type CanaryEigGrounding = {
  readonly type: 'canary_eig'
  readonly task_set_id: string
  readonly measured_eig_bits: number
  readonly methodology: string
  readonly measured_at: string
}

export type DemonstrationTraceGrounding = {
  readonly type: 'demonstration_trace'
  readonly trace_hash: string
  readonly task_description: string       // ≤ 500 chars
  readonly reasoning_steps_summary: string // ≤ 2000 chars
  readonly outcome: 'success' | 'partial' | 'failure'
  readonly trace_uri?: string
}

export type PraxonRefGrounding = {
  readonly type: 'praxon_ref'
  readonly praxon_id: string
  readonly store_hint?: string
}

export type FormalProofGrounding = {
  readonly type: 'formal_proof'
  readonly system: string
  readonly proof_hash: string
  readonly source_uri: string
}

export type DatasetGrounding = {
  readonly type: 'dataset'
  readonly uri: string
  readonly hash: string
}

export type Provenance = {
  readonly derived_from: readonly string[]
  readonly consensus_episode?: string
  readonly challenges_survived?: readonly string[]
}

export type VerificationReport = {
  readonly praxon_id: string
  readonly gate1_schema: GateResult
  readonly gate2_grounding: Gate2Result
  readonly gate3_eig: Gate3Result
  readonly overall: OverallVerdict
  readonly verified_at: string
}

export type GateResult = { readonly ok: boolean; readonly reason?: string }
export type Gate2Result = {
  readonly attempted: number
  readonly passed: number
  readonly failed: readonly { index: number; reason: string }[]
}
export type Gate3Result = {
  readonly measured_eig_bits: number | null
  readonly issuer_claimed_eig: number | null
  readonly eig_mismatch: boolean
  readonly skipped: boolean
}
export type OverallVerdict = 'verified' | 'partially-verified' | 'unverified' | 'rejected'
```

## Algorithm: Canonicalization & Signing

```
ALGORITHM computePraxonId(praxonBody)
INPUT: Praxon object WITHOUT praxon_id and signature
OUTPUT: SHA-256 hex string

  canonical ← JCS(praxonBody)          // RFC 8785
  RETURN SHA256(canonical).hex()

ALGORITHM signPraxon(praxonWithId, privateKey)
INPUT: Praxon WITH praxon_id, WITHOUT signature
OUTPUT: base64url Ed25519 signature

  canonical ← JCS(praxonWithId)
  sig ← Ed25519.sign(privateKey, canonical)
  RETURN base64url(sig)

ALGORITHM buildAndSign(praxonBody, privateKey)
INPUT: raw fields, Ed25519 private key
OUTPUT: complete signed Praxon

  praxon_id ← computePraxonId(praxonBody)
  bodyWithId ← { ...praxonBody, praxon_id }
  signature ← signPraxon(bodyWithId, privateKey)
  RETURN { ...bodyWithId, signature }
```

## Algorithm: Three-Gate Verify

```
ALGORITHM verifyPraxon(praxon, options)
OUTPUT: VerificationReport

  gate1 ← gate1Schema(praxon)
  IF NOT gate1.ok → RETURN 'rejected'

  gate2 ← gate2Grounding(praxon.grounding)

  IF options.skipEig THEN
    gate3 ← { skipped: true }
  ELSE
    gate3 ← gate3Eig(praxon, options.canaryTaskSet)

  overall ← computeOverall(gate1, gate2, gate3)
  RETURN report

ALGORITHM computeOverall(gate1, gate2, gate3)
  IF NOT gate1.ok → 'rejected'
  IF gate2.passed = 0 → 'unverified'
  IF NOT gate3.skipped AND gate3.measured_eig ≤ 0 → 'unverified'
  IF gate2.failed.length > 0 → 'partially-verified'
  → 'verified'
```

## Algorithm: Gate 3 EIG Measurement

```
ALGORITHM measureEig(praxon, canaryTaskSet)
OUTPUT: Gate3Result

  FOR EACH task IN canaryTaskSet:
    beforeConf ← agent.predict(task, without praxon)
    afterConf ← agent.predict(task, with praxon)

  H_before ← mean(beforeConf.map(binaryEntropy))
  H_after ← mean(afterConf.map(binaryEntropy))
  eig ← H_before - H_after

  issuerClaimed ← findCanaryEigGrounding(praxon)?.measured_eig_bits
  mismatch ← issuerClaimed != null AND |eig - issuerClaimed| > 0.3

  RETURN { measured_eig_bits: eig, issuer_claimed_eig: issuerClaimed, eig_mismatch: mismatch }

FUNCTION binaryEntropy(p)
  p ← clamp(p, 0.001, 0.999)
  RETURN -p * log2(p) - (1-p) * log2(1-p)
```

## Algorithm: PEV → Praxon Converter

```
ALGORITHM pevToPraxon(hypothesis, ledger, issuerKey)
OUTPUT: signed Praxon

  groundings ← []
  FOR EACH evId IN hypothesis.evidenceTrail:
    ev ← ledger.evidenceLog.find(e.id = evId)
    groundings.push({ type: 'tool_run', tool_manifest_ref: ev.planId,
                      args_hash: SHA256(ev.toolArgs), stdout_hash: SHA256(ev.resultDigest),
                      exit_code: ev.outcome='success' ? 0 : 1 })

  IF groundings.length = 0:
    groundings.push({ type: 'demonstration_trace',
                      trace_hash: SHA256(hypothesis.text),
                      task_description: 'PEV hypothesis',
                      reasoning_steps_summary: hypothesis.text.slice(0,2000),
                      outcome: hypothesis.status='evidence' ? 'success' : 'partial' })

  claim ← buildClaim(hypothesis)
  body ← { version:'1.0', praxon_class:'operational', issuer: issuerKey.did,
            issued_at: now(), claim, grounding: groundings, provenance: { derived_from: [] } }
  RETURN buildAndSign(body, issuerKey.privateKey)
```

## Module Interface Signatures

```typescript
// canonicalize.ts
export function computePraxonId(body: Omit<Praxon, 'praxon_id' | 'signature'>): string
export function signPraxon(p: Omit<Praxon, 'signature'>, key: Uint8Array): string
export function buildAndSign(body: Omit<Praxon, 'praxon_id' | 'signature'>, key: Uint8Array): Praxon
export function verifySignature(praxon: Praxon): boolean

// identity.ts
export type AgentIdentity = { did: string; publicKey: Uint8Array; privateKey: Uint8Array }
export function generateIdentity(): AgentIdentity
export function didFromPublicKey(pk: Uint8Array): string
export function publicKeyFromDid(did: string): Uint8Array

// publish.ts
export function publishPraxon(praxon: Praxon, store: PraxonStore): Promise<PublishResult>

// fetch.ts
export function fetchPraxon(id: string, hints?: string[]): Promise<FetchResult>

// verify/threeGate.ts
export function verifyPraxon(praxon: Praxon, opts?: VerifyOptions): Promise<VerificationReport>

// store/interface.ts
export interface PraxonStore {
  put(id: string, content: string): Promise<void>
  get(id: string): Promise<string | null>
  has(id: string): Promise<boolean>
}

// converter.ts
export function pevToPraxon(h: Hypothesis, ledger: SharedLedger, issuer: AgentIdentity): Praxon

// announcement.ts
export function announce(a: Announcement, webhookUrl: string): Promise<void>
export function createAnnouncementHandler(cb: (a: Announcement) => Promise<void>): RequestHandler

// audit.ts
export function appendAudit(entry: PraxonAuditEntry, logPath: string): void
export function queryAudit(logPath: string, filter: Partial<PraxonAuditEntry>): PraxonAuditEntry[]
```

## Third-Party Dependencies

| Package | Purpose | Version |
|---|---|---|
| `@noble/ed25519` | Ed25519 sign/verify | `^2.1.0` |
| `canonicalize` | RFC 8785 JCS | `^2.0.0` |
| `ajv` | JSON Schema validation | `^8.12.0` |
| Node `crypto` | SHA-256 | built-in |

Test: `vitest` + `@faker-js/faker`.

## Canary Task Set (MVP v1)

10 TypeScript code review tasks with known ground truth:

| ID | Bug Type | Ground Truth |
|---|---|---|
| ts-unawait-01 | Unhandled Promise | has_bug |
| ts-unawait-02 | Properly awaited | no_bug |
| ts-nullcheck-01 | Missing null check | has_bug |
| ts-nullcheck-02 | Proper narrowing | no_bug |
| ts-errorhandling-01 | Swallowed error | has_bug |
| ts-errorhandling-02 | Proper try/catch | no_bug |
| ts-race-01 | Race condition in state | has_bug |
| ts-race-02 | Proper mutex pattern | no_bug |
| ts-useeffect-01 | Missing cleanup | has_bug |
| ts-closure-01 | Stale closure | has_bug |

Canary set is versioned, publishable as Praxon, challengeable per Axiom 1.

## Demo Architecture (R10)

```
Machine A                              Machine B
┌──────────────────────┐              ┌──────────────────────┐
│ agent-a/             │              │ agent-b/             │
│   identity.json      │              │   identity.json      │
│   config.json        │              │   config.json        │
│   praxon-store/      │   HTTPS      │   canary-tasks/      │
│   http-server ───────┼──────────────┼──► webhook-server    │
└──────────────────────┘              └──────────────────────┘

Flow: A discovers heuristic → converts to Praxon → publishes →
      announces to B → B fetches → B runs Three-Gate → B logs result
```

3 test paths: Normal (verified) / Tampered (rejected) / EIG fail (unverified).

## Implementation Order

1. `types.ts` + `schema/praxon.v1.schema.json`
2. `identity.ts`
3. `canonicalize.ts`
4. `store/interface.ts` + `store/fsStore.ts`
5. `publish.ts` + `fetch.ts`
6. `verify/gate1Schema.ts`
7. `grounding/*.ts`
8. `verify/gate2Grounding.ts`
9. `verify/gate3Eig.ts` + canary task set
10. `verify/threeGate.ts`
11. `converter.ts`
12. `announcement.ts`
13. `audit.ts`
14. `__tests__/demo/`

Estimated: 4-5 weeks solo, 2-3 weeks pair.

## Open Questions Resolved

| Question | Decision |
|---|---|
| Canary content | 10 TS code review tasks |
| EIG algorithm | Binary entropy before/after on canary |
| Founding tool manifests | PEV 6 + tsc + eslint + vitest + git |
| Demonstration trace format | ≤ 2000 chars: input + steps + output + verdict |
| Composition | Via provenance.derived_from; grounding = "combined EIG > individual" |
