// Package praxon defines the core Praxon data types for the CAV protocol.
// These types mirror the JSON wire schema defined in cav-praxon/design.md.
package praxon

// Praxon is the fundamental cognitive particle of CAV.
type Praxon struct {
	Version    string         `json:"version"`
	PraxonID   string         `json:"praxon_id"`
	PraxonClass PraxonClass   `json:"praxon_class"`
	Issuer     string         `json:"issuer"`
	IssuedAt   string         `json:"issued_at"`
	Claim      PraxonClaim    `json:"claim"`
	Grounding  []GroundingHandle `json:"grounding"`
	Provenance Provenance     `json:"provenance"`
	Signature  string         `json:"signature"`
}

// PraxonClass distinguishes operational traffic from deliberation.
type PraxonClass string

const (
	ClassOperational          PraxonClass = "operational"
	ClassDeliberationMotion   PraxonClass = "deliberation_motion"
	ClassDeliberationResolution PraxonClass = "deliberation_resolution"
)

// PraxonClaim contains the four Axiom-3 required components.
type PraxonClaim struct {
	CausalSkeleton      CausalSkeleton      `json:"causal_skeleton"`
	UncertaintyGeometry UncertaintyGeometry `json:"uncertainty_geometry"`
	Methodology         Methodology         `json:"methodology"`
	Falsifiability      Falsifiability      `json:"falsifiability"`
}

type CausalSkeleton struct {
	Subject             string  `json:"subject"`
	Relation            string  `json:"relation"`
	Object              string  `json:"object"`
	MechanismHypothesis string  `json:"mechanism_hypothesis"`
	Strength            float64 `json:"strength"`
}

type UncertaintyGeometry struct {
	Confidence                 float64  `json:"confidence"`
	CounterfactualNeighborhood string   `json:"counterfactual_neighborhood"`
	KnownFailureModes          []string `json:"known_failure_modes"`
}

type Methodology struct {
	PriorSourceTag     string   `json:"prior_source_tag"`
	InferenceMethodTag string   `json:"inference_method_tag"`
	DataSourceHashes   []string `json:"data_source_hashes"`
}

type Falsifiability struct {
	WouldBeRetractedIf     string `json:"would_be_retracted_if"`
	TestProtocolPraxonRef  string `json:"test_protocol_praxon_ref,omitempty"`
}

// GroundingHandle is a polymorphic grounding reference.
// The Type field determines which other fields are populated.
type GroundingHandle struct {
	Type string `json:"type"`

	// tool_run fields
	ToolManifestRef string `json:"tool_manifest_ref,omitempty"`
	ArgsHash        string `json:"args_hash,omitempty"`
	StdoutHash      string `json:"stdout_hash,omitempty"`
	ExitCode        *int   `json:"exit_code,omitempty"`

	// canary_eig fields
	TaskSetID       string  `json:"task_set_id,omitempty"`
	MeasuredEigBits float64 `json:"measured_eig_bits,omitempty"`
	EigMethodology  string  `json:"methodology,omitempty"`
	MeasuredAt      string  `json:"measured_at,omitempty"`

	// demonstration_trace fields
	TraceHash              string `json:"trace_hash,omitempty"`
	TaskDescription        string `json:"task_description,omitempty"`
	ReasoningStepsSummary  string `json:"reasoning_steps_summary,omitempty"`
	Outcome                string `json:"outcome,omitempty"`
	TraceURI               string `json:"trace_uri,omitempty"`

	// praxon_ref fields
	PraxonID  string `json:"praxon_id,omitempty"`
	StoreHint string `json:"store_hint,omitempty"`

	// formal_proof fields
	System    string `json:"system,omitempty"`
	ProofHash string `json:"proof_hash,omitempty"`
	SourceURI string `json:"source_uri,omitempty"`

	// dataset fields
	URI  string `json:"uri,omitempty"`
	Hash string `json:"hash,omitempty"`
}

// Provenance tracks the derivation history of a Praxon.
type Provenance struct {
	DerivedFrom        []string `json:"derived_from"`
	ConsensusEpisode   string   `json:"consensus_episode,omitempty"`
	ChallengesSurvived []string `json:"challenges_survived,omitempty"`
}

// Announcement is the lightweight notification sent to peers.
type Announcement struct {
	PraxonID    string   `json:"praxon_id"`
	Issuer      string   `json:"issuer"`
	StoreHints  []string `json:"store_hints"`
	AnnouncedAt string   `json:"announced_at"`
}
