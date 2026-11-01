package inventory_test

import (
	"strings"
	"testing"

	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/parser"
)

func TestExtractValidResources(t *testing.T) {
	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: web-pod
  namespace: prod
  labels:
    tier: frontend
  annotations:
    owner: devops
spec:
  containers:
  - name: nginx
    image: nginx
---
apiVersion: v1
kind: Namespace
metadata:
  name: prod
`
	p := parser.Default()
	docs, pFindings, err := p.ParseBytes("test.yaml", []byte(manifest))
	if err != nil || len(pFindings) > 0 {
		t.Fatalf("parser failed: err=%v, findings=%v", err, pFindings)
	}

	inv, findings := inventory.Extract(docs)
	if len(findings) > 0 {
		t.Fatalf("expected 0 inventory findings, got: %v", findings)
	}

	if len(inv.Resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(inv.Resources))
	}

	// Pod: namespaced
	pod := inv.Resources[0]
	if pod.ID.Kind != "Pod" || pod.ID.Group != "" || pod.ID.Version != "v1" {
		t.Errorf("unexpected pod GVK: %+v", pod.ID)
	}
	if pod.ID.Namespace != "prod" || pod.ID.Name != "web-pod" {
		t.Errorf("unexpected pod namespace/name: %s/%s", pod.ID.Namespace, pod.ID.Name)
	}
	if pod.IsClusterScoped {
		t.Errorf("expected pod to NOT be cluster scoped")
	}
	if pod.Labels["tier"] != "frontend" || pod.Annotations["owner"] != "devops" {
		t.Errorf("unexpected labels/annotations: %+v, %+v", pod.Labels, pod.Annotations)
	}

	// Namespace: cluster-scoped
	ns := inv.Resources[1]
	if ns.ID.Kind != "Namespace" || ns.ID.Name != "prod" {
		t.Errorf("unexpected namespace: %+v", ns.ID)
	}
	if !ns.IsClusterScoped {
		t.Errorf("expected namespace to be cluster-scoped")
	}
	if ns.ID.Namespace != "" {
		t.Errorf("expected empty namespace for cluster-scoped resource, got %q", ns.ID.Namespace)
	}

	// Query helpers
	if found := inv.GetByKind("Pod"); len(found) != 1 {
		t.Errorf("GetByKind(Pod) expected 1, got %d", len(found))
	}
	if found := inv.GetByScopeKey(pod.ID.ScopeKey()); found == nil {
		t.Errorf("GetByScopeKey failed for pod")
	}
	if str := pod.ID.String(); !strings.Contains(str, "Pod/prod/web-pod") {
		t.Errorf("unexpected ResourceID.String(): %s", str)
	}
}

func TestExtractListResource(t *testing.T) {
	manifest := `apiVersion: v1
kind: List
items:
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: dep-1
    namespace: test
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: dep-2
    namespace: test
`
	p := parser.Default()
	docs, _, _ := p.ParseBytes("list.yaml", []byte(manifest))

	inv, findings := inventory.Extract(docs)
	if len(findings) > 0 {
		t.Fatalf("unexpected findings: %v", findings)
	}
	if len(inv.Resources) != 2 {
		t.Fatalf("expected 2 unwrapped resources from List, got %d", len(inv.Resources))
	}
	if inv.Resources[0].ID.Name != "dep-1" || inv.Resources[1].ID.Name != "dep-2" {
		t.Errorf("unexpected resource names: %s, %s", inv.Resources[0].ID.Name, inv.Resources[1].ID.Name)
	}
}

func TestDuplicateResourceDetection(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: prod
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: prod
`
	p := parser.Default()
	docs, _, _ := p.ParseBytes("dup.yaml", []byte(manifest))

	inv, findings := inventory.Extract(docs)
	if len(inv.Resources) != 1 {
		t.Errorf("expected only first resource to be indexed, got %d", len(inv.Resources))
	}

	foundDup := false
	for _, f := range findings {
		if f.RuleID == inventory.RuleDuplicateResource {
			foundDup = true
			if !strings.Contains(f.Message, "Duplicate resource declaration") {
				t.Errorf("unexpected message: %s", f.Message)
			}
		}
	}
	if !foundDup {
		t.Errorf("expected RuleDuplicateResource (MAINT-DUP-001) finding, got: %v", findings)
	}
}

func TestRootValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		ruleID   string
	}{
		{
			name:     "root not mapping",
			manifest: "- a\n- b\n",
			ruleID:   inventory.RuleRootNotMapping,
		},
		{
			name:     "missing apiVersion",
			manifest: "kind: Pod\nmetadata:\n  name: p\n",
			ruleID:   inventory.RuleMissingAPIVersion,
		},
		{
			name:     "apiVersion not string",
			manifest: "apiVersion: 123\nkind: Pod\nmetadata:\n  name: p\n",
			ruleID:   inventory.RuleAPIVersionNotStr,
		},
		{
			name:     "missing kind",
			manifest: "apiVersion: v1\nmetadata:\n  name: p\n",
			ruleID:   inventory.RuleMissingKind,
		},
		{
			name:     "kind not string",
			manifest: "apiVersion: v1\nkind: true\nmetadata:\n  name: p\n",
			ruleID:   inventory.RuleKindNotStr,
		},
		{
			name:     "invalid apiVersion format",
			manifest: "apiVersion: a/b/c\nkind: Pod\nmetadata:\n  name: p\n",
			ruleID:   inventory.RuleInvalidAPIVersion,
		},
		{
			name:     "missing metadata",
			manifest: "apiVersion: v1\nkind: Pod\n",
			ruleID:   inventory.RuleMissingMetadata,
		},
		{
			name:     "metadata not mapping",
			manifest: "apiVersion: v1\nkind: Pod\nmetadata: invalid\n",
			ruleID:   inventory.RuleMetadataNotMapping,
		},
		{
			name:     "missing name",
			manifest: "apiVersion: v1\nkind: Pod\nmetadata:\n  namespace: default\n",
			ruleID:   inventory.RuleMissingName,
		},
	}

	p := parser.Default()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			docs, _, _ := p.ParseBytes("test.yaml", []byte(tc.manifest))
			_, findings := inventory.Extract(docs)

			found := false
			for _, f := range findings {
				if f.RuleID == tc.ruleID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected finding %s for %s, got: %v", tc.ruleID, tc.name, findings)
			}
		})
	}
}

func TestFindingAndResourceIDString(t *testing.T) {
	id := inventory.ResourceID{
		Group:     "apps",
		Version:   "v1",
		Kind:      "Deployment",
		Namespace: "prod",
		Name:      "web",
	}
	if id.String() != "Deployment.apps/prod/web" {
		t.Errorf("unexpected id.String(): %s", id.String())
	}

	finding := inventory.Finding{
		RuleID:   inventory.RuleMissingAPIVersion,
		Severity: "error",
		Message:  "missing apiVersion",
	}
	if !strings.Contains(finding.String(), "K8S-GVK-002") {
		t.Errorf("unexpected finding.String(): %s", finding.String())
	}
}
