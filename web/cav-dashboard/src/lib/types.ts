// Mirror of server/cav-node/internal/praxon/types.go

export type PraxonClass =
  | "operational"
  | "deliberation_motion"
  | "deliberation_resolution";

export interface CausalSkeleton {
  subject: string;
  relation: string;
  object: string;
  mechanism_hypothesis: string;
  strength: number;
}

export interface UncertaintyGeometry {
  confidence: number;
  counterfactual_neighborhood: string;
  known_failure_modes: string[];
}

export interface Methodology {
  prior_source_tag: string;
  inference_method_tag: string;
  data_source_hashes: string[];
}

export interface Falsifiability {
  would_be_retracted_if: string;
  test_protocol_praxon_ref?: string;
}

export interface PraxonClaim {
  causal_skeleton: CausalSkeleton;
  uncertainty_geometry: UncertaintyGeometry;
  methodology: Methodology;
  falsifiability: Falsifiability;
}

export interface GroundingHandle {
  type: string;
  // tool_run
  tool_manifest_ref?: string;
  args_hash?: string;
  stdout_hash?: string;
  exit_code?: number;
  // canary_eig
  task_set_id?: string;
  measured_eig_bits?: number;
  methodology?: string;
  measured_at?: string;
  // demonstration_trace
  trace_hash?: string;
  task_description?: string;
  reasoning_steps_summary?: string;
  outcome?: string;
  trace_uri?: string;
  // praxon_ref
  praxon_id?: string;
  store_hint?: string;
  // formal_proof
  system?: string;
  proof_hash?: string;
  source_uri?: string;
  // dataset
  uri?: string;
  hash?: string;
}

export interface Provenance {
  derived_from: string[];
  consensus_episode?: string;
  challenges_survived?: string[];
}

export interface Praxon {
  version: string;
  praxon_id: string;
  praxon_class: PraxonClass;
  issuer: string;
  issued_at: string;
  claim: PraxonClaim;
  grounding: GroundingHandle[];
  provenance: Provenance;
  signature: string;
}

export interface Announcement {
  praxon_id: string;
  issuer: string;
  store_hints: string[];
  announced_at: string;
}

export interface AuditEntry {
  timestamp: string;
  event: string;
  praxon_id: string;
  issuer: string;
  detail: string;
}

export interface HealthStatus {
  status: string;
  protocol: string;
  version: string;
}
