// Package nir defines the Normalized Intermediate Representation for Kubernetes YAML manifests.
// It preserves 1-based source coordinates, structural hierarchy, and provides language-neutral
// traversal for schema validation and diagnostic rule engines.
// See docs/yaml/YAML_VALIDATION_AND_NORMALIZATION.md §5 and SYSTEM_ARCHITECTURE.md §7.1.
package nir

import (
	"fmt"
	"strconv"
	"strings"
)

// NodeKind identifies the syntactic kind of a node in the NIR AST.
type NodeKind int

const (
	KindNull NodeKind = iota
	KindScalar
	KindMapping
	KindSequence
)

func (k NodeKind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindScalar:
		return "scalar"
	case KindMapping:
		return "mapping"
	case KindSequence:
		return "sequence"
	default:
		return "unknown"
	}
}

// ScalarType identifies the normalized primitive data type of a scalar node.
type ScalarType int

const (
	TypeUnknown ScalarType = iota
	TypeString
	TypeInt
	TypeFloat
	TypeBool
	TypeNull
)

func (t ScalarType) String() string {
	switch t {
	case TypeString:
		return "string"
	case TypeInt:
		return "integer"
	case TypeFloat:
		return "float"
	case TypeBool:
		return "boolean"
	case TypeNull:
		return "null"
	default:
		return "unknown"
	}
}

// Location represents an exact 1-based source span within an input file.
type Location struct {
	FilePath      string `json:"file_path"`
	DocumentIndex int    `json:"document_index"`
	Line          int    `json:"line"`
	Column        int    `json:"column"`
	ByteOffset    int    `json:"byte_offset"`
	EndLine       int    `json:"end_line,omitempty"`
	EndColumn     int    `json:"end_column,omitempty"`
}

func (l Location) String() string {
	return fmt.Sprintf("%s:%d:%d (doc #%d)", l.FilePath, l.Line, l.Column, l.DocumentIndex)
}

// MappingEntry represents a single key-value association within a Mapping node.
type MappingEntry struct {
	Key   *Node
	Value *Node
}

// Node represents a normalized AST node with full source attribution.
type Node struct {
	Kind       NodeKind
	Location   Location
	Tag        string
	Value      string     // Textual representation of scalar value
	ScalarType ScalarType // Inferred or schema-normalized type
	Fields     []MappingEntry
	Items      []*Node
}

// Document represents a single normalized YAML document within a stream.
type Document struct {
	Index    int
	Location Location
	Root     *Node
	Raw      []byte
}

// Get traverses a dot-separated object path (e.g., "metadata.name", "spec.replicas").
func (n *Node) Get(path string) *Node {
	if n == nil || path == "" {
		return n
	}
	parts := strings.Split(path, ".")
	curr := n

	for _, part := range parts {
		if curr == nil {
			return nil
		}
		if curr.Kind != KindMapping {
			return nil
		}
		found := false
		for _, entry := range curr.Fields {
			if entry.Key != nil && entry.Key.Value == part {
				curr = entry.Value
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return curr
}

// GetString returns the string value of a scalar field at path, if present.
func (n *Node) GetString(path string) (string, bool) {
	node := n.Get(path)
	if node == nil || node.Kind != KindScalar {
		return "", false
	}
	return node.Value, true
}

// GetInt returns the integer value of a scalar field at path, if present.
func (n *Node) GetInt(path string) (int64, bool) {
	node := n.Get(path)
	if node == nil || node.Kind != KindScalar {
		return 0, false
	}
	val, err := strconv.ParseInt(node.Value, 10, 64)
	if err != nil {
		return 0, false
	}
	return val, true
}

// GetBool returns the boolean value of a scalar field at path, if present.
func (n *Node) GetBool(path string) (bool, bool) {
	node := n.Get(path)
	if node == nil || node.Kind != KindScalar {
		return false, false
	}
	val, err := strconv.ParseBool(node.Value)
	if err != nil {
		return false, false
	}
	return val, true
}

// ToMap converts the NIR Mapping node into a generic Go map[string]any for schema validation.
func (n *Node) ToMap() any {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case KindNull:
		return nil
	case KindScalar:
		switch n.ScalarType {
		case TypeInt:
			if i, err := strconv.ParseInt(n.Value, 10, 64); err == nil {
				return i
			}
		case TypeFloat:
			if f, err := strconv.ParseFloat(n.Value, 64); err == nil {
				return f
			}
		case TypeBool:
			if b, err := strconv.ParseBool(n.Value); err == nil {
				return b
			}
		case TypeNull:
			return nil
		}
		return n.Value
	case KindSequence:
		list := make([]any, len(n.Items))
		for i, item := range n.Items {
			list[i] = item.ToMap()
		}
		return list
	case KindMapping:
		m := make(map[string]any, len(n.Fields))
		for _, entry := range n.Fields {
			if entry.Key != nil {
				m[entry.Key.Value] = entry.Value.ToMap()
			}
		}
		return m
	default:
		return n.Value
	}
}
