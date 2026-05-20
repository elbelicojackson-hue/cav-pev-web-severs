package config

// This file is reserved for additional validation helpers that may be
// needed as the config schema grows. The core Validate() function lives
// in config.go. Keeping this file ensures the package structure matches
// the design document's directory layout.
//
// Future additions:
// - Custom role schema validation
// - Cross-NPC constraint checks (e.g., diversity enforcement)
// - Config diff for SIGHUP reload decisions
