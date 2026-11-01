package graph_test

import (
	"strings"
	"testing"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
	"github.com/kubecheck/kubecheck/pkg/graph"
	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/parser"
)

func parseAndBuild(t *testing.T, yamlContent string) (*graph.Graph, []diagnostic.Finding) {
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

	builder := graph.NewBuilder(inv)
	g := builder.Build()
	findings := graph.Analyze(g)
	return g, findings
}

func TestValidCrossResourceRelationships(t *testing.T) {
	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: app-cfg
  namespace: prod
data:
  color: blue
---
apiVersion: v1
kind: Secret
metadata:
  name: app-sec
  namespace: prod
data:
  key: c2VjcmV0
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
  namespace: prod
spec:
  replicas: 2
  selector:
    matchLabels:
      app: web
      tier: frontend
  template:
    metadata:
      labels:
        app: web
        tier: frontend
    spec:
      containers:
      - name: web
        image: nginx:1.25.3
        ports:
        - containerPort: 8080
          name: http
        envFrom:
        - configMapRef:
            name: app-cfg
        - secretRef:
            name: app-sec
---
apiVersion: v1
kind: Service
metadata:
  name: web-svc
  namespace: prod
spec:
  type: ClusterIP
  selector:
    app: web
  ports:
  - port: 80
    targetPort: 8080
`
	g, findings := parseAndBuild(t, manifest)

	if len(findings) > 0 {
		t.Fatalf("expected 0 findings for valid graph, got: %v", findings)
	}

	if len(g.Edges) == 0 {
		t.Fatalf("expected graph edges to be created")
	}

	// Verify edges resolved
	for _, e := range g.Edges {
		if e.Status != graph.StatusResolved {
			t.Errorf("expected edge %s to be resolved, got %s", e.ID, e.Status)
		}
	}
}

func TestWorkloadSelectorMismatch(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: broken-deploy
  namespace: default
spec:
  selector:
    matchLabels:
      app: web # mismatch with template!
  template:
    metadata:
      labels:
        app: api # template label is different!
    spec:
      containers:
      - name: web
        image: nginx:1.25
`
	_, findings := parseAndBuild(t, manifest)

	foundMismatch := false
	for _, f := range findings {
		if f.RuleID == graph.RuleWorkloadSelectorSelfMismatch {
			foundMismatch = true
			if f.Severity != diagnostic.SeverityError {
				t.Errorf("expected error severity for workload selector mismatch, got %v", f.Severity)
			}
		}
	}

	if !foundMismatch {
		t.Errorf("expected RuleWorkloadSelectorSelfMismatch (CROSS-WORKLOAD-SEL-001), got: %v", findings)
	}
}

func TestServiceSelectorNoMatch(t *testing.T) {
	manifest := `apiVersion: v1
kind: Service
metadata:
  name: orphan-svc
  namespace: default
spec:
  selector:
    app: nonexistent
  ports:
  - port: 80
`
	_, findings := parseAndBuild(t, manifest)

	foundNoMatch := false
	for _, f := range findings {
		if f.RuleID == graph.RuleServiceSelectorNoMatch {
			foundNoMatch = true
			if f.Severity != diagnostic.SeverityInfo {
				t.Errorf("expected info severity, got %v", f.Severity)
			}
			if !strings.Contains(f.Message, "target pods may already exist in the cluster") {
				t.Errorf("expected explicit cluster-existence disclaimer, got: %s", f.Message)
			}
		}
	}

	if !foundNoMatch {
		t.Errorf("expected RuleServiceSelectorNoMatch (CROSS-SVC-001), got: %v", findings)
	}
}

func TestServicePortMismatch(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
  namespace: default
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
        ports:
        - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: web-svc
  namespace: default
spec:
  selector:
    app: web
  ports:
  - port: 80
    targetPort: 9090 # mismatched! container exposes 8080
`
	_, findings := parseAndBuild(t, manifest)

	foundPortMismatch := false
	for _, f := range findings {
		if f.RuleID == graph.RuleServicePortMismatch {
			foundPortMismatch = true
			if f.Severity != diagnostic.SeverityWarning {
				t.Errorf("expected warning severity, got %v", f.Severity)
			}
			if !strings.Contains(f.Message, "does not expose a matching containerPort") {
				t.Errorf("unexpected message: %s", f.Message)
			}
		}
	}

	if !foundPortMismatch {
		t.Errorf("expected RuleServicePortMismatch (CROSS-SVC-PORT-001), got: %v", findings)
	}
}

func TestMissingConfigMapAndSecret(t *testing.T) {
	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: ref-pod
  namespace: default
spec:
  containers:
  - name: app
    image: nginx:1.25
    env:
    - name: CFG_VAL
      valueFrom:
        configMapKeyRef:
          name: missing-cfg
          key: val
    - name: OPTIONAL_VAL
      valueFrom:
        configMapKeyRef:
          name: missing-optional-cfg
          key: val
          optional: true # should NOT be flagged!
`
	_, findings := parseAndBuild(t, manifest)

	countMissing := 0
	for _, f := range findings {
		if f.RuleID == graph.RuleMissingConfigRef {
			countMissing++
			if strings.Contains(f.Message, "missing-optional-cfg") {
				t.Errorf("optional configmap should not be flagged!")
			}
			if !strings.Contains(f.Message, "missing-cfg") {
				t.Errorf("expected missing-cfg in message, got: %s", f.Message)
			}
		}
	}

	if countMissing != 1 {
		t.Errorf("expected exactly 1 missing config finding, got %d: %v", countMissing, findings)
	}
}

func TestMissingIngressAndRBAC(t *testing.T) {
	manifest := `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: test-ing
  namespace: default
spec:
  rules:
  - http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: missing-backend-svc
            port:
              number: 80
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: test-rb
  namespace: default
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: missing-role
subjects:
- kind: ServiceAccount
  name: default
`
	_, findings := parseAndBuild(t, manifest)

	foundIng := false
	foundRBAC := false

	for _, f := range findings {
		if f.RuleID == graph.RuleMissingIngressBackend {
			foundIng = true
		}
		if f.RuleID == graph.RuleMissingRBACRole {
			foundRBAC = true
		}
	}

	if !foundIng {
		t.Errorf("expected RuleMissingIngressBackend (CROSS-ING-001), got: %v", findings)
	}
	if !foundRBAC {
		t.Errorf("expected RuleMissingRBACRole (CROSS-RBAC-001), got: %v", findings)
	}
}

func TestCycleDetection(t *testing.T) {
	// A circular owner reference between two configmaps
	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-a
  namespace: default
  ownerReferences:
  - apiVersion: v1
    kind: ConfigMap
    name: cm-b
    uid: "123"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-b
  namespace: default
  ownerReferences:
  - apiVersion: v1
    kind: ConfigMap
    name: cm-a
    uid: "456"
`
	_, findings := parseAndBuild(t, manifest)

	foundCycle := false
	for _, f := range findings {
		if f.RuleID == graph.RuleCyclicRelationship {
			foundCycle = true
			if f.Severity != diagnostic.SeverityError {
				t.Errorf("expected error severity for cycle, got %v", f.Severity)
			}
			if !strings.Contains(f.Message, "Cyclic owner relationship detected") {
				t.Errorf("unexpected message: %s", f.Message)
			}
		}
	}

	if !foundCycle {
		t.Errorf("expected RuleCyclicRelationship (CROSS-CYCLE-001), got: %v", findings)
	}
}

func TestEdgeTargetHintString(t *testing.T) {
	h1 := graph.EdgeTargetHint{Kind: "Service", Namespace: "prod", Name: "web"}
	if h1.String() != "Service/prod/web" {
		t.Errorf("unexpected h1.String(): %s", h1.String())
	}

	h2 := graph.EdgeTargetHint{Group: "apps", Kind: "Deployment", Namespace: "prod", Name: "web"}
	if h2.String() != "Deployment.apps/prod/web" {
		t.Errorf("unexpected h2.String(): %s", h2.String())
	}

	h3 := graph.EdgeTargetHint{Kind: "ClusterRole", Name: "admin"}
	if h3.String() != "ClusterRole/admin" {
		t.Errorf("unexpected h3.String(): %s", h3.String())
	}

	h4 := graph.EdgeTargetHint{Kind: "Pod", Namespace: "default", Selector: map[string]string{"app": "web"}}
	if !strings.Contains(h4.String(), "matching selector") {
		t.Errorf("unexpected h4.String(): %s", h4.String())
	}

	// Nil / empty graph analyze
	if graph.Analyze(nil) != nil {
		t.Errorf("expected nil for nil graph")
	}
	b := graph.NewBuilder(nil)
	if b.Build() == nil {
		t.Errorf("expected non-nil graph from nil inventory")
	}
}

func TestVolumeAndSecretReferences(t *testing.T) {
	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: advanced-pod
  namespace: prod
spec:
  serviceAccountName: custom-sa
  volumes:
  - name: sec-vol
    secret:
      secretName: missing-vol-sec
  - name: data-pvc
    persistentVolumeClaim:
      claimName: missing-pvc
  containers:
  - name: app
    image: nginx:1.25
    env:
    - name: SECRET_PASS
      valueFrom:
        secretKeyRef:
          name: missing-env-sec
          key: password
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: tls-ing
  namespace: prod
spec:
  tls:
  - secretName: missing-tls-cert
    hosts:
    - app.example.com
`
	g, findings := parseAndBuild(t, manifest)

	foundSA := false
	foundSec := false
	foundPVC := false

	for _, f := range findings {
		if f.RuleID == graph.RuleMissingServiceAccount {
			foundSA = true
		}
		if f.RuleID == graph.RuleMissingConfigRef {
			foundSec = true
		}
	}

	if !foundSA {
		t.Errorf("expected RuleMissingServiceAccount finding, got: %v", findings)
	}
	if !foundSec {
		t.Errorf("expected RuleMissingConfigRef finding for secret, got: %v", findings)
	}

	// Verify PVC edge was created
	for _, e := range g.Edges {
		if e.Type == graph.EdgePVCRef {
			foundPVC = true
		}
	}
	if !foundPVC {
		t.Errorf("expected EdgePVCRef to be constructed")
	}
}
