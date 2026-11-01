// Package parser implements secure, bounded YAML decoding, duplicate-key detection,
// safe tag enforcement, and normalization into NIR documents.
// See docs/yaml/YAML_SECURITY_AND_PARSER_POLICY.md and docs/yaml/YAML_PARSING_AND_EDGE_CASES.md.
package parser

import "fmt"

// Resource limit constant identifiers matching normative specifications.
const (
	LimitIDInputBytes  = "LIM-YAML-SIZE-01"
	LimitIDDocuments   = "LIM-YAML-DOCS-01"
	LimitIDDepth       = "LIM-YAML-DEPTH-01"
	LimitIDAliases     = "LIM-YAML-ALIAS-01"
	LimitIDScalarBytes = "LIM-YAML-SCALAR-01"
	LimitIDCollections = "LIM-YAML-COLLECT-01"
)

// Limits defines the configurable resource ceiling bounds enforced by the parser governor.
type Limits struct {
	MaxInputBytes        int64 `json:"max_input_bytes"`
	MaxDocuments         int   `json:"max_documents"`
	MaxDepth             int   `json:"max_depth"`
	MaxAliases           int   `json:"max_aliases"`
	MaxScalarBytes       int   `json:"max_scalar_bytes"`
	MaxCollectionEntries int   `json:"max_collection_entries"`
}

// DefaultLimits initializes normative production resource limits.
func DefaultLimits() Limits {
	return Limits{
		MaxInputBytes:        10 * 1024 * 1024, // 10 MiB
		MaxDocuments:         1000,
		MaxDepth:             256,
		MaxAliases:           1000,
		MaxScalarBytes:       10 * 1024 * 1024, // 10 MiB
		MaxCollectionEntries: 100000,
	}
}

// Governor tracks consumption across an input stream to prevent DoS attacks.
type Governor struct {
	limits        Limits
	documentCount int
	aliasCount    int
	entryCount    int
}

// NewGovernor constructs a Governor initialized with specified limits.
func NewGovernor(limits Limits) *Governor {
	return &Governor{limits: limits}
}

// RecordDocument increments and bounds total document count in stream.
func (g *Governor) RecordDocument() error {
	g.documentCount++
	if g.documentCount > g.limits.MaxDocuments {
		return fmt.Errorf("[%s] stream document count (%d) exceeded limit of %d",
			LimitIDDocuments, g.documentCount, g.limits.MaxDocuments)
	}
	return nil
}

// RecordAlias tracks and bounds alias expansions (Billion-Laughs defense).
func (g *Governor) RecordAlias() error {
	g.aliasCount++
	if g.aliasCount > g.limits.MaxAliases {
		return fmt.Errorf("[%s] alias expansions (%d) exceeded limit of %d",
			LimitIDAliases, g.aliasCount, g.limits.MaxAliases)
	}
	return nil
}

// CheckDepth enforces stack overflow protection.
func (g *Governor) CheckDepth(depth int) error {
	if depth > g.limits.MaxDepth {
		return fmt.Errorf("[%s] nesting depth (%d) exceeded limit of %d",
			LimitIDDepth, depth, g.limits.MaxDepth)
	}
	return nil
}

// CheckScalarSize bounds individual scalar allocation.
func (g *Governor) CheckScalarSize(size int) error {
	if size > g.limits.MaxScalarBytes {
		return fmt.Errorf("[%s] scalar size (%d bytes) exceeded limit of %d bytes",
			LimitIDScalarBytes, size, g.limits.MaxScalarBytes)
	}
	return nil
}

// RecordCollectionEntry bounds total collection element count across document.
func (g *Governor) RecordCollectionEntry() error {
	g.entryCount++
	if g.entryCount > g.limits.MaxCollectionEntries {
		return fmt.Errorf("[%s] total collection entries (%d) exceeded limit of %d",
			LimitIDCollections, g.entryCount, g.limits.MaxCollectionEntries)
	}
	return nil
}
