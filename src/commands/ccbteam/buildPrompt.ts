/**
 * Compose the final agent prompt for `/ccbteam` from a resolved profile +
 * the user's claim. Pure function, side-effect free, deterministic — easy
 * to unit-test.
 *
 * Structure stays close to the original prompt so dispatcher + observer
 * keep working unmodified:
 *   1. Heading + claim
 *   2. Compartment table (4 rows from profile)
 *   3. Per-agent CAV protocol block (unchanged)
 *   4. Recommended tools / oracles (profile-specific)
 *   5. Adversarial fixed-point loop (unchanged)
 *   6. Termination criteria (profile-specific)
 *   7. Deliverable (profile-specific)
 *   8. Domain notes (profile-specific)
 *
 * The CAV block is shared because the recorder / observer / analyzer
 * pipeline expects exactly this shape.
 */

import type { CcbTeamProfile } from './profiles/index.js'
import type { ProfileResolution } from './profiles/index.js'

/**
 * Build the full prompt. `resolution.claim` is the user's actual claim
 * (after the profile flag was stripped); never empty by the time we get
 * here — empty-claim guard is in the command shim.
 *
 * `opts.epistemicHonestyBlock` (R13) — optional Markdown segment
 * appended INSIDE the per-Agent CAV protocol block, right after the
 * existing `<cav>` JSON template. When undefined / absent / empty the
 * function output is byte-for-byte identical to the pre-Super-Agent-
 * Cluster version (R10-2 hard contract).
 */
export function buildCcbTeamPrompt(
  resolution: ProfileResolution,
  opts?: { epistemicHonestyBlock?: string },
): string {
  const { profile, claim } = resolution
  const banner = renderBanner(resolution)
  const compartments = renderCompartments(profile)
  const cavBlockBody = renderCavBlock(claim, profile)
  const epistemicTail =
    opts?.epistemicHonestyBlock && opts.epistemicHonestyBlock.length > 0
      ? `\n\n${opts.epistemicHonestyBlock}`
      : ''
  const cavBlock = `${cavBlockBody}${epistemicTail}`
  const tools = renderTools(profile)
  const oracles = renderOracles(profile)
  const termination = renderTermination(profile)
  const deliverable = renderDeliverable(profile)
  const notes = renderNotes(profile)

  return `# 你正在以 V2 协议团队组长身份启动一个 CAV-aware 共识团队

参考: https://github.com/elbelicojackson-hue/Cognitive-Affect-Vector- (v0.3)

${banner}

## 待共识的 claim / 问题

${claim}

---

## 第 1 步:四链区室分配 (profile = \`${profile.id}\`)

**Profile 定位**: ${profile.tagline}

按本 profile 的轴向派出 **4 个 in-process teammate**,每个绑定一个区室:

${compartments}

**用 \`Agent\` 工具一次性并行 spawn 这 4 个 teammate**(同一 turn 多个 tool_call)。

每个 \`Agent\` 调用的 prompt 必须包含以下 **CAV 协议指令块**(原样复制,把 \`<role>\` 换成对应 \`compartment.role\`,\`<claim>\` 换成上面的 goal,\`<axis>\` 换成对应轴):

\`\`\`
${cavBlock}
\`\`\`

## 第 2 步:本 profile 推荐工具链

${tools}

## 第 3 步:本 profile 的外部真值锚点 (∇H_oracle)

${oracles}

## 第 4 步:对抗性不动点循环

每轮 (round t):

  1. **观测**: 等所有 4 个 teammate 完成本轮发言,从他们的 \`<cav>\` 块收集 𝓒_t 矩阵
  2. **熵估计**: 内心估计 α_classical_t、强信号 MI(𝓒_i, 𝓒_j) 矩阵、是否所有 update_kl ≈ 0
  3. **触发动作** (按 §5½.3 的 5 个梯度轴选一个):
       - **∇H_attack**: 让某 teammate 攻击其他 teammate claim 中熵最高的步骤
       - **∇H_swap**: 检测共谋 → 杀掉一个,spawn 不同 model 的 teammate 替换
       - **∇H_oracle**: 调用上面"外部真值锚点"列表里的工具
       - **∇H_chain**: 切换主导链(\`${profile.compartments.map(c => c.role).join('\` / \`')}\` 轮换)
       - **∇H_discretize**: 调整内心 belief 聚类粒度
  4. **判定**: 用本 profile 的终止条件检查

### 自适应早停规则 (improvement #4)

除了上面 profile 给的终止条件之外,**有两个跨 profile 的硬退出**可以**立刻**结束循环、节省 token:

  1. **gradient-quiet**: belief entropy / Σupdate_kl / maxMI 三个梯度信号同时进入 ε-band
  2. **alpha-slope-flat**: α_weighted 在最近 ≥ 2 轮的振幅 ≤ 0.02,且已经跑过 ≥ 4 轮

第 2 条专治"agents 仍在表面争论但实际谁也说服不了谁"的死锁场景。一旦命中,**不要再起新一轮**,直接进入第 6 步出回执并把 \`status\` 标为 DEADLOCK(或对应 profile 给的等价标签)。

### 历轮 context 规则 (improvement #1)

dispatcher 已经在 prompt 层做了**增量 history 压缩**:最新一轮给 teammate 看完整 claim,更早的 1-2 轮自动压成 1 行带 repair_style 标签的摘要。所以你**不需要**自己手动复述全部历史给 teammate,他们看到的 context 已经包含必要的方向信息。这一条只是让你心里有数,不必担心 teammate 会忘记上轮发生了什么。

## 第 5 步:终止条件 (profile-specific)

${termination}

**交叉验证硬要求**: ${profile.crossValidation}

## 第 6 步:最终回执 (deliverable)

${deliverable}

## 第 7 步:domain notes

${notes}

## 通用注意

- **不要省略 \`<cav>\` 块** — 它是整个 V3 实验的数据来源
- **不要 spawn 同 model 的 4 个 teammate** — 至少跨 2 个不同 backend
- **不要让 teammate 看到对方的 CAV** — §6 双通道协议硬要求
- **如果用户的 claim 含糊不清**,先调 \`AskUserQuestion\` 收窄,再启动 4 链
`
}

/** Banner shown right under the heading explaining how this profile was chosen. */
function renderBanner(resolution: ProfileResolution): string {
  const { profile, source, matchedKeyword } = resolution
  if (source === 'flag') {
    return `> **Profile**: \`${profile.id}\` — ${profile.displayName} _(显式 \`--profile\` 指定)_`
  }
  if (source === 'auto-detect' && matchedKeyword) {
    return `> **Profile**: \`${profile.id}\` — ${profile.displayName} _(自动检测,匹配关键词 "${matchedKeyword}";加 \`--profile=generic\` 可强制回退)_`
  }
  return `> **Profile**: \`${profile.id}\` — ${profile.displayName}`
}

/** Render the 4-row compartment table. */
function renderCompartments(profile: CcbTeamProfile): string {
  const header = '| 角色名 | 区室 | 轴 | 输入限制 | 推理风格 |'
  const sep = '|---|---|---|---|---|'
  const rows = profile.compartments
    .map(c => `| \`${c.role}\` | ${c.label} | ${c.axis} | ${c.inputLimit} | ${c.style} |`)
    .join('\n')
  return `${header}\n${sep}\n${rows}`
}

/** Per-agent CAV protocol block — shared verbatim across profiles. */
function renderCavBlock(claim: string, _profile: CcbTeamProfile): string {
  // Note: claim is interpolated for the agent so it sees the goal directly,
  // matching the original prompt's behaviour.
  return `你是 V2 多智能体共识协议的 <role> 链 agent (区室轴: <axis>)。

待共识 claim:
> ${claim}

# 输出协议

每次发言必须以 *两段* 结构输出:

## 1. 内容部分 (content)

按 <role> 区室的输入限制和推理风格作答。给出你对 claim 的判断和理由。
当你听到其他 agent 反驳时,你必须明确表态:
  - 我维持 (defend): 给出新论证
  - 我接受 (concede): 说明放弃了什么
  - 我替换 (substitute): 给一个新 claim
  - 我拆分 (split): 哪部分接受、哪部分仍维持

## 2. CAV 自报 (元状态)

最后一行用 fenced block 输出你的 CAV 强信号子向量:

\\\`\\\`\\\`cav
{
  "self_entropy": <0..1>,
  "calibration": <0..1>,
  "update_kl": <0..2>,
  "repair_style": "<defend|concede|substitute|split|none>",
  "commitment": <0..1>,
  "trace_depth": <integer>
}
\\\`\\\`\\\`

# 诚实信号约束

- **不要伪造 self_entropy**: hedge 词 ("可能"/"也许"/"or") 出现就标 ≥ 0.5
- **不要伪造 calibration**: 不知道填 0.5,不要为了显得权威填 0.95
- **repair_style 必须诚实**: 下一段 content 必须和声称的 style 一致
  (concede 后不能下一句又转头辩护)

伪造 CAV 的代价 ≥ 收益 (Zahavian 信号理论, §6.7) — 评审会用 self_entropy
× repair_style 一致性算"虚伪指数",分数低的 agent 在 α_weighted 共识里票
权被稀释。`
}

function renderTools(profile: CcbTeamProfile): string {
  return profile.recommendedTools.map(t => `- ${t}`).join('\n')
}

function renderOracles(profile: CcbTeamProfile): string {
  return profile.oracles.map(o => `- ${o}`).join('\n')
}

function renderTermination(profile: CcbTeamProfile): string {
  return profile.terminationCriteria.map(t => `- ${t}`).join('\n')
}

function renderDeliverable(profile: CcbTeamProfile): string {
  return profile.deliverable.map(d => `- ${d}`).join('\n')
}

function renderNotes(profile: CcbTeamProfile): string {
  return profile.domainNotes.map(n => `- ${n}`).join('\n')
}
