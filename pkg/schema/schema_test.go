package schema_test

import (
	"strings"
	"testing"

	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/parser"
	"github.com/kubecheck/kubecheck/pkg/schema"
)

func parseAndExtract(t *testing.T, yamlContent string) *inventory.Resource {
	t.Helper()
	p := parser.Default()
	docs, pFindings, err := p.ParseBytes("test.yaml", []byte(yamlContent))
	if err != nil || len(pFindings) > 0 {
		t.Fatalf("parser failed: err=%v, findings=%v", err, pFindings)
	}
	inv, invFindings := inventory.Extract(docs)
	if len(invFindings) > 0 {
		t.Fatalf("inventory extraction failed: %v", invFindings)
	}
	if len(inv.Resources) == 0 {
		t.Fatalf("no resources extracted")
	}
	return inv.Resources[0]
}

func TestValidPodValidation(t *testing.T) {
	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: default
spec:
  restartPolicy: Always
  containers:
  - name: nginx
    image: nginx:1.25
    ports:
    - containerPort: 80
      protocol: TCP
`
	res := parseAndExtract(t, manifest)
	v := schema.NewValidator(nil)
	findings := v.Validate(res, "1.31")

	if len(findings) > 0 {
		t.Fatalf("expected 0 findings for valid Pod, got: %v", findings)
	}
}

func TestValidDeploymentValidation(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
  namespace: default
spec:
  replicas: 3
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: web
        image: nginx
`
	res := parseAndExtract(t, manifest)
	v := schema.NewValidator(nil)
	findings := v.Validate(res, "1.31")

	if len(findings) > 0 {
		t.Fatalf("expected 0 findings for valid Deployment, got: %v", findings)
	}
}

func TestUnknownFieldDetection(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
spec:
  replicaCount: 3 # unknown field (should be replicas)
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: web
        image: nginx
`
	res := parseAndExtract(t, manifest)
	v := schema.NewValidator(nil)
	findings := v.Validate(res, "1.31")

	foundUnknown := false
	for _, f := range findings {
		if f.RuleID == schema.RuleUnknownField {
			foundUnknown = true
			if !strings.Contains(f.Message, "Unknown field \"replicaCount\"") {
				t.Errorf("unexpected message: %s", f.Message)
			}
			if f.Severity != "warning" {
				t.Errorf("expected warning severity for unknown field, got %s", f.Severity)
			}
		}
	}
	if !foundUnknown {
		t.Errorf("expected RuleUnknownField (K8S-SCHEMA-001), got: %v", findings)
	}
}

func TestMissingRequiredField(t *testing.T) {
	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: no-containers
spec:
  restartPolicy: Always
  # containers is required
`
	res := parseAndExtract(t, manifest)
	v := schema.NewValidator(nil)
	findings := v.Validate(res, "1.31")

	foundMissing := false
	for _, f := range findings {
		if f.RuleID == schema.RuleMissingRequired {
			foundMissing = true
			if !strings.Contains(f.Message, "Missing required field \"containers\"") {
				t.Errorf("unexpected message: %s", f.Message)
			}
		}
	}
	if !foundMissing {
		t.Errorf("expected RuleMissingRequired (K8S-SCHEMA-003), got: %v", findings)
	}
}

func TestWrongTypeDetection(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: bad-type
spec:
  replicas: "three" # should be integer
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: web
        image: 12345 # should be string
`
	res := parseAndExtract(t, manifest)
	v := schema.NewValidator(nil)
	findings := v.Validate(res, "1.31")

	countWrongType := 0
	for _, f := range findings {
		if f.RuleID == schema.RuleWrongType {
			countWrongType++
		}
	}
	if countWrongType < 2 {
		t.Errorf("expected at least 2 RuleWrongType findings, got %d: %v", countWrongType, findings)
	}
}

func TestInvalidEnumDetection(t *testing.T) {
	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: bad-enum
spec:
  restartPolicy: Sometimes # invalid enum (must be Always, OnFailure, Never)
  containers:
  - name: app
    image: nginx
`
	res := parseAndExtract(t, manifest)
	v := schema.NewValidator(nil)
	findings := v.Validate(res, "1.31")

	foundEnum := false
	for _, f := range findings {
		if f.RuleID == schema.RuleInvalidEnum {
			foundEnum = true
			if !strings.Contains(f.Message, "Invalid enum value \"Sometimes\"") {
				t.Errorf("unexpected message: %s", f.Message)
			}
		}
	}
	if !foundEnum {
		t.Errorf("expected RuleInvalidEnum (K8S-SCHEMA-004), got: %v", findings)
	}
}

func TestAPIVersionLifecycle(t *testing.T) {
	cronJobYAML := `apiVersion: batch/v1beta1
kind: CronJob
metadata:
  name: old-cron
spec:
  schedule: "*/5 * * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: job
            image: busybox
`
	res := parseAndExtract(t, cronJobYAML)
	v := schema.NewValidator(nil)

	// Target 1.24: Deprecated warning (DEP-API-001)
	findings24 := v.Validate(res, "1.24")
	foundDep := false
	for _, f := range findings24 {
		if f.RuleID == schema.RuleDeprecatedAPI {
			foundDep = true
			if !strings.Contains(f.Message, "deprecated since Kubernetes v1.21") {
				t.Errorf("unexpected deprecation message: %s", f.Message)
			}
		}
	}
	if !foundDep {
		t.Errorf("expected RuleDeprecatedAPI (DEP-API-001) in v1.24, got: %v", findings24)
	}

	// Target 1.25: Removed error (DEP-API-002)
	findings25 := v.Validate(res, "1.25")
	foundRem := false
	for _, f := range findings25 {
		if f.RuleID == schema.RuleRemovedAPI {
			foundRem = true
			if !strings.Contains(f.Message, "removed in Kubernetes v1.25") {
				t.Errorf("unexpected removed message: %s", f.Message)
			}
		}
	}
	if !foundRem {
		t.Errorf("expected RuleRemovedAPI (DEP-API-002) in v1.25, got: %v", findings25)
	}
}

func TestUnknownGVKWarning(t *testing.T) {
	manifest := `apiVersion: custom.example.com/v1alpha1
kind: Widget
metadata:
  name: my-widget
spec:
  foo: bar
`
	res := parseAndExtract(t, manifest)
	v := schema.NewValidator(nil)
	findings := v.Validate(res, "1.31")

	foundUnknownGVK := false
	for _, f := range findings {
		if f.RuleID == schema.RuleUnknownGVK {
			foundUnknownGVK = true
			if f.Severity != "warning" {
				t.Errorf("expected warning for unknown GVK, got %s", f.Severity)
			}
		}
	}
	if !foundUnknownGVK {
		t.Errorf("expected RuleUnknownGVK (K8S-GVK-009), got: %v", findings)
	}
}

func TestParseMinorVersionAndFindingString(t *testing.T) {
	if schema.ParseMinorVersion("1.31") != 31 {
		t.Errorf("expected 31, got %d", schema.ParseMinorVersion("1.31"))
	}
	if schema.ParseMinorVersion("v1.29.2") != 29 {
		t.Errorf("expected 29, got %d", schema.ParseMinorVersion("v1.29.2"))
	}
	if schema.ParseMinorVersion("28") != 28 {
		t.Errorf("expected 28, got %d", schema.ParseMinorVersion("28"))
	}
	if schema.ParseMinorVersion("invalid") != 0 {
		t.Errorf("expected 0 for invalid, got %d", schema.ParseMinorVersion("invalid"))
	}

	finding := schema.Finding{
		RuleID:   schema.RuleUnknownField,
		Severity: "warning",
		Message:  "unknown field",
	}
	if !strings.Contains(finding.String(), "K8S-SCHEMA-001") {
		t.Errorf("unexpected finding.String(): %s", finding.String())
	}
}
