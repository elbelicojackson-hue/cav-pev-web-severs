import type { ContentBlockParam } from '@anthropic-ai/sdk/resources/messages.js'

import type { Command } from '../../types/command.js'
import {
  applyInvocationGate,
  buildEpistemicHonestyBlock,
} from '../../services/cav/ccbteam-discipline/index.js'
import {
  startSidecar,
  type SidecarOptions,
} from '../../services/cav/ccbteam-math/sidecar.js'
import {
  DEFAULT_CR_EIG_WEIGHTS,
} from '../../services/cav/ccbteam-math/constants.js'
import type { CrEigWeights } from '../../services/cav/ccbteam-math/types.js'
import { getSessionId } from '../../bootstrap/state.js'
import { buildCcbTeamPrompt } from './buildPrompt.js'
import { listProfileIds, resolveProfile, type ProfileResolution } from './profiles/index.js'

/**
 * `/ccbteam <claim>` — entry point with optional engineering profile.
 *
 * Syntax (all valid):
 *   /ccbteam <claim>                          → auto-detect profile or generic
 *   /ccbteam --profile=reverse <claim>        → forced reverse profile
 *   /ccbteam --profile reverse <claim>        → same, space-separated
 *   /ccbteam --profile=generic <claim>        → opt out of auto-detect
 *
 * Super Agent Cluster flags (all optional, R10-3 minimal additions):
 *   --strategy=<observe|prompt-only>          → mount sidecar (default observe)
 *   --weights="key=val,key=val"               → CR-EIG weight overrides
 *   --explain                                 → terminal-only verbose output
 *   --epsilon=<float>                         → ε_t early-stop hint (audit only)
 *
 * The profile reshapes the 4-chain compartments, recommended tools,
 * oracles, termination criteria, and deliverable. The dispatcher /
 * observer / CAV recorder / analyzer pipeline are untouched — profiles
 * are pure prompt data.
 */

const HELP_REPLY = `用户调用了 \`/ccbteam\` 但没有给出 claim/问题。回复一句话:
"请提供需要群体共识的 claim 或问题。例:
  /ccbteam 量子计算机能在 2028 年破解 RSA-2048
  /ccbteam --profile=reverse 分析这个 dll 的反调试逻辑
  /ccbteam --profile=decompile rxjh.exe 的核心算法是什么
  /ccbteam --profile=security XSS in /api/search?q=
  /ccbteam --profile=data 这份 csv 的转化率有显著差异吗

可选 profile: ${listProfileIds().join(', ')}
不指定时,会从 claim 关键词自动选,选不出退到 generic。"
不要做任何工具调用。`

/**
 * Parsed sidecar / strategy args extracted from rawArgs. The remainder
 * (`leftover`) is the original profile flag + claim text passed
 * verbatim to the existing `resolveProfile` so R10-2 byte-parity is
 * preserved.
 */
type ParsedStrategyArgs = {
  readonly strategy: 'prompt-only' | 'observe'
  readonly weights: CrEigWeights
  readonly explain: boolean
  readonly epsilonTarget?: number
  readonly leftover: string
  readonly errors: readonly string[]
}

/** R6-1 weight key set — used by `--weights` parsing in T13. */
const ALLOWED_WEIGHT_KEYS = new Set<keyof CrEigWeights>([
  'lambdaCost',
  'gammaCausal',
  'kappaUrgency',
  'gammaExplore',
  'deltaZero',
  'useAdaptiveDelta',
])

const STRATEGY_PATTERN = /^--strategy(?:=([^\s]+))?$/i
const WEIGHTS_PATTERN = /^--weights=("([^"]*)"|([^\s]+))$/i
const EPSILON_PATTERN = /^--epsilon(?:=([^\s]+))?$/i
const EXPLAIN_PATTERN = /^--explain$/i

/**
 * Parse the sidecar/strategy flags off the front of `rawArgs`. The
 * leftover string carries through to `resolveProfile` so existing
 * `--profile=...` parsing remains untouched (R10-2 byte parity).
 */
export function parseStrategyArgs(rawArgs: string): ParsedStrategyArgs {
  const tokens = (rawArgs ?? '').split(/\s+/).filter(Boolean)
  let strategy: 'prompt-only' | 'observe' = 'observe'
  let explain = false
  let epsilonTarget: number | undefined
  let weights: CrEigWeights = DEFAULT_CR_EIG_WEIGHTS
  const errors: string[] = []
  const leftoverTokens: string[] = []

  for (let i = 0; i < tokens.length; i++) {
    const tok = tokens[i]!

    // --strategy=<value> | --strategy <value>
    const sm = STRATEGY_PATTERN.exec(tok)
    if (sm) {
      let value: string | null = sm[1] ?? null
      if (value === null && i + 1 < tokens.length) {
        value = tokens[i + 1]!
        i += 1
      }
      if (value === null) {
        errors.push('--strategy requires a value (observe | prompt-only)')
        continue
      }
      const lower = value.toLowerCase()
      if (lower === 'cr-eig' || lower === 'cr-eig+gtpo') {
        errors.push(
          `--strategy=${value} 已被 R5 移除,请使用 observe(默认)或 prompt-only`,
        )
        continue
      }
      if (lower === 'observe' || lower === 'prompt-only') {
        strategy = lower
        continue
      }
      errors.push(`unknown --strategy value: ${value}`)
      continue
    }

    // --weights="key=val,key=val" | --weights=key=val
    const wm = WEIGHTS_PATTERN.exec(tok)
    if (wm) {
      const raw = wm[2] ?? wm[3] ?? ''
      const parsed = parseWeightsOverride(raw, weights, errors)
      if (parsed) weights = parsed
      continue
    }

    // --epsilon=<float>
    const em = EPSILON_PATTERN.exec(tok)
    if (em) {
      let value: string | null = em[1] ?? null
      if (value === null && i + 1 < tokens.length) {
        value = tokens[i + 1]!
        i += 1
      }
      if (value === null) {
        errors.push('--epsilon requires a numeric value')
        continue
      }
      const num = Number(value)
      if (!Number.isFinite(num) || num < 0) {
        errors.push(`--epsilon must be a finite non-negative number (got ${value})`)
        continue
      }
      epsilonTarget = num
      continue
    }

    if (EXPLAIN_PATTERN.test(tok)) {
      explain = true
      continue
    }

    leftoverTokens.push(tok)
  }

  return {
    strategy,
    weights,
    explain,
    epsilonTarget,
    leftover: leftoverTokens.join(' '),
    errors,
  }
}

/**
 * Parse the `--weights` value (either `key=val,key=val` or single
 * `key=val`). Validates against R6-1 key set and finite non-negative
 * (or boolean for `useAdaptiveDelta`) values.
 */
function parseWeightsOverride(
  raw: string,
  current: CrEigWeights,
  errors: string[],
): CrEigWeights | null {
  const next: CrEigWeights = { ...current }
  if (!raw) return next
  const pairs = raw.split(',').map(p => p.trim()).filter(Boolean)
  for (const pair of pairs) {
    const eq = pair.indexOf('=')
    if (eq <= 0) {
      errors.push(`--weights pair has no '=': "${pair}"`)
      continue
    }
    const key = pair.slice(0, eq).trim()
    const valueRaw = pair.slice(eq + 1).trim()
    if (!ALLOWED_WEIGHT_KEYS.has(key as keyof CrEigWeights)) {
      errors.push(`--weights unknown key: "${key}"`)
      continue
    }
    if (key === 'useAdaptiveDelta') {
      const lower = valueRaw.toLowerCase()
      if (lower === 'true' || lower === '1') {
        ;(next as { useAdaptiveDelta: boolean }).useAdaptiveDelta = true
        continue
      }
      if (lower === 'false' || lower === '0') {
        ;(next as { useAdaptiveDelta: boolean }).useAdaptiveDelta = false
        continue
      }
      errors.push(`--weights useAdaptiveDelta must be true|false (got ${valueRaw})`)
      continue
    }
    const num = Number(valueRaw)
    if (!Number.isFinite(num) || num < 0) {
      errors.push(
        `--weights ${key} must be a finite non-negative number (got ${valueRaw})`,
      )
      continue
    }
    ;(next as Record<string, number | boolean>)[key] = num
  }
  return next
}

/**
 * Resolve an `OracleAnchor[]` from a profile's `oracles[]` text. v1 just
 * hashes the index + entire text — no Firecrawl pre-grounding here.
 */
function buildOracleAnchorsFromProfile(
  resolution: ProfileResolution,
): SidecarOptions['oracleAnchors'] {
  return resolution.profile.oracles.map((text, i) => ({
    id: `${resolution.profile.id}-oracle-${i}`,
    referenceText: text,
    source: 'profile' as const,
  }))
}

/**
 * Module-level WeakMap keeping each session's sidecar handle alive
 * across `getPromptForCommand` calls. (`type: 'prompt'` commands cannot
 * persist instance state, but the module is loaded once per process,
 * so a static map suffices.)
 */
const ACTIVE_SIDECARS = new Map<string, ReturnType<typeof startSidecar>>()

/**
 * Build the final prompt. R10-2 contract: when no `--strategy` /
 * `--weights` flags are present and the user passed a plain claim, the
 * leftover is identical to the original `rawArgs` (modulo whitespace
 * normalisation), so `resolveProfile` + `buildCcbTeamPrompt` produce a
 * byte-identical output to the pre-Super-Agent-Cluster version.
 */
const buildPrompt = (rawArgs: string): string => {
  const trimmed = (rawArgs ?? '').trim()
  if (!trimmed) {
    return applyInvocationGate(HELP_REPLY)
  }

  const parsed = parseStrategyArgs(trimmed)

  // Hard fail when --weights / --strategy etc. were syntactically wrong.
  if (parsed.errors.length > 0) {
    return [
      `用户调用了 \`/ccbteam\` 但参数有误:`,
      ...parsed.errors.map(e => `- ${e}`),
      '请修正后再试。',
      '不要做任何工具调用。',
    ].join('\n')
  }

  const resolved = resolveProfile(parsed.leftover)
  if (!resolved.claim) {
    return applyInvocationGate(HELP_REPLY)
  }

  // Mount sidecar when observe (default). prompt-only is the legacy
  // path — no math layer code runs.
  if (parsed.strategy === 'observe') {
    const sessionId = safeGetSessionId()
    const sessionDir = process.cwd() // tasks own session dir; for v1 we
                                     // co-locate audit log with cwd

    const opts: SidecarOptions = {
      strategy: 'observe',
      weights: parsed.weights,
      explain: parsed.explain,
      sessionId,
      sessionDir,
      profileId: resolved.profile.id,
      oracleAnchors: buildOracleAnchorsFromProfile(resolved),
      epsilonTarget: parsed.epsilonTarget,
    }

    const existing = ACTIVE_SIDECARS.get(sessionId)
    if (existing) {
      // Already running for this session — leave it alone.
    } else {
      const handle = startSidecar(opts)
      ACTIVE_SIDECARS.set(sessionId, handle)
    }
  }

  // R13: build the epistemic block once per call. Empty string under
  // prompt-only because R13-10 still injects, but we keep parity by
  // letting buildCcbTeamPrompt do the empty short-circuit.
  const epistemicHonestyBlock = buildEpistemicHonestyBlock()
  return buildCcbTeamPrompt(resolved, { epistemicHonestyBlock })
}

function safeGetSessionId(): string {
  try {
    return getSessionId()
  } catch {
    return `ad-hoc-${Date.now()}`
  }
}

const ccbteam: Command = {
  type: 'prompt',
  name: 'ccbteam',
  aliases: ['cav-team', 'ccb', 'consensus'],
  description:
    'Bootstrap a CAV-aware 4-chain consensus team. Supports engineering profiles: --profile=<reverse|decompile|security|data|generic>',
  progressMessage: 'orchestrating CAV consensus team',
  contentLength: 0,
  source: 'builtin',
  async getPromptForCommand(args): Promise<ContentBlockParam[]> {
    return [{ type: 'text', text: buildPrompt(args) }]
  },
}

export default ccbteam
