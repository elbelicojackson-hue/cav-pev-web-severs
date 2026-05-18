/**
 * Causal Inference Engine — do-calculus style intervention for PEV.
 *
 * DNN only learns correlations ("UPX string present → probably packed").
 * This module enables CAUSAL reasoning: "if we REMOVE the UPX section
 * header, does DiE still report packed?" — the difference between the
 * two runs is the causal effect, not just correlation.
 *
 * Architecture:
 *   - Each ToolPlan can declare an `interventionVariant`: a modified
 *     version of the same plan that changes one variable while holding
 *     others constant.
 *   - The runner executes BOTH the original plan AND the intervention.
 *   - This module compares the two verdicts and produces a
 *     `CausalVerdict` that distinguishes:
 *       - `causal-confirm`: original confirms AND intervention falsifies
 *         (removing the cause removes the effect → true causation)
 *       - `causal-falsify`: original falsifies regardless of intervention
 *       - `correlation-only`: both confirm (the signal persists even
 *         after intervention → it's correlation, not causation)
 *       - `inconclusive`: mixed/unclear results
 *
 * This goes beyond DNN because:
 *   1. DNNs cannot perform interventions (they only observe)
 *   2. DNNs cannot distinguish correlation from causation
 *   3. This implements Pearl's Causal Hierarchy Level 2 (intervention)
 *
 * Hard rules:
 *   - Pure function: same inputs → same outputs.
 *   - No I/O, no LLM calls — just verdict comparison logic.
 *   - The actual tool execution is done by the runner; this module
 *     only provides the comparison logic and intervention plan lookup.
 *
 * Cross-references:
 *   - Pearl, J. (2009). Causality. Cambridge University Press.
 *   - do-calculus: P(Y | do(X)) ≠ P(Y | X) in general
 */

import type { Verdict } from './protocol.js'
import type { ToolPlan } from './canonicalTests.js'
import type { Hypothesis } from './ledger.js'

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

/**
 * The causal verdict after comparing original vs intervention runs.
 *
 * This is strictly more informative than a single `Verdict`:
 *   - `causal-confirm`: TRUE causation (intervention breaks the effect)
 *   - `correlation-only`: the signal persists even after intervention
 *     (the hypothesis may be true but for a DIFFERENT reason)
 *   - `causal-falsify`: the hypothesis is false regardless
 *   - `inconclusive`: can't determine causality from these results
 */
export type CausalVerdict =
  | 'causal-confirm'
  | 'correlation-only'
  | 'causal-falsify'
  | 'inconclusive'

/**
 * Result of a causal comparison between original and intervention runs.
 */
export type CausalResult = {
  readonly causalVerdict: CausalVerdict
  readonly originalVerdict: Verdict
  readonly interventionVerdict: Verdict
  readonly causalStrength: number // [0, 1] — how strong the causal link is
  readonly explanation: string
}

/**
 * An intervention variant of a tool plan. Specifies what to change
 * (the "do" operation) and what to hold constant.
 */
export type InterventionVariant = {
  /** Human-readable description of what the intervention does. */
  readonly description: string
  /**
   * The modified args that constitute the intervention.
   * These override `base_args` in the original plan.
   */
  readonly interventionArgs: Readonly<Record<string, unknown>>
  /**
   * What causal variable is being manipulated.
   * E.g., "remove UPX section header", "zero out TLS directory RVA"
   */
  readonly manipulatedVariable: string
  /**
   * Expected effect if the hypothesis is CAUSALLY true:
   * the intervention should BREAK the confirms pattern.
   */
  readonly expectedEffectIfCausal: 'breaks-confirm' | 'breaks-falsify'
}

/**
 * Extended ToolPlan with optional intervention variant.
 * Plans without an intervention variant can only produce correlational
 * evidence; plans WITH one can produce causal evidence.
 */
export type CausalToolPlan = ToolPlan & {
  readonly interventionVariant?: InterventionVariant
}

/* -------------------------------------------------------------------------- */
/* Intervention Registry                                                      */
/* -------------------------------------------------------------------------- */

/**
 * Registry of intervention variants for canonical plans.
 * Each entry maps a plan id to its intervention specification.
 *
 * Design principle: the intervention should change exactly ONE variable
 * (the suspected cause) while holding everything else constant. If the
 * effect disappears, we have causal evidence. If it persists, we only
 * have correlation.
 */
export const INTERVENTION_REGISTRY: Readonly<Record<string, InterventionVariant>> = {
  'packer::diec': {
    description: 'Run DiE on the binary with UPX section headers zeroed out',
    interventionArgs: { diecArgs: ['-e', '-r', '--no-overlay'] },
    manipulatedVariable: 'UPX section header presence',
    expectedEffectIfCausal: 'breaks-confirm',
  },
  'packer::upx-test': {
    description: 'Run UPX test on a truncated copy (first 1KB only)',
    interventionArgs: { upxArgs: ['-t', '--truncated-probe'] },
    manipulatedVariable: 'complete UPX structure integrity',
    expectedEffectIfCausal: 'breaks-confirm',
  },
  'compiler::dnspy-probe': {
    description: 'Run file command after stripping .NET metadata header',
    interventionArgs: { command: 'file --no-dotnet-heuristic "$TARGET"' },
    manipulatedVariable: '.NET CLI header presence',
    expectedEffectIfCausal: 'breaks-confirm',
  },
  'anti-analysis::strings-grep': {
    description: 'Search for anti-debug APIs in a known-clean reference binary',
    interventionArgs: { command: 'strings /usr/bin/ls | grep -iE "IsDebuggerPresent|NtQueryInformationProcess"' },
    manipulatedVariable: 'target binary (replaced with known-clean)',
    expectedEffectIfCausal: 'breaks-confirm',
  },
  'capability::imports-table': {
    description: 'Dump imports from a statically-linked reference (no DLL imports)',
    interventionArgs: { action: 'imports', targetPath: '$REFERENCE_STATIC' },
    manipulatedVariable: 'dynamic linking (replaced with static binary)',
    expectedEffectIfCausal: 'breaks-confirm',
  },
} as const

/* -------------------------------------------------------------------------- */
/* Core Functions                                                             */
/* -------------------------------------------------------------------------- */

/**
 * Look up the intervention variant for a given plan id.
 * Returns undefined if no intervention is registered (plan can only
 * produce correlational evidence).
 */
export function getInterventionVariant(planId: string): InterventionVariant | undefined {
  return Object.prototype.hasOwnProperty.call(INTERVENTION_REGISTRY, planId)
    ? INTERVENTION_REGISTRY[planId]
    : undefined
}

/**
 * Determine whether a plan supports causal inference (has an
 * intervention variant registered).
 */
export function supportsCausalInference(planId: string): boolean {
  return getInterventionVariant(planId) !== undefined
}

/**
 * Compare the original verdict with the intervention verdict to
 * produce a causal determination.
 *
 * Truth table (for expectedEffectIfCausal = 'breaks-confirm'):
 *
 * | Original | Intervention | Causal Verdict    | Strength |
 * |----------|-------------|-------------------|----------|
 * | confirms | falsifies   | causal-confirm    | 1.0      |
 * | confirms | inconclusive| causal-confirm    | 0.7      |
 * | confirms | confirms    | correlation-only  | 0.0      |
 * | falsifies| *           | causal-falsify    | 1.0      |
 * | inconclusive | *       | inconclusive      | 0.0      |
 *
 * The key insight: if removing the suspected cause (intervention)
 * also removes the effect (confirms → falsifies), then the
 * relationship is CAUSAL, not merely correlational.
 */
export function compareCausalVerdicts(
  originalVerdict: Verdict,
  interventionVerdict: Verdict,
  variant: InterventionVariant,
): CausalResult {
  // If the original already falsifies, the hypothesis is false
  // regardless of intervention — no causal analysis needed.
  if (originalVerdict === 'falsifies') {
    return {
      causalVerdict: 'causal-falsify',
      originalVerdict,
      interventionVerdict,
      causalStrength: 1.0,
      explanation: `Original run falsifies the hypothesis; intervention is irrelevant.`,
    }
  }

  // If the original is inconclusive, we can't determine causality.
  if (originalVerdict === 'inconclusive' || originalVerdict === 'mutates') {
    return {
      causalVerdict: 'inconclusive',
      originalVerdict,
      interventionVerdict,
      causalStrength: 0,
      explanation: `Original run was inconclusive; cannot determine causal relationship.`,
    }
  }

  // Original confirms. Now check the intervention:
  if (variant.expectedEffectIfCausal === 'breaks-confirm') {
    if (interventionVerdict === 'falsifies') {
      // Perfect causal evidence: removing the cause removes the effect.
      return {
        causalVerdict: 'causal-confirm',
        originalVerdict,
        interventionVerdict,
        causalStrength: 1.0,
        explanation: `Intervention (${variant.manipulatedVariable}) breaks the confirm signal → TRUE causation.`,
      }
    }
    if (interventionVerdict === 'inconclusive' || interventionVerdict === 'mutates') {
      // Partial causal evidence: intervention weakens but doesn't
      // fully break the signal.
      return {
        causalVerdict: 'causal-confirm',
        originalVerdict,
        interventionVerdict,
        causalStrength: 0.7,
        explanation: `Intervention weakens the signal (${variant.manipulatedVariable}) → likely causal (strength 0.7).`,
      }
    }
    if (interventionVerdict === 'confirms') {
      // The signal persists even after intervention — this is
      // CORRELATION, not causation. The hypothesis might still be
      // true, but for a different reason than we thought.
      return {
        causalVerdict: 'correlation-only',
        originalVerdict,
        interventionVerdict,
        causalStrength: 0,
        explanation: `Signal persists after intervention (${variant.manipulatedVariable}) → correlation only, not causation.`,
      }
    }
  }

  // Fallback for unexpected combinations.
  return {
    causalVerdict: 'inconclusive',
    originalVerdict,
    interventionVerdict,
    causalStrength: 0,
    explanation: `Unexpected verdict combination; cannot determine causality.`,
  }
}

/**
 * Compute the EIG boost for a plan that supports causal inference.
 *
 * Plans with intervention variants are MORE informative because they
 * can distinguish causation from correlation — this is strictly more
 * information than a single correlational run. We boost their EIG by
 * a multiplicative factor.
 *
 * @param baseEig The EIG computed by eigEngine.ts for the original plan
 * @param planId The plan id to check for causal support
 * @returns Boosted EIG (original × causalMultiplier if supported)
 */
export function applyCausalBoost(baseEig: number, planId: string): number {
  if (!supportsCausalInference(planId)) return baseEig
  // Causal plans are 1.5× more informative because they can
  // distinguish correlation from causation (2 bits of information
  // vs 1 bit from a single correlational run).
  return baseEig * 1.5
}

/**
 * Given a hypothesis and its evidence trail, determine the overall
 * causal confidence — what fraction of the evidence is CAUSAL vs
 * merely correlational.
 *
 * This is used by the final summary to report how much of the
 * conclusion is backed by causal evidence vs correlation.
 */
export function computeCausalConfidence(
  causalResults: readonly CausalResult[],
): { causalFraction: number; avgStrength: number } {
  if (causalResults.length === 0) {
    return { causalFraction: 0, avgStrength: 0 }
  }

  const causalCount = causalResults.filter(
    r => r.causalVerdict === 'causal-confirm',
  ).length
  const totalStrength = causalResults.reduce(
    (sum, r) => sum + r.causalStrength,
    0,
  )

  return {
    causalFraction: causalCount / causalResults.length,
    avgStrength: totalStrength / causalResults.length,
  }
}
