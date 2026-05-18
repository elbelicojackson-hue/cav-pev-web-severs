# -CAV-CCB

基于CAV唯一公民协议的CCB — 多智能体逆向工程协同框架

<!-- 项目 Banner（替换为你的实际截图） -->
<!-- ![CAV-CCB Banner](docs/images/banner.png) -->

> 🛡️ **Hacking Code 工具套件** | 💰 **商业授权: ¥499** | 📧 联系: [@elbelicojackson-hue](https://github.com/elbelicojackson-hue)

---

## 💰 定价 & 商业授权

| 方案 | 价格 | 包含内容 |
|------|------|----------|
| **个人学习** | 免费查看 | 仅限阅读源码，不可使用/部署/修改 |
| **Hacking Code 工具授权** | **¥499** (一次性) | 完整源码使用权 + 部署权 + 1 年更新 |
| **团队/企业授权** | 联系报价 | 多人使用 + 定制开发 + 技术支持 |

### Hacking Code 工具授权 (¥499) 包含：

- ✅ 完整 CAV/CCB/PEV 源码使用权
- ✅ 逆向工程全套工具链（DiE、UPX、IDA headless、Ghidra、YARA）
- ✅ 多智能体协同分析能力（4+ LLM 并行）
- ✅ 24 个 Canonical Tool Plans（开箱即用）
- ✅ 私有部署权（本地/内网）
- ✅ 1 年内版本更新
- ❌ 不含转授权 / 二次分发权
- ❌ 不含算法专利授权（仅使用权）

**购买方式**: 通过 GitHub 联系 [@elbelicojackson-hue](https://github.com/elbelicojackson-hue)

---

## 概述

CAV/CCB 是一套**多智能体协同逆向工程框架**，将传统单模型"一问一答"的 RE 工作流升级为结构化的假设驱动执行循环。系统通过进程持有的状态机（而非模型记忆）管理假设生命周期，彻底解决了 LLM 在第 4-5 轮后注意力塌陷、漏验证、混淆已证伪假设等核心痛点。

## 核心使用场景

### 🔬 逆向工程 (Reverse Engineering)
- PE/ELF/Mach-O 二进制格式识别
- 加壳检测与脱壳验证（UPX、VMProtect、Themida）
- 编译器/链接器指纹识别（MSVC、GCC、Go、.NET）
- 恶意软件家族归属（YARA 规则 + imphash 比对）
- 反调试/反分析技术检测（TLS callback、PEB.BeingDebugged）

### 🔓 二进制反编译 (Decompilation)
- IDA Pro / Ghidra headless 自动化分析
- 加密算法识别（AES/RSA/RC4/ChaCha20）
- 函数签名恢复与交叉引用分析
- 控制流混淆检测

### 🏗️ 软件重构 (Software Reconstruction)
- 协议逆向（HTTP/gRPC/MQTT/自定义二进制协议）
- 网络能力枚举（imports table + syscall trace）
- C2 通信通道识别
- 动态行为分析（strace/tshark 集成）

### 🛡️ 安全审计 (Security Audit)
- 漏洞面分析（攻击面枚举）
- 供应链组件识别
- 加密实现审计
- 权限提升路径发现

## 与单模型方案的对比

| 维度 | 单模型 (GPT-4/Claude 单轮) | CAV/CCB 多智能体 |
|------|---------------------------|-----------------|
| **状态管理** | 依赖模型记忆（4-5 轮后塌陷） | 进程持有的 SharedLedger（永不遗忘） |
| **假设追踪** | 自由文本，混淆 open/falsified | Typed Hypothesis Bank（8 类 × 5 状态） |
| **工具调用** | 模型自由编造命令 | 白名单 Canonical Plans（24+ 确定性模板） |
| **结果判定** | 模型主观读屏 | VerdictEngine 正则自动判定（确定性） |
| **跨轮一致性** | 第 5 轮开始遗忘前轮结论 | Ledger reducer 保证全程一致 |
| **失败修正** | 隐式（模型自我反思，常失败） | 显式 falsify + stale cascade 传播 |
| **多视角** | 单一模型偏见 | 4+ 异构 LLM 交叉验证 |
| **可审计性** | 对话记录（非结构化） | `.pev.json` 完整轨迹（可重放） |
| **停机保证** | 无（模型决定何时停） | 5 种确定性停机条件 |
| **注意力效率** | O(n²) 随轮次衰减 | O(1) 每轮只看自己的 active H + inbox |

### 实测效果对比（UPX 加壳 PE 分析）

| 指标 | Claude 单模型 | CAV/CCB PEV |
|------|--------------|-------------|
| 正确识别加壳 | ✓（第 1 轮） | ✓（第 1 轮） |
| 验证加壳类型 | ✗（第 3 轮遗忘验证） | ✓（E1 confirms, 自动） |
| 识别编译器 | ✓（第 2 轮） | ✓（E2 confirms, 自动） |
| 排除反调试 | ✗（第 5 轮仍在讨论） | ✓（E3 falsifies → stale cascade） |
| 总工具调用 | 8（含 3 次重复） | 4（零重复，scheduler 去重） |
| 总轮次 | 7（人工终止） | 4（all-resolved 自动停机） |
| 可重放 | ✗ | ✓（.pev.json 完整轨迹） |

## 系统架构

```
┌─────────────────────────────────────────────────────────────────┐
│  入口层                                                          │
│    /ccb-pev <binary> [goal] [--max-rounds=N]                    │
│    /ccb-arena <claim>                                           │
│    /ccbteam <task>                                              │
├─────────────────────────────────────────────────────────────────┤
│  PEV 核心层 (纯函数，零副作用)                                    │
│    protocol.ts    → zod 严格 schema + TypeScript 类型            │
│    validator.ts   → 跨字段引用完整性校验                          │
│    parser.ts      → 三层容错 (strict → repair → retry)           │
│    ledger.ts      → 不可变 reducer (假设 + 证据状态机)            │
│    canonicalTests.ts → 24+ 工具计划 const 表                     │
│    verdict.ts     → 正则判定引擎 (≤50ms/1MB)                     │
│    scheduler.ts   → 每轮调度 (confidence 优先 + 去重)             │
│    propagator.ts  → 跨 agent 推送 + 子假设派生                    │
│    promptBuilder.ts → system + user prompt 组装                  │
│    pevRunner.ts   → 主循环 async generator                       │
│    persistence.ts → .pev.json 原子写入                           │
├─────────────────────────────────────────────────────────────────┤
│  工具层 (零修改复用)                                              │
│    ReverseCli  → DiE / UPX / IDA / Ghidra / PE-header / strings │
│    Bash        → file / readelf / strace / tshark / yara        │
│    Grep        → 正则搜索                                        │
│    Read        → 文件读取                                        │
│    WebSearch   → 在线情报查询                                     │
│    Firecrawl   → 网页内容抓取                                     │
├─────────────────────────────────────────────────────────────────┤
│  UI 层                                                           │
│    PevSession.tsx         → 主状态组件                            │
│    HypothesisTreeView.tsx → 假设树 (layered indent + 颜色编码)    │
│    EvidenceLogView.tsx    → 证据日志 (last 5, verdict 着色)       │
│    AgentStatusBar.tsx     → agent 状态条                          │
└─────────────────────────────────────────────────────────────────┘
```

## 核心算法

| 算法 | 描述 | 位置 |
|------|------|------|
| **CAV 共识协议** | 多智能体校准对抗验证，∇H ≤ 0 不动点收敛 | `src/services/cav/` |
| **PEV 执行循环** | 假设驱动的 Plan-Execute-Verify 循环 | `src/services/cav/pev/pevRunner.ts` |
| **SharedLedger Reducer** | 不可变纯函数状态机，5 种 op 的完备枚举 | `src/services/cav/pev/ledger.ts` |
| **VerdictEngine** | 纯正则确定性判定（confirms/falsifies/inconclusive） | `src/services/cav/pev/verdict.ts` |
| **Stale Cascade** | 单向假设失效传播（parent falsify → 子树 stale） | `src/services/cav/pev/ledger.ts` |
| **Cross-agent Propagator** | 横向证据推送 + 纵向子假设派生（DERIVE_RULES） | `src/services/cav/pev/propagator.ts` |
| **Three-layer Parser** | strict JSON → lenient repair → single retry | `src/services/cav/pev/parser.ts` |
| **Confidence Scheduler** | 最高置信度优先 + 工具去重 + stall guard | `src/services/cav/pev/scheduler.ts` |

## 源码结构

```
src/
├── services/cav/                    # CAV 核心服务
│   ├── pev/                         # PEV 子系统 (本项目核心)
│   │   ├── protocol.ts              # zod schema (PevOutputSchema)
│   │   ├── validator.ts             # 16 种 errorKind 的跨字段校验
│   │   ├── parser.ts                # 三层容错 + 20 个 corpus 样本
│   │   ├── ledger.ts                # 不可变 reducer (6 个纯函数)
│   │   ├── canonicalTests.ts        # 24 个工具计划 (8 kind × 3+)
│   │   ├── verdict.ts               # 正则判定 (≤50ms 性能保证)
│   │   ├── scheduler.ts             # 调度算法 (Algorithm 1)
│   │   ├── propagator.ts            # 推送算法 (Algorithm 4)
│   │   ├── promptBuilder.ts         # prompt 组装 (≤4000 token)
│   │   ├── pevRunner.ts             # 主循环 (Algorithm 5)
│   │   ├── persistence.ts           # 原子持久化
│   │   ├── README.md                # 技术文档
│   │   └── __tests__/               # 449 个测试 (unit + PBT)
│   ├── arena/                       # CCB-Arena 多 LLM 调度
│   │   ├── dispatcher.ts            # 并行分发
│   │   ├── providers.ts             # provider 加载
│   │   └── convergence.ts           # ∇H ≤ 0 收敛检测
│   ├── extractor.ts                 # CAV 信号提取
│   ├── recorder.ts                  # .cav.jsonl 记录
│   ├── analyzer.ts                  # α_weighted / MI / Rényi 谱
│   └── types.ts                     # CAV 类型定义
├── commands/
│   ├── ccb-pev/                     # /ccb-pev 命令
│   ├── ccb-arena/                   # /ccb-arena 命令
│   ├── ccb/                         # /ccb 命令
│   └── ccbteam/                     # /ccbteam 命令
└── packages/builtin-tools/
    └── ReverseCliTool/              # 22-action RE 工具引擎
```

## 测试覆盖

```bash
bun test src/services/cav/pev/__tests__/    # 429 pass, 11 个 PBT property
bun test src/commands/ccb-pev/__tests__/    # 20 pass (parseArgs + integration)
bun test src/services/cav/                  # 全 CAV 回归
```

Property-Based Tests (fast-check, 200 runs each):
- Property 1: schema_version 字面量不变性
- Property 2: HypothesisUpdate discriminator 排他性
- Property 3: HypothesisId 正则不变性
- Property 4: Canonical Tests 表完备性
- Property 6: lastEvidenceId 单调递增
- Property 7: stall guard 触发条件
- Property 8: stale H 不被调度
- Property 9: VerdictEngine 引用透明性
- Property 10: 自反不变性（无自我反馈）
- Property 11: reducer 不可变性

## 截图展示

<!-- 把你的截图放到 docs/images/ 目录，然后取消下面的注释 -->

<!-- ### PEV 运行界面 -->
<!-- ![PEV Session UI](docs/images/pev-session-ui.png) -->

<!-- ### 假设树实时渲染 -->
<!-- ![Hypothesis Tree](docs/images/hypothesis-tree.png) -->

<!-- ### 证据日志 + Verdict 着色 -->
<!-- ![Evidence Log](docs/images/evidence-log.png) -->

<!-- ### 与单模型对比效果 -->
<!-- ![Comparison](docs/images/comparison.png) -->

<!-- ### 最终 .pev.json 审计轨迹 -->
<!-- ![PEV JSON](docs/images/pev-json-output.png) -->

> 💡 **如何添加截图**: 将 PNG/JPG 文件放入 `docs/images/` 目录，然后取消上面对应行的 `<!-- -->` 注释即可显示。

---

## 许可证

**⚠️ 专有许可 — 所有权利保留**

本仓库中的所有底层算法、协议和技术实现均为版权所有者的独占知识产权。

- ❌ 未经书面授权，不得使用、复制、修改或分发
- ❌ 禁止商业用途
- ❌ 禁止逆向工程本项目的算法用于重新实现
- ✅ 仅允许查看（展示/作品集/学术审查）

详见 [LICENSE](./LICENSE)。

## 联系

如需使用许可，请通过 GitHub 联系：[@elbelicojackson-hue](https://github.com/elbelicojackson-hue)
