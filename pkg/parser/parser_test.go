package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kubecheck/kubecheck/pkg/nir"
	"github.com/kubecheck/kubecheck/pkg/parser"
)

func TestParseValidSingleDocument(t *testing.T) {
	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: production
spec:
  containers:
  - name: app
    image: nginx:1.25
`
	p := parser.Default()
	docs, findings, err := p.ParseBytes("test.yaml", []byte(manifest))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) > 0 {
		t.Fatalf("expected 0 findings, got: %v", findings)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	doc := docs[0]
	if doc.Index != 0 {
		t.Errorf("expected doc index 0, got %d", doc.Index)
	}

	// Verify NIR structure and locations
	kind, ok := doc.Root.GetString("kind")
	if !ok || kind != "Pod" {
		t.Errorf("expected kind Pod, got %s (ok=%t)", kind, ok)
	}

	name, ok := doc.Root.GetString("metadata.name")
	if !ok || name != "test-pod" {
		t.Errorf("expected name test-pod, got %s (ok=%t)", name, ok)
	}
}

func TestParseMultiDocumentStream(t *testing.T) {
	stream := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
---
apiVersion: v1
kind: Service
metadata:
  name: web-service
`
	p := parser.Default()
	docs, findings, err := p.ParseBytes("multi.yaml", []byte(stream))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) > 0 {
		t.Fatalf("expected 0 findings, got: %v", findings)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(docs))
	}

	if docs[0].Index != 0 || docs[1].Index != 1 {
		t.Errorf("document indexing incorrect: %d, %d", docs[0].Index, docs[1].Index)
	}

	d0Kind, _ := docs[0].Root.GetString("kind")
	d1Kind, _ := docs[1].Root.GetString("kind")

	if d0Kind != "Deployment" || d1Kind != "Service" {
		t.Errorf("expected Deployment and Service, got %s and %s", d0Kind, d1Kind)
	}
}

func TestDuplicateMappingKeys(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 1
  replicas: 2
`
	p := parser.Default()
	docs, findings, err := p.ParseBytes("dup.yaml", []byte(manifest))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) == 0 {
		t.Fatalf("expected document to be returned despite duplicate key")
	}

	// Assert duplicate key finding KCHK-SYN-001
	foundDup := false
	for _, f := range findings {
		if f.RuleID == parser.RuleDuplicateKey {
			foundDup = true
			if !strings.Contains(f.Message, "Duplicate mapping key \"replicas\"") {
				t.Errorf("unexpected duplicate key message: %s", f.Message)
			}
			if f.Location.Line != 7 {
				t.Errorf("expected duplicate key location at line 7, got line %d", f.Location.Line)
			}
		}
	}

	if !foundDup {
		t.Errorf("expected RuleDuplicateKey (KCHK-SYN-001) finding, got: %v", findings)
	}
}

func TestUnsafeTagRejection(t *testing.T) {
	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: exploit
data:
  payload: !!python/object/apply:os.system ["rm -rf /"]
`
	p := parser.Default()
	_, findings, _ := p.ParseBytes("exploit.yaml", []byte(manifest))

	foundUnsafe := false
	for _, f := range findings {
		if f.RuleID == parser.RuleUnsafeTag {
			foundUnsafe = true
			if !strings.Contains(f.Message, "Unsafe constructor tag") {
				t.Errorf("unexpected unsafe tag message: %s", f.Message)
			}
		}
	}

	if !foundUnsafe {
		t.Errorf("expected RuleUnsafeTag (KCHK-SEC-001) finding, got: %v", findings)
	}
}

func TestGovernorDepthLimit(t *testing.T) {
	manifest := `a:
  b:
    c:
      d:
        e: deep
`
	// Configure max depth = 3
	limits := parser.DefaultLimits()
	limits.MaxDepth = 3

	p := parser.New(limits)
	_, findings, _ := p.ParseBytes("deep.yaml", []byte(manifest))

	foundDepth := false
	for _, f := range findings {
		if f.RuleID == parser.RuleLimitDepth {
			foundDepth = true
		}
	}

	if !foundDepth {
		t.Errorf("expected RuleLimitDepth (KCHK-GOV-001) finding, got: %v", findings)
	}
}

func TestGovernorAliasBomb(t *testing.T) {
	// Billion-laughs alias bomb
	bomb := `
a: &a ["lol","lol","lol","lol","lol"]
b: &b [*a,*a,*a,*a,*a]
c: &c [*b,*b,*b,*b,*b]
d: &d [*c,*c,*c,*c,*c]
e: &e [*d,*d,*d,*d,*d]
root: *e
`
	// Set low alias threshold
	limits := parser.DefaultLimits()
	limits.MaxAliases = 10

	p := parser.New(limits)
	_, findings, _ := p.ParseBytes("bomb.yaml", []byte(bomb))

	foundBomb := false
	for _, f := range findings {
		if f.RuleID == parser.RuleLimitAliases {
			foundBomb = true
		}
	}

	if !foundBomb {
		t.Errorf("expected RuleLimitAliases (KCHK-GOV-002) finding, got: %v", findings)
	}
}

func TestInvalidUTF8Rejection(t *testing.T) {
	badBytes := []byte("apiVersion: v1\nkind: Pod\n\xff\xfe\xfd")

	p := parser.Default()
	_, findings, err := p.ParseBytes("bad.yaml", badBytes)

	if err == nil {
		t.Errorf("expected error on invalid UTF-8 sequence")
	}

	foundUTF8 := false
	for _, f := range findings {
		if f.RuleID == parser.RuleInvalidUTF8 {
			foundUTF8 = true
		}
	}

	if !foundUTF8 {
		t.Errorf("expected RuleInvalidUTF8 (KCHK-SYN-003) finding, got: %v", findings)
	}
}

func TestParseReaderAndFile(t *testing.T) {
	manifest := "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: dev\n"
	p := parser.Default()

	// Test ParseReader
	docs, findings, err := p.ParseReader("stream.yaml", strings.NewReader(manifest))
	if err != nil || len(findings) > 0 || len(docs) != 1 {
		t.Fatalf("ParseReader failed: docs=%d, findings=%d, err=%v", len(docs), len(findings), err)
	}

	// Test ParseFile with temp file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "ns.yaml")
	if err := os.WriteFile(filePath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	docs, findings, err = p.ParseFile(filePath)
	if err != nil || len(findings) > 0 || len(docs) != 1 {
		t.Fatalf("ParseFile failed: docs=%d, findings=%d, err=%v", len(docs), len(findings), err)
	}

	// Test ParseFile nonexistent
	_, _, err = p.ParseFile(filepath.Join(tmpDir, "missing.yaml"))
	if err == nil {
		t.Errorf("expected error for nonexistent file")
	}
}

func TestDiscoverFiles(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	f1 := filepath.Join(tmpDir, "root.yaml")
	f2 := filepath.Join(tmpDir, "root.yml")
	f3 := filepath.Join(tmpDir, "ignored.txt")
	f4 := filepath.Join(subDir, "nested.yaml")

	for _, f := range []string{f1, f2, f3, f4} {
		if err := os.WriteFile(f, []byte("a: b\n"), 0o600); err != nil {
			t.Fatalf("failed to write file %s: %v", f, err)
		}
	}

	// Stdin "-"
	files, err := parser.DiscoverFiles([]string{"-"}, false)
	if err != nil || len(files) != 1 || files[0] != "-" {
		t.Errorf("expected [-], got %v, err=%v", files, err)
	}

	// Single file
	files, err = parser.DiscoverFiles([]string{f1}, false)
	if err != nil || len(files) != 1 || files[0] != f1 {
		t.Errorf("expected [%s], got %v, err=%v", f1, files, err)
	}

	// Directory non-recursive (should find root.yaml and root.yml, but not ignored.txt or nested.yaml)
	files, err = parser.DiscoverFiles([]string{tmpDir}, false)
	if err != nil {
		t.Fatalf("DiscoverFiles failed: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files non-recursive, got %d: %v", len(files), files)
	}

	// Directory recursive (should find root.yaml, root.yml, nested.yaml)
	files, err = parser.DiscoverFiles([]string{tmpDir}, true)
	if err != nil {
		t.Fatalf("DiscoverFiles recursive failed: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("expected 3 files recursive, got %d: %v", len(files), files)
	}

	// Non-existent path
	_, err = parser.DiscoverFiles([]string{"/nonexistent/path/xyz"}, false)
	if err == nil {
		t.Errorf("expected error for nonexistent path")
	}
}

func TestFindingStringAndLimits(t *testing.T) {
	finding := parser.Finding{
		RuleID:   parser.RuleDuplicateKey,
		Severity: "error",
		Message:  "test message",
		Location: nir.Location{FilePath: "test.yaml", Line: 5, Column: 2},
	}
	str := finding.String()
	if !strings.Contains(str, "KCHK-SYN-001") || !strings.Contains(str, "test.yaml:5:2") {
		t.Errorf("unexpected Finding.String(): %s", str)
	}

	// Test MaxInputBytes limit
	limits := parser.DefaultLimits()
	limits.MaxInputBytes = 10
	p := parser.New(limits)
	_, _, err := p.ParseBytes("test.yaml", []byte("too long content exceeding limit"))
	if err == nil {
		t.Errorf("expected error exceeding MaxInputBytes")
	}
}

func TestScalarTypeInference(t *testing.T) {
	manifest := `
intVal: 42
floatVal: 3.1415
boolVal: true
nullVal: null
tildeNull: ~
strVal: "hello world"
plainStr: kubernetes
`
	p := parser.Default()
	docs, findings, err := p.ParseBytes("scalars.yaml", []byte(manifest))
	if err != nil || len(findings) > 0 || len(docs) != 1 {
		t.Fatalf("unexpected failure: err=%v, findings=%v", err, findings)
	}

	root := docs[0].Root
	if node := root.Get("intVal"); node == nil || node.ScalarType != nir.TypeInt {
		t.Errorf("expected TypeInt for intVal, got %v", node)
	}
	if node := root.Get("floatVal"); node == nil || node.ScalarType != nir.TypeFloat {
		t.Errorf("expected TypeFloat for floatVal, got %v", node)
	}
	if node := root.Get("boolVal"); node == nil || node.ScalarType != nir.TypeBool {
		t.Errorf("expected TypeBool for boolVal, got %v", node)
	}
	if node := root.Get("nullVal"); node == nil || node.ScalarType != nir.TypeNull {
		t.Errorf("expected TypeNull for nullVal, got %v", node)
	}
	if node := root.Get("tildeNull"); node == nil || node.ScalarType != nir.TypeNull {
		t.Errorf("expected TypeNull for tildeNull, got %v", node)
	}
	if node := root.Get("strVal"); node == nil || node.ScalarType != nir.TypeString {
		t.Errorf("expected TypeString for strVal, got %v", node)
	}
	if node := root.Get("plainStr"); node == nil || node.ScalarType != nir.TypeString {
		t.Errorf("expected TypeString for plainStr, got %v", node)
	}
}
