package parser

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/kubecheck/kubecheck/pkg/nir"
)

// Parser manages safe YAML manifest decoding, governor enforcement, and NIR transformation.
type Parser struct {
	limits Limits
}

// New constructs a Parser configured with specified resource limits.
func New(limits Limits) *Parser {
	return &Parser{limits: limits}
}

// Default constructs a Parser with normative production limits.
func Default() *Parser {
	return New(DefaultLimits())
}

// ParseBytes decodes YAML bytes from a named file or stream into NIR documents.
func (p *Parser) ParseBytes(filePath string, data []byte) ([]*nir.Document, []Finding, error) {
	// 1. Enforce input byte size governor
	if int64(len(data)) > p.limits.MaxInputBytes {
		return nil, []Finding{
			{
				RuleID:        RuleLimitInputSize,
				Severity:      "error",
				Message:       fmt.Sprintf("[%s] input size (%d bytes) exceeds maximum limit of %d bytes", LimitIDInputBytes, len(data), p.limits.MaxInputBytes),
				Location:      nir.Location{FilePath: filePath, Line: 1, Column: 1},
				EvidenceClass: "deterministic",
			},
		}, errors.New("input size limit exceeded")
	}

	// 2. Validate strict UTF-8 byte stream per FAIL-INPUT-001
	if !utf8.Valid(data) {
		return nil, []Finding{
			{
				RuleID:        RuleInvalidUTF8,
				Severity:      "error",
				Message:       "invalid UTF-8 byte sequence detected; manifest stream cannot be safely decoded",
				Location:      nir.Location{FilePath: filePath, Line: 1, Column: 1},
				EvidenceClass: "deterministic",
			},
		}, errors.New("invalid UTF-8 sequence")
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	gov := NewGovernor(p.limits)

	var (
		documents []*nir.Document
		findings  []Finding
		docIndex  = 0
	)

	for {
		var rootNode yaml.Node
		err := decoder.Decode(&rootNode)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			// Parse error in current document; isolate and record finding
			findings = append(findings, Finding{
				RuleID:        RuleSyntaxError,
				Severity:      "error",
				Message:       fmt.Sprintf("YAML syntax error: %v", err),
				Location:      nir.Location{FilePath: filePath, DocumentIndex: docIndex, Line: 1, Column: 1},
				EvidenceClass: "deterministic",
			})
			break
		}

		if err := gov.RecordDocument(); err != nil {
			findings = append(findings, Finding{
				RuleID:        RuleLimitDocuments,
				Severity:      "error",
				Message:       err.Error(),
				Location:      nir.Location{FilePath: filePath, DocumentIndex: docIndex, Line: 1, Column: 1},
				EvidenceClass: "deterministic",
			})
			break
		}

		// Process document root node
		docFindings, nirDoc := p.convertDocument(filePath, docIndex, &rootNode, gov)
		findings = append(findings, docFindings...)
		if nirDoc != nil {
			documents = append(documents, nirDoc)
		}
		docIndex++
	}

	return documents, findings, nil
}

// ParseReader decodes YAML content from an io.Reader stream.
func (p *Parser) ParseReader(filePath string, r io.Reader) ([]*nir.Document, []Finding, error) {
	// Read bounded bytes to avoid memory exhaustion on unbounded stream
	lr := io.LimitReader(r, p.limits.MaxInputBytes+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read manifest stream: %w", err)
	}
	return p.ParseBytes(filePath, data)
}

// ParseFile reads and parses a manifest file from the local file system.
func (p *Parser) ParseFile(filePath string) ([]*nir.Document, []Finding, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}
	if info.Size() > p.limits.MaxInputBytes {
		return nil, []Finding{
			{
				RuleID:        RuleLimitInputSize,
				Severity:      "error",
				Message:       fmt.Sprintf("[%s] file size (%d bytes) exceeds maximum limit of %d bytes", LimitIDInputBytes, info.Size(), p.limits.MaxInputBytes),
				Location:      nir.Location{FilePath: filePath, Line: 1, Column: 1},
				EvidenceClass: "deterministic",
			},
		}, errors.New("file size limit exceeded")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	return p.ParseBytes(filePath, data)
}

func (p *Parser) convertDocument(filePath string, docIdx int, root *yaml.Node, gov *Governor) ([]Finding, *nir.Document) {
	var findings []Finding

	// A document root node in yaml.v3 is usually Kind == DocumentNode
	nodeToProcess := root
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		nodeToProcess = root.Content[0]
	}

	nirRoot, nodeFindings := p.convertNode(filePath, docIdx, nodeToProcess, 1, gov)
	findings = append(findings, nodeFindings...)

	doc := &nir.Document{
		Index: docIdx,
		Location: nir.Location{
			FilePath:      filePath,
			DocumentIndex: docIdx,
			Line:          root.Line,
			Column:        root.Column,
		},
		Root: nirRoot,
	}

	return findings, doc
}

func (p *Parser) convertNode(filePath string, docIdx int, yn *yaml.Node, depth int, gov *Governor) (*nir.Node, []Finding) {
	if yn == nil {
		return nil, nil
	}

	var findings []Finding

	// Enforce depth limit
	if err := gov.CheckDepth(depth); err != nil {
		findings = append(findings, Finding{
			RuleID:        RuleLimitDepth,
			Severity:      "error",
			Message:       err.Error(),
			Location:      nir.Location{FilePath: filePath, DocumentIndex: docIdx, Line: yn.Line, Column: yn.Column},
			EvidenceClass: "deterministic",
		})
		return nil, findings
	}

	// Check unsafe YAML tags (e.g. !!python/object, !!ruby/object)
	if yn.Tag != "" && isUnsafeTag(yn.Tag) {
		findings = append(findings, Finding{
			RuleID:        RuleUnsafeTag,
			Severity:      "error",
			Message:       fmt.Sprintf("Unsafe constructor tag %q rejected for security compliance", yn.Tag),
			Location:      nir.Location{FilePath: filePath, DocumentIndex: docIdx, Line: yn.Line, Column: yn.Column},
			EvidenceClass: "deterministic",
		})
	}

	loc := nir.Location{
		FilePath:      filePath,
		DocumentIndex: docIdx,
		Line:          yn.Line,
		Column:        yn.Column,
	}

	node := &nir.Node{
		Location: loc,
		Tag:      yn.Tag,
		Value:    yn.Value,
	}

	switch yn.Kind {
	case yaml.ScalarNode:
		if err := gov.CheckScalarSize(len(yn.Value)); err != nil {
			findings = append(findings, Finding{
				RuleID:        RuleLimitScalar,
				Severity:      "error",
				Message:       err.Error(),
				Location:      loc,
				EvidenceClass: "deterministic",
			})
			return nil, findings
		}
		node.Kind = nir.KindScalar
		node.ScalarType = inferScalarType(yn)

	case yaml.MappingNode:
		node.Kind = nir.KindMapping
		seenKeys := make(map[string]nir.Location)

		// MappingNode content is [key0, val0, key1, val1, ...]
		for i := 0; i < len(yn.Content); i += 2 {
			if err := gov.RecordCollectionEntry(); err != nil {
				findings = append(findings, Finding{
					RuleID:        RuleLimitDocuments,
					Severity:      "error",
					Message:       err.Error(),
					Location:      loc,
					EvidenceClass: "deterministic",
				})
				return node, findings
			}

			keyNode := yn.Content[i]
			var valNode *yaml.Node
			if i+1 < len(yn.Content) {
				valNode = yn.Content[i+1]
			}

			// Duplicate Key Detection
			keyVal := keyNode.Value
			keyLoc := nir.Location{
				FilePath:      filePath,
				DocumentIndex: docIdx,
				Line:          keyNode.Line,
				Column:        keyNode.Column,
			}

			if origLoc, exists := seenKeys[keyVal]; exists {
				findings = append(findings, Finding{
					RuleID:        RuleDuplicateKey,
					Severity:      "error",
					Message:       fmt.Sprintf("Duplicate mapping key %q (first defined at line %d, column %d)", keyVal, origLoc.Line, origLoc.Column),
					Location:      keyLoc,
					EvidenceClass: "deterministic",
				})
			} else {
				seenKeys[keyVal] = keyLoc
			}

			kNIR, kFindings := p.convertNode(filePath, docIdx, keyNode, depth+1, gov)
			findings = append(findings, kFindings...)

			vNIR, vFindings := p.convertNode(filePath, docIdx, valNode, depth+1, gov)
			findings = append(findings, vFindings...)

			node.Fields = append(node.Fields, nir.MappingEntry{
				Key:   kNIR,
				Value: vNIR,
			})
		}

	case yaml.SequenceNode:
		node.Kind = nir.KindSequence
		for _, item := range yn.Content {
			if err := gov.RecordCollectionEntry(); err != nil {
				break
			}
			itemNIR, itemFindings := p.convertNode(filePath, docIdx, item, depth+1, gov)
			findings = append(findings, itemFindings...)
			node.Items = append(node.Items, itemNIR)
		}

	case yaml.AliasNode:
		if err := gov.RecordAlias(); err != nil {
			findings = append(findings, Finding{
				RuleID:        RuleLimitAliases,
				Severity:      "error",
				Message:       err.Error(),
				Location:      loc,
				EvidenceClass: "deterministic",
			})
			return nil, findings
		}
		// Expand alias target safely with bounded depth
		if yn.Alias != nil {
			return p.convertNode(filePath, docIdx, yn.Alias, depth+1, gov)
		}

	default:
		node.Kind = nir.KindNull
	}

	return node, findings
}

func isUnsafeTag(tag string) bool {
	lower := strings.ToLower(tag)
	unsafePrefixes := []string{
		"!!python/",
		"!!ruby/",
		"!!java/",
		"!!php/",
		"!!perl/",
		"!!js/",
		"tag:yaml.org,2002:python",
		"tag:yaml.org,2002:ruby",
		"tag:yaml.org,2002:java",
		"tag:yaml.org,2002:php",
		"tag:yaml.org,2002:perl",
		"tag:yaml.org,2002:js",
	}
	for _, prefix := range unsafePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func inferScalarType(yn *yaml.Node) nir.ScalarType {
	switch yn.Tag {
	case "!!int":
		return nir.TypeInt
	case "!!float":
		return nir.TypeFloat
	case "!!bool":
		return nir.TypeBool
	case "!!null":
		return nir.TypeNull
	case "!!str":
		return nir.TypeString
	}

	// Auto-inference from text
	val := yn.Value
	if val == "null" || val == "~" || val == "" {
		return nir.TypeNull
	}
	if val == "true" || val == "false" {
		return nir.TypeBool
	}
	if _, err := strconv.ParseInt(val, 10, 64); err == nil {
		return nir.TypeInt
	}
	if _, err := strconv.ParseFloat(val, 64); err == nil {
		return nir.TypeFloat
	}
	return nir.TypeString
}

// DiscoverFiles gathers all YAML files from a list of paths, respecting recursion and ignore patterns.
func DiscoverFiles(paths []string, recursive bool) ([]string, error) {
	var results []string

	for _, p := range paths {
		if p == "-" {
			results = append(results, "-")
			continue
		}

		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}

		if !info.IsDir() {
			results = append(results, p)
			continue
		}

		walkFn := func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if !recursive && path != p {
					return filepath.SkipDir
				}
				// Skip hidden directories (e.g. .git)
				if strings.HasPrefix(d.Name(), ".") && path != p {
					return filepath.SkipDir
				}
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".yaml" || ext == ".yml" {
				results = append(results, path)
			}
			return nil
		}

		if err := filepath.WalkDir(p, walkFn); err != nil {
			return nil, err
		}
	}

	return results, nil
}
