package rule_test

import (
	"strings"
	"testing"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/parser"
	"github.com/kubecheck/kubecheck/pkg/rule"
)

func parseAndExtract(t *testing.T, yamlContent string) *inventory.Inventory {
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
	return inv
}

func TestRuleContainerImageTag(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: image-test
spec:
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: unpinned
        image: nginx # missing tag
      - name: latest-tag
        image: redis:latest # latest tag
      - name: pinned-tag
        image: postgres:15.4 # pinned
      - name: digest-pinned
        image: alpine@sha256:7144f7ba3b4796032db401f96fb5822769fb3d0c522c497d40661b30b70948e0
`
	inv := parseAndExtract(t, manifest)
	engine := rule.DefaultEngine()
	findings := engine.Evaluate(inv)

	unpinnedCount := 0
	for _, f := range findings {
		if f.RuleID == rule.RuleContainerImageTag {
			unpinnedCount++
		}
	}

	// Should flag "unpinned" and "latest-tag", but not "pinned-tag" or "digest-pinned"
	if unpinnedCount != 2 {
		t.Errorf("expected 2 RuleContainerImageTag findings, got %d: %v", unpinnedCount, findings)
	}
}

func TestRuleResourceLimitsRequests(t *testing.T) {
	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: res-test
spec:
  containers:
  - name: no-res
    image: nginx:1.25
  - name: with-res
    image: redis:7.0
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 512Mi
`
	inv := parseAndExtract(t, manifest)
	engine := rule.DefaultEngine()
	findings := engine.Evaluate(inv)

	found := false
	for _, f := range findings {
		if f.RuleID == rule.RuleResourceLimitsRequests && strings.Contains(f.Message, "no-res") {
			found = true
			if f.Severity != diagnostic.SeverityWarning {
				t.Errorf("expected warning severity, got %v", f.Severity)
			}
		}
	}

	if !found {
		t.Errorf("expected RuleResourceLimitsRequests on container no-res, got: %v", findings)
	}
}

func TestRuleContainerProbes(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: probe-test
spec:
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
        image: nginx:1.25
        # missing liveness & readiness
`
	inv := parseAndExtract(t, manifest)
	engine := rule.DefaultEngine()
	findings := engine.Evaluate(inv)

	found := false
	for _, f := range findings {
		if f.RuleID == rule.RuleContainerProbes {
			found = true
			if f.Severity != diagnostic.SeverityInfo {
				t.Errorf("expected info severity for probe rule, got %v", f.Severity)
			}
		}
	}

	if !found {
		t.Errorf("expected RuleContainerProbes finding, got: %v", findings)
	}
}

func TestRulePrivilegedAndEscalation(t *testing.T) {
	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: sec-test
spec:
  containers:
  - name: priv-container
    image: alpine:3.19
    securityContext:
      privileged: true
      allowPrivilegeEscalation: true
`
	inv := parseAndExtract(t, manifest)
	engine := rule.DefaultEngine()
	findings := engine.Evaluate(inv)

	foundPriv := false
	foundEsc := false

	for _, f := range findings {
		if f.RuleID == rule.RulePrivilegedContainer {
			foundPriv = true
			if f.Severity != diagnostic.SeverityError {
				t.Errorf("expected error severity for privileged container, got %v", f.Severity)
			}
		}
		if f.RuleID == rule.RulePrivilegeEscalation {
			foundEsc = true
		}
	}

	if !foundPriv {
		t.Errorf("expected RulePrivilegedContainer (SEC-CTX-001), got: %v", findings)
	}
	if !foundEsc {
		t.Errorf("expected RulePrivilegeEscalation (SEC-CTX-002), got: %v", findings)
	}
}

func TestRuleServiceMissingSelector(t *testing.T) {
	manifest := `apiVersion: v1
kind: Service
metadata:
  name: headless-manual
spec:
  clusterIP: None
  ports:
  - port: 80
---
apiVersion: v1
kind: Service
metadata:
  name: broken-svc
spec:
  ports:
  - port: 80
  # missing selector on standard ClusterIP service
`
	inv := parseAndExtract(t, manifest)
	engine := rule.DefaultEngine()
	findings := engine.Evaluate(inv)

	foundBroken := false
	for _, f := range findings {
		if f.RuleID == rule.RuleServiceMissingSelector {
			if strings.Contains(f.Message, "broken-svc") {
				foundBroken = true
			}
			if strings.Contains(f.Message, "headless-manual") {
				t.Errorf("headless service should not be flagged for missing selector")
			}
		}
	}

	if !foundBroken {
		t.Errorf("expected RuleServiceMissingSelector on broken-svc, got: %v", findings)
	}
}

func TestCronJobWorkloadRule(t *testing.T) {
	manifest := `apiVersion: batch/v1
kind: CronJob
metadata:
  name: cron-test
spec:
  schedule: "0 0 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
          - name: worker
            image: worker:latest
`
	inv := parseAndExtract(t, manifest)
	engine := rule.DefaultEngine()
	findings := engine.Evaluate(inv)

	found := false
	for _, f := range findings {
		if f.RuleID == rule.RuleContainerImageTag {
			found = true
		}
	}

	if !found {
		t.Errorf("expected RuleContainerImageTag inside CronJob pod template, got: %v", findings)
	}
}

func TestEngineRegisterAndEmpty(t *testing.T) {
	e := rule.NewEngine(nil)
	if len(e.Rules()) != 0 {
		t.Errorf("expected 0 rules, got %d", len(e.Rules()))
	}

	e.Register(&rule.Rule{
		ID:       "CUSTOM-001",
		Category: rule.CategoryMaintainability,
	})
	if len(e.Rules()) != 1 {
		t.Errorf("expected 1 rule after register, got %d", len(e.Rules()))
	}

	findings := e.Evaluate(nil)
	if findings != nil {
		t.Errorf("expected nil for nil inventory, got: %v", findings)
	}
}
