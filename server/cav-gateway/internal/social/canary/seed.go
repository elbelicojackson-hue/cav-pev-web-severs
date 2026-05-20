// Built-in canary task seeds.
//
// The seed corpus is embedded at compile time so the gateway can bootstrap a
// fresh BadgerDB pool without external files. Production deployments can
// supplement these via Pool.Upsert at runtime (the seed loader is idempotent —
// re-running with the same task ID is a no-op for the pool count).

package canary

import (
	"encoding/json"
	"fmt"

	_ "embed"
)

//go:embed seed_tasks.json
var defaultSeedJSON []byte

// LoadDefaultSeeds reads the embedded seed corpus and upserts every task
// into `pool`. Returns the count of tasks that were inserted (existing IDs
// count as zero — they are overwritten in place).
func LoadDefaultSeeds(pool *Pool) (int, error) {
	return LoadSeedJSON(pool, defaultSeedJSON)
}

// LoadSeedJSON parses a JSON document of the shape
//
//	[
//	  { "task": { ... }, "ground_truth": { ... } },
//	  ...
//	]
//
// and upserts every entry. The shape mirrors poolEnvelope (private to this
// package) so tests can hand-craft seed bytes.
func LoadSeedJSON(pool *Pool, data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	var entries []poolEnvelope
	if err := json.Unmarshal(data, &entries); err != nil {
		return 0, fmt.Errorf("seed unmarshal: %w", err)
	}
	count := 0
	for _, e := range entries {
		t := e.Task
		if t.ID == "" {
			continue
		}
		if t.GeneratedFrom == "" {
			t.GeneratedFrom = "seed"
		}
		if err := pool.Upsert(&t, e.GroundTruth); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
