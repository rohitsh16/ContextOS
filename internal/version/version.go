package version

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// ApplicationVersion defines the semantic version of ContextOS runtime.
	ApplicationVersion = "0.7.0"

	// ConfigSchemaVersion defines the schema version for .contextos/config.toml and generated provider configs.
	ConfigSchemaVersion = 1

	// StoreSchemaVersion defines the persistent database schema version.
	StoreSchemaVersion = 2

	// ProtocolVersion defines the Model Context Protocol (MCP) version ContextOS implements.
	ProtocolVersion = "2026-07-28"

	// EngineVersion describes the active allocation and inference engine release.
	EngineVersion = "ASC-1.4"
)

// Info returns a formatted version string.
func Info() string {
	return fmt.Sprintf("ContextOS v%s (Engine: %s, Schema: v%d, MCP: %s)",
		ApplicationVersion, EngineVersion, ConfigSchemaVersion, ProtocolVersion)
}

// Compare compares two semver strings (e.g. "0.7.0" vs "0.6.0").
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2.
func Compare(v1, v2 string) int {
	p1 := parseParts(v1)
	p2 := parseParts(v2)
	for i := 0; i < 3; i++ {
		if p1[i] < p2[i] {
			return -1
		}
		if p1[i] > p2[i] {
			return 1
		}
	}
	return 0
}

func parseParts(v string) [3]int {
	var res [3]int
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		n, _ := strconv.Atoi(parts[i])
		res[i] = n
	}
	return res
}
