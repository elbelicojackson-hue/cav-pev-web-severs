/**
 * Causal Inference Engine — unit tests.
 *
 * Validates the do-calculus style intervention comparison logic that
 * distinguishes causation from correlation.
 */

import { describe, expect, test } from 'bun:test'
import {
  applyCausalBoost,
  compareCausalVerdicts,
  computeCausalConfidence,
  getInterventionVariant,
  INTERVENTION_REGISTRY,
  supportsCausalInference,
  type CausalResult,
  type InterventionVariant,
} from '../causalEngine.js'

/* -------------------------------------------------------------------------- */
/* Intervention Registry                                                      */
/* -------------------------------------------------------------------------- */

describe('INTERVENTION_REGISTRY', () => {
  test('packer::diec has an intervention variant', () => {
    const v = getInterventionVariant('packer::diec')
    expect(v).toBeDefined()
    expect(v!.manipulatedVariable).toContain('UPX')
    expect(v!.expectedEffectIfCausal).toBe('breaks-confirm')
  })

  test('unknown plan returns undefined', () => {
    expect(getInterventionVariant('does::not-exist')).toBeUndefined()
  })

  test('supportsCausalInference returns true for registered plans', () => {
    expect(supportsCausalInference('packer::diec')).toBe(true)
    expect(supportsCausalInference('packer::upx-test')).toBe(true)
    expect(supportsCausalInference('compiler::dnspy-probe')).toBe(true)
  })

  test('supportsCausalInference returns false for unregistered plans', () => {
    expect(supportsCausalInference('algorithm::ida-script-dump')).toBe(false)
    expect(supportsCausalInference('protocol::tshark')).toBe(false)
  })

  test('every registered variant has required fields', () => {
    for (const [id, v] of Object.entries(INTERVENTION_REGISTRY)) {
      expect(v.description.length).toBeGreaterThan(0)
      expect(v.manipulatedVariable.length).toBeGreaterThan(0)
      expect(v.expectedEffectIfCausal).toBe('breaks-confirm')
      expect(typeof v.interventionArgs).toBe('object')
    }
  })
})

/* -------------------------------------------------------------------------- */
/* compareCausalVerdicts                                                      */
/* -------------------------------------------------------------------------- */

describe('compareCausalVerdicts', () => {
  const variant: InterventionVariant = {
    description: 'test intervention',
    interventionArgs: {},
    manipulatedVariable: 'test variable',
    expectedEffectIfCausal: 'breaks-confirm',
  }

  test('confirms + falsifies → causal-confirm (strength 1.0)', () => {
    const r = compareCausalVerdicts('confirms', 'falsifies', variant)
    expect(r.causalVerdict).toBe('causal-confirm')
    expect(r.causalStrength).toBe(1.0)
    expect(r.explanation).toContain('TRUE causation')
  })

  test('confirms + inconclusive → causal-confirm (strength 0.7)', () => {
    const r = compareCausalVerdicts('confirms', 'inconclusive', variant)
    expect(r.causalVerdict).toBe('causal-confirm')
    expect(r.causalStrength).toBe(0.7)
    expect(r.explanation).toContain('likely causal')
  })

  test('confirms + confirms → correlation-only (strength 0)', () => {
    const r = compareCausalVerdicts('confirms', 'confirms', variant)
    expect(r.causalVerdict).toBe('correlation-only')
    expect(r.causalStrength).toBe(0)
    expect(r.explanation).toContain('correlation only')
  })

  test('falsifies + anything → causal-falsify', () => {
    for (const iv of ['confirms', 'falsifies', 'inconclusive', 'mutates'] as const) {
      const r = compareCausalVerdicts('falsifies', iv, variant)
      expect(r.causalVerdict).toBe('causal-falsify')
      expect(r.causalStrength).toBe(1.0)
    }
  })

  test('inconclusive + anything → inconclusive', () => {
    for (const iv of ['confirms', 'falsifies', 'inconclusive'] as const) {
      const r = compareCausalVerdicts('inconclusive', iv, variant)
      expect(r.causalVerdict).toBe('inconclusive')
      expect(r.causalStrength).toBe(0)
    }
  })

  test('mutates + anything → inconclusive', () => {
    const r = compareCausalVerdicts('mutates', 'confirms', variant)
    expect(r.causalVerdict).toBe('inconclusive')
  })

  test('result always contains both original and intervention verdicts', () => {
    const r = compareCausalVerdicts('confirms', 'falsifies', variant)
    expect(r.originalVerdict).toBe('confirms')
    expect(r.interventionVerdict).toBe('falsifies')
  })
})

/* -------------------------------------------------------------------------- */
/* applyCausalBoost                                                           */
/* -------------------------------------------------------------------------- */

describe('applyCausalBoost', () => {
  test('plan with intervention gets 1.5× EIG boost', () => {
    const boosted = applyCausalBoost(0.5, 'packer::diec')
    expect(boosted).toBeCloseTo(0.75, 5)
  })

  test('plan without intervention gets no boost', () => {
    const boosted = applyCausalBoost(0.5, 'algorithm::ida-script-dump')
    expect(boosted).toBe(0.5)
  })

  test('boost of 0 EIG is still 0', () => {
    expect(applyCausalBoost(0, 'packer::diec')).toBe(0)
  })
})

/* -------------------------------------------------------------------------- */
/* computeCausalConfidence                                                    */
/* -------------------------------------------------------------------------- */

describe('computeCausalConfidence', () => {
  test('empty results → 0 fraction, 0 strength', () => {
    const { causalFraction, avgStrength } = computeCausalConfidence([])
    expect(causalFraction).toBe(0)
    expect(avgStrength).toBe(0)
  })

  test('all causal-confirm → fraction=1.0', () => {
    const results: CausalResult[] = [
      { causalVerdict: 'causal-confirm', originalVerdict: 'confirms', interventionVerdict: 'falsifies', causalStrength: 1.0, explanation: '' },
      { causalVerdict: 'causal-confirm', originalVerdict: 'confirms', interventionVerdict: 'inconclusive', causalStrength: 0.7, explanation: '' },
    ]
    const { causalFraction, avgStrength } = computeCausalConfidence(results)
    expect(causalFraction).toBe(1.0)
    expect(avgStrength).toBeCloseTo(0.85, 5)
  })

  test('mixed results → correct fraction', () => {
    const results: CausalResult[] = [
      { causalVerdict: 'causal-confirm', originalVerdict: 'confirms', interventionVerdict: 'falsifies', causalStrength: 1.0, explanation: '' },
      { causalVerdict: 'correlation-only', originalVerdict: 'confirms', interventionVerdict: 'confirms', causalStrength: 0, explanation: '' },
      { causalVerdict: 'inconclusive', originalVerdict: 'inconclusive', interventionVerdict: 'confirms', causalStrength: 0, explanation: '' },
    ]
    const { causalFraction, avgStrength } = computeCausalConfidence(results)
    expect(causalFraction).toBeCloseTo(1 / 3, 5)
    expect(avgStrength).toBeCloseTo(1 / 3, 5)
  })
})
