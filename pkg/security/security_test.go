package security_test

import (
	"strings"
	"testing"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/parser"
	"github.com/kubecheck/kubecheck/pkg/rule"
	"github.com/kubecheck/kubecheck/pkg/security"
)

func parseAndAdmit(t *testing.T, yamlStr string) (*inventory.Inventory, *inventory.Resource) {
	t.Helper()
	p := parser.Default()
	docs, findings, err := p.ParseBytes("test.yaml", []byte(yamlStr))
	if err != nil {
		t.Fatalf("unexpected parse error: %v (findings: %+v)", err, findings)
	}
	inv, invFindings := inventory.Extract(docs)
	for _, f := range invFindings {
		if f.Severity == "error" {
			t.Fatalf("unexpected inventory error: %+v", f)
		}
	}
	if len(inv.Resources) == 0 {
		t.Fatalf("no admitted resources")
	}
	return inv, inv.Resources[0]
}

func evaluateSecurityRules(t *testing.T, res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
	t.Helper()
	engine := rule.NewEngine(security.Rules())
	return engine.Evaluate(inv)
}

func hasRuleFinding(findings []diagnostic.Finding, ruleID string) bool {
	for _, f := range findings {
		if f.RuleID == ruleID {
			return true
		}
	}
	return false
}

func TestHostNamespacesAndPathMounts(t *testing.T) {
	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: host-access-pod
spec:
  hostNetwork: true
  hostPID: true
  hostIPC: true
  volumes:
  - name: root-vol
    hostPath:
      path: /etc
  - name: socket-vol
    hostPath:
      path: /var/run/docker.sock
  - name: kubelet-vol
    hostPath:
      path: /var/lib/kubelet/pods
  - name: data-vol
    hostPath:
      path: /mnt/data
  containers:
  - name: test-c
    image: test:1.0
`
	inv, res := parseAndAdmit(t, manifest)
	findings := evaluateSecurityRules(t, res, inv)

	if !hasRuleFinding(findings, security.RuleHostNetwork) {
		t.Errorf("expected %s for hostNetwork", security.RuleHostNetwork)
	}
	if !hasRuleFinding(findings, security.RuleHostPID) {
		t.Errorf("expected %s for hostPID", security.RuleHostPID)
	}
	if !hasRuleFinding(findings, security.RuleHostIPC) {
		t.Errorf("expected %s for hostIPC", security.RuleHostIPC)
	}
	if !hasRuleFinding(findings, security.RuleHostPathRootOrEtc) {
		t.Errorf("expected %s for /etc hostPath", security.RuleHostPathRootOrEtc)
	}
	if !hasRuleFinding(findings, security.RuleHostPathRuntimeSocket) {
		t.Errorf("expected %s for runtime socket hostPath", security.RuleHostPathRuntimeSocket)
	}
	if !hasRuleFinding(findings, security.RuleHostPathKubelet) {
		t.Errorf("expected %s for kubelet hostPath", security.RuleHostPathKubelet)
	}
	if !hasRuleFinding(findings, security.RuleHostPathGeneral) {
		t.Errorf("expected %s for general hostPath", security.RuleHostPathGeneral)
	}
}

func TestCapabilitiesAndRootExecution(t *testing.T) {
	// 1. CAP_SYS_ADMIN and drop != ALL
	manifestCaps := `apiVersion: v1
kind: Pod
metadata:
  name: caps-pod
spec:
  containers:
  - name: c1
    image: test:1.0
    securityContext:
      capabilities:
        add: ["CAP_SYS_ADMIN", "NET_BIND_SERVICE"]
        drop: ["KILL"]
`
	inv, res := parseAndAdmit(t, manifestCaps)
	findings := evaluateSecurityRules(t, res, inv)
	if !hasRuleFinding(findings, security.RuleCapSysAdmin) {
		t.Errorf("expected %s for CAP_SYS_ADMIN", security.RuleCapSysAdmin)
	}
	if !hasRuleFinding(findings, security.RuleCapDropNotAll) {
		t.Errorf("expected %s for drop not ALL", security.RuleCapDropNotAll)
	}

	// 2. runAsNonRoot with runAsUser: 0 contradiction
	manifestConflict := `apiVersion: v1
kind: Pod
metadata:
  name: conflict-pod
spec:
  securityContext:
    runAsNonRoot: true
  containers:
  - name: root-c
    image: test:1.0
    securityContext:
      runAsUser: 0
`
	inv2, res2 := parseAndAdmit(t, manifestConflict)
	findings2 := evaluateSecurityRules(t, res2, inv2)
	if !hasRuleFinding(findings2, security.RuleRunAsNonRootWithZero) {
		t.Errorf("expected %s for runAsNonRoot + runAsUser 0 contradiction", security.RuleRunAsNonRootWithZero)
	}

	// 3. runAsUser: 0 without non-root
	manifestRootUser := `apiVersion: v1
kind: Pod
metadata:
  name: root-user-pod
spec:
  containers:
  - name: root-c
    image: test:1.0
    securityContext:
      runAsUser: 0
`
	inv3, res3 := parseAndAdmit(t, manifestRootUser)
	findings3 := evaluateSecurityRules(t, res3, inv3)
	if !hasRuleFinding(findings3, security.RuleRunAsRootUser) {
		t.Errorf("expected %s for runAsUser: 0", security.RuleRunAsRootUser)
	}
}

func TestSeccompAndReadOnlyRootFS(t *testing.T) {
	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: unconfined-pod
spec:
  securityContext:
    seccompProfile:
      type: Unconfined
  containers:
  - name: c1
    image: test:1.0
`
	inv, res := parseAndAdmit(t, manifest)
	findings := evaluateSecurityRules(t, res, inv)

	if !hasRuleFinding(findings, security.RuleSeccompUnconfined) {
		t.Errorf("expected %s for unconfined seccomp", security.RuleSeccompUnconfined)
	}
	if !hasRuleFinding(findings, security.RuleReadOnlyRootFSUnset) {
		t.Errorf("expected %s for unset readOnlyRootFilesystem", security.RuleReadOnlyRootFSUnset)
	}
}

func TestRBACWildcardsAndDangerousBindings(t *testing.T) {
	// 1. Wildcard Role
	manifestRole := `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: wildcard-role
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]
`
	inv, res := parseAndAdmit(t, manifestRole)
	findings := evaluateSecurityRules(t, res, inv)
	if !hasRuleFinding(findings, security.RuleRBACWildcardAPIGroups) {
		t.Errorf("expected %s for wildcard apiGroups", security.RuleRBACWildcardAPIGroups)
	}
	if !hasRuleFinding(findings, security.RuleRBACWildcardResources) {
		t.Errorf("expected %s for wildcard resources", security.RuleRBACWildcardResources)
	}
	if !hasRuleFinding(findings, security.RuleRBACWildcardVerbs) {
		t.Errorf("expected %s for wildcard verbs", security.RuleRBACWildcardVerbs)
	}

	// 2. Binding to system:masters and cluster-admin to ServiceAccount
	manifestBinding := `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: dangerous-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: Group
  name: system:masters
  apiGroup: rbac.authorization.k8s.io
- kind: ServiceAccount
  name: app-runner
  namespace: default
`
	inv2, res2 := parseAndAdmit(t, manifestBinding)
	findings2 := evaluateSecurityRules(t, res2, inv2)
	if !hasRuleFinding(findings2, security.RuleRBACSystemMasters) {
		t.Errorf("expected %s for system:masters", security.RuleRBACSystemMasters)
	}
	if !hasRuleFinding(findings2, security.RuleRBACClusterAdminSA) {
		t.Errorf("expected %s for cluster-admin to ServiceAccount", security.RuleRBACClusterAdminSA)
	}
}

func TestSecretReferencesAndLiteralEnv(t *testing.T) {
	manifest := `apiVersion: v1
kind: Pod
metadata:
  name: secret-pod
spec:
  containers:
  - name: c1
    image: test:1.0
    env:
    - name: DB_PASSWORD
      value: "supersecret123"
    - name: FROM_SECRET
      valueFrom:
        secretKeyRef:
          name: app-secret
          key: password
          namespace: other-ns
    envFrom:
    - secretRef:
        name: extra-secret
        namespace: invalid-ns
`
	inv, res := parseAndAdmit(t, manifest)
	findings := evaluateSecurityRules(t, res, inv)

	if !hasRuleFinding(findings, security.RuleLiteralSecretEnv) {
		t.Errorf("expected %s for literal DB_PASSWORD", security.RuleLiteralSecretEnv)
	}
	if !hasRuleFinding(findings, security.RuleSecretRefWithNamespace) {
		t.Errorf("expected %s for secretRef namespace", security.RuleSecretRefWithNamespace)
	}
}

func TestPSSEvaluator(t *testing.T) {
	evaluator := security.NewPSSEvaluator(security.ProfileRestricted)

	// 1. Privileged profile should return no findings
	manifestPriv := `apiVersion: v1
kind: Pod
metadata:
  name: pss-pod
spec:
  hostNetwork: true
  containers:
  - name: c1
    image: test:1.0
    securityContext:
      privileged: true
`
	invPriv, resPriv := parseAndAdmit(t, manifestPriv)
	findingsPriv := evaluator.Evaluate(resPriv, security.ProfilePrivileged)
	if len(findingsPriv) != 0 {
		t.Errorf("expected 0 findings for privileged profile, got %d", len(findingsPriv))
	}

	// 2. Baseline profile on violating pod
	findingsBaseline := evaluator.Evaluate(resPriv, security.ProfileBaseline)
	if len(findingsBaseline) == 0 {
		t.Errorf("expected PSS baseline violations for hostNetwork and privileged")
	}

	// 3. Restricted profile checks
	findingsRestricted := evaluator.Evaluate(resPriv, security.ProfileRestricted)
	if len(findingsRestricted) == 0 {
		t.Errorf("expected PSS restricted violations")
	}

	// 4. Clean Restricted workload
	manifestClean := `apiVersion: v1
kind: Pod
metadata:
  name: clean-pod
spec:
  securityContext:
    runAsNonRoot: true
    seccompProfile:
      type: RuntimeDefault
  containers:
  - name: app
    image: nginx:1.25.3
    securityContext:
      allowPrivilegeEscalation: false
      runAsUser: 10001
      capabilities:
        drop: ["ALL"]
        add: ["NET_BIND_SERVICE"]
`
	invClean, resClean := parseAndAdmit(t, manifestClean)
	findingsClean := evaluator.Evaluate(resClean, security.ProfileRestricted)
	if len(findingsClean) != 0 {
		t.Errorf("expected 0 PSS violations for clean restricted pod, got: %+v", findingsClean)
	}

	// 5. Non-workload resource (ConfigMap) returns nil
	manifestCM := `apiVersion: v1
kind: ConfigMap
metadata:
  name: cm
data:
  k: v
`
	invCM, resCM := parseAndAdmit(t, manifestCM)
	if res := evaluator.Evaluate(resCM, security.ProfileRestricted); res != nil {
		t.Errorf("expected nil PSS evaluation for ConfigMap, got %+v", res)
	}
	_ = invPriv
	_ = invClean
	_ = invCM
}

func TestRemediationGovernor(t *testing.T) {
	secrets := []string{"realSecretTokenXYZ987", "databaseP@ssw0rd!"}
	gov := security.NewRemediationGovernor(secrets)

	// 1. Valid remediation
	validRem := &security.Remediation{
		Action:      "Store credential in Secret",
		Recommended: "Use valueFrom.secretKeyRef referencing Secret 'app-creds' key 'token'",
		Verify:      "Re-run kubecheck analyze to confirm resolution",
	}
	if err := gov.Validate(validRem); err != nil {
		t.Errorf("expected valid remediation to pass, got: %v", err)
	}

	// 2. Reject storing credential in ConfigMap
	badCM := &security.Remediation{
		Action:      "Store password",
		Recommended: "Store the password in a ConfigMap data entry",
		Verify:      "Check configmap",
	}
	if err := gov.Validate(badCM); err == nil || !strings.Contains(err.Error(), "recommends storing credential in ConfigMap") {
		t.Errorf("expected rejection for credential in ConfigMap, got: %v", err)
	}

	// 3. Reject literal value for credential
	badVal := &security.Remediation{
		Action:      "Set password",
		Recommended: "Set value: mysecretpassword in env",
		Verify:      "Check env",
	}
	if err := gov.Validate(badVal); err == nil || !strings.Contains(err.Error(), "recommends literal value for credential") {
		t.Errorf("expected rejection for literal value, got: %v", err)
	}

	// 4. Reject disabling readOnly
	badRO := &security.Remediation{
		Action:      "Mount Secret",
		Recommended: "Set readOnly: false on Secret volumeMount",
		Verify:      "Check mount",
	}
	if err := gov.Validate(badRO); err == nil || !strings.Contains(err.Error(), "recommends disabling readOnly") {
		t.Errorf("expected rejection for disabling readOnly, got: %v", err)
	}

	// 5. Reject privileged: true
	badPriv := &security.Remediation{
		Action:      "Fix permissions",
		Recommended: "Enable privileged: true on container",
		Verify:      "Check container",
	}
	if err := gov.Validate(badPriv); err == nil || !strings.Contains(err.Error(), "recommends privileged: true") {
		t.Errorf("expected rejection for privileged: true, got: %v", err)
	}

	// 6. Reject allowPrivilegeEscalation: true
	badEsc := &security.Remediation{
		Action:      "Fix binary error",
		Recommended: "Set allowPrivilegeEscalation: true",
		Verify:      "Check container",
	}
	if err := gov.Validate(badEsc); err == nil || !strings.Contains(err.Error(), "recommends allowPrivilegeEscalation: true") {
		t.Errorf("expected rejection for allowPrivilegeEscalation: true, got: %v", err)
	}

	// 7. Reject removing NetworkPolicy
	badNP := &security.Remediation{
		Action:      "Fix connectivity",
		Recommended: "Delete NetworkPolicy to allow ingress traffic",
		Verify:      "Check traffic",
	}
	if err := gov.Validate(badNP); err == nil || !strings.Contains(err.Error(), "recommends removing NetworkPolicy") {
		t.Errorf("expected rejection for removing NetworkPolicy, got: %v", err)
	}

	// 8. Reject --insecure
	badInsecure := &security.Remediation{
		Action:      "Ignore TLS errors",
		Recommended: "Pass --insecure or insecure-skip-tls-verify",
		Verify:      "Check flag",
	}
	if err := gov.Validate(badInsecure); err == nil || !strings.Contains(err.Error(), "recommends --insecure flag") {
		t.Errorf("expected rejection for --insecure, got: %v", err)
	}

	// 9. Reject leaking raw secret value
	badLeak := &security.Remediation{
		Action:      "Set password",
		Recommended: "The secret value is realSecretTokenXYZ987 which should be rotated",
		Verify:      "Check token",
	}
	if err := gov.Validate(badLeak); err == nil || !strings.Contains(err.Error(), "leaks raw secret value") {
		t.Errorf("expected rejection for secret leakage, got: %v", err)
	}

	// 10. Nil remediation
	if err := gov.Validate(nil); err == nil {
		t.Errorf("expected error for nil remediation")
	}
}
