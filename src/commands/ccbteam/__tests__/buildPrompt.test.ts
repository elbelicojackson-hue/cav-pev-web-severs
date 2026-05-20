/**
 * T20 — buildCcbTeamPrompt opts parameter tests.
 *
 * R10-2 contract: with no `opts`, output must be byte-for-byte
 * identical to the pre-Super-Agent-Cluster version. With `opts.
 * epistemicHonestyBlock` provided, the block is injected immediately
 * after the existing `<cav>` block.
 */

import { describe, expect, it } from 'bun:test'
import { buildCcbTeamPrompt } from '../buildPrompt.js'
import { resolveProfile } from '../profiles/index.js'

const claim = '量子计算机能在 2028 年破解 RSA-2048'

describe('buildCcbTeamPrompt — R10-2 backward compatibility', () => {
  it('single-arg call output equals two-arg call with undefined opts', () => {
    const r = resolveProfile(claim)
    const a = buildCcbTeamPrompt(r)
    const b = buildCcbTeamPrompt(r, undefined)
    expect(a).toBe(b)
  })

  it('two-arg call with empty object equals single-arg', () => {
    const r = resolveProfile(claim)
    const a = buildCcbTeamPrompt(r)
    const b = buildCcbTeamPrompt(r, {})
    expect(a).toBe(b)
  })

  it('two-arg call with undefined epistemicHonestyBlock equals single-arg', () => {
    const r = resolveProfile(claim)
    const a = buildCcbTeamPrompt(r)
    const b = buildCcbTeamPrompt(r, { epistemicHonestyBlock: undefined })
    expect(a).toBe(b)
  })

  it('two-arg call with empty-string epistemicHonestyBlock equals single-arg', () => {
    const r = resolveProfile(claim)
    const a = buildCcbTeamPrompt(r)
    const b = buildCcbTeamPrompt(r, { epistemicHonestyBlock: '' })
    expect(a).toBe(b)
  })

  it('output is deterministic for repeated calls', () => {
    const r = resolveProfile(claim)
    const a = buildCcbTeamPrompt(r)
    const b = buildCcbTeamPrompt(r)
    expect(a).toBe(b)
  })
})

describe('buildCcbTeamPrompt — epistemicHonestyBlock injection', () => {
  it('injects the block after the original CAV body', () => {
    const r = resolveProfile(claim)
    const block = 'INJECTED-EPISTEMIC-MARKER-12345'
    const result = buildCcbTeamPrompt(r, { epistemicHonestyBlock: block })
    expect(result).toContain(block)
  })

  it('block appears INSIDE the fenced CAV protocol block (i.e. visible to teammates)', () => {
    const r = resolveProfile(claim)
    const block = 'EPI-BODY-XYZ'
    const result = buildCcbTeamPrompt(r, { epistemicHonestyBlock: block })
    // Expect the block to appear before the fence-close `\`\`\`` that
    // wraps the per-agent prompt.
    const blockIdx = result.indexOf(block)
    expect(blockIdx).toBeGreaterThan(0)
    const closingFenceIdx = result.indexOf('\n```\n', blockIdx)
    expect(closingFenceIdx).toBeGreaterThan(blockIdx)
  })

  it('does not duplicate the original CAV body when injecting', () => {
    const r = resolveProfile(claim)
    const block = 'EPI-MARKER'
    const result = buildCcbTeamPrompt(r, { epistemicHonestyBlock: block })
    // CAV block heading appears exactly once
    const cavHeadCount = (result.match(/V2 多智能体共识协议/g) ?? []).length
    expect(cavHeadCount).toBe(1)
  })
})
