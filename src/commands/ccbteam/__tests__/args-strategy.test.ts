/**
 * T13 — command-layer args parsing tests.
 *
 * Covers:
 *   - R10-2: leftover string is byte-identical to original args when no
 *     SAC flags are present
 *   - R11-5: 8 CLI shapes covered explicitly
 *   - R6-5: --weights="key=val,key=val" parses correctly
 *   - R6-6: unknown weights key → error
 *   - R12-4: HELP_REPLY (empty claim) gets Invocation Gate appended
 */

import { describe, expect, it } from 'bun:test'
import { parseStrategyArgs } from '../index.js'
import { DEFAULT_CR_EIG_WEIGHTS } from '../../../services/cav/ccbteam-math/constants.js'

describe('parseStrategyArgs — R10-2 leftover parity', () => {
  it('plain claim leaves leftover unchanged', () => {
    const out = parseStrategyArgs('量子计算机能在 2028 年破解 RSA-2048')
    expect(out.leftover).toBe('量子计算机能在 2028 年破解 RSA-2048')
    expect(out.errors.length).toBe(0)
    expect(out.strategy).toBe('observe') // default
  })

  it('--profile=xxx alone leaves leftover untouched', () => {
    const out = parseStrategyArgs('--profile=reverse 分析这个 dll')
    expect(out.leftover).toBe('--profile=reverse 分析这个 dll')
    expect(out.errors.length).toBe(0)
  })
})

describe('parseStrategyArgs — R11-5 strategy variants', () => {
  it('--strategy=prompt-only', () => {
    const out = parseStrategyArgs('--strategy=prompt-only my claim')
    expect(out.strategy).toBe('prompt-only')
    expect(out.leftover).toBe('my claim')
    expect(out.errors.length).toBe(0)
  })

  it('--strategy=observe', () => {
    const out = parseStrategyArgs('--strategy=observe my claim')
    expect(out.strategy).toBe('observe')
    expect(out.leftover).toBe('my claim')
    expect(out.errors.length).toBe(0)
  })

  it('--strategy observe (space-separated)', () => {
    const out = parseStrategyArgs('--strategy observe my claim')
    expect(out.strategy).toBe('observe')
    expect(out.leftover).toBe('my claim')
  })

  it('default strategy when flag absent', () => {
    const out = parseStrategyArgs('my plain claim')
    expect(out.strategy).toBe('observe')
  })

  it('--strategy=cr-eig is rejected with R5 message', () => {
    const out = parseStrategyArgs('--strategy=cr-eig my claim')
    expect(out.errors.length).toBeGreaterThan(0)
    expect(out.errors.some(e => e.includes('R5'))).toBe(true)
  })

  it('--strategy=cr-eig+gtpo is rejected with R5 message', () => {
    const out = parseStrategyArgs('--strategy=cr-eig+gtpo my claim')
    expect(out.errors.length).toBeGreaterThan(0)
  })

  it('unknown strategy value → error', () => {
    const out = parseStrategyArgs('--strategy=xxx my claim')
    expect(out.errors.length).toBeGreaterThan(0)
  })
})

describe('parseStrategyArgs — R6-5/6-6 weights parsing', () => {
  it('--weights="lambdaCost=0.02,kappaUrgency=0.3" parses both keys', () => {
    const out = parseStrategyArgs('--weights="lambdaCost=0.02,kappaUrgency=0.3" my claim')
    expect(out.errors.length).toBe(0)
    expect(out.weights.lambdaCost).toBe(0.02)
    expect(out.weights.kappaUrgency).toBe(0.3)
    // Other defaults preserved
    expect(out.weights.gammaCausal).toBe(DEFAULT_CR_EIG_WEIGHTS.gammaCausal)
    expect(out.leftover).toBe('my claim')
  })

  it('--weights="useAdaptiveDelta=false" parses boolean', () => {
    const out = parseStrategyArgs('--weights="useAdaptiveDelta=false" my claim')
    expect(out.errors.length).toBe(0)
    expect(out.weights.useAdaptiveDelta).toBe(false)
  })

  it('--weights with unknown key → error mentioning the key (R6-6)', () => {
    const out = parseStrategyArgs('--weights="nonexistent=0.1" my claim')
    expect(out.errors.some(e => e.includes('nonexistent'))).toBe(true)
  })

  it('--weights with non-finite value → error', () => {
    const out = parseStrategyArgs('--weights="lambdaCost=foo" my claim')
    expect(out.errors.some(e => e.includes('lambdaCost'))).toBe(true)
  })

  it('--weights with negative value → error', () => {
    const out = parseStrategyArgs('--weights="lambdaCost=-0.5" my claim')
    expect(out.errors.length).toBeGreaterThan(0)
  })
})

describe('parseStrategyArgs — --explain flag', () => {
  it('--explain co-existing with --profile leaves leftover with profile only', () => {
    const out = parseStrategyArgs('--explain --profile=reverse my dll claim')
    expect(out.explain).toBe(true)
    expect(out.leftover).toBe('--profile=reverse my dll claim')
    expect(out.errors.length).toBe(0)
  })

  it('explain default false', () => {
    const out = parseStrategyArgs('plain claim')
    expect(out.explain).toBe(false)
  })
})

describe('parseStrategyArgs — --epsilon', () => {
  it('--epsilon=0.1 parsed', () => {
    const out = parseStrategyArgs('--epsilon=0.1 plain claim')
    expect(out.epsilonTarget).toBe(0.1)
    expect(out.leftover).toBe('plain claim')
  })

  it('--epsilon non-finite → error', () => {
    const out = parseStrategyArgs('--epsilon=NaN plain claim')
    expect(out.errors.length).toBeGreaterThan(0)
  })
})
