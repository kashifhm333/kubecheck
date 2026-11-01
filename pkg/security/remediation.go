package security

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrUnsafeRemediation is returned when a remediation violates safety requirements per RCA-REMED-REQ-053.
	ErrUnsafeRemediation = errors.New("unsafe remediation rejected by safety governor")
)

// Remediation provides structured, verifiable guidance to resolve a diagnostic finding per RCA-REMED-REQ-001.
type Remediation struct {
	Action      string   `json:"action"`
	Recommended string   `json:"recommended"`
	Verify      string   `json:"verify"`
	References  []string `json:"references,omitempty"`
}

// RemediationGovernor checks remediations against safety invariants per RCA-REMED-REQ-052 and RCA-REMED-REQ-053.
type RemediationGovernor struct {
	secretValues []string
}

// NewRemediationGovernor creates a governor with optional secret values to ensure redaction.
func NewRemediationGovernor(secretValues []string) *RemediationGovernor {
	return &RemediationGovernor{secretValues: secretValues}
}

// Validate checks a remediation against all prohibited unsafe patterns.
func (g *RemediationGovernor) Validate(r *Remediation) error {
	if r == nil {
		return errors.New("nil remediation")
	}

	combined := strings.ToLower(fmt.Sprintf("%s %s %s", r.Action, r.Recommended, r.Verify))

	// RCA-REMED-REQ-053 invariant 1: Never recommend storing a credential in a ConfigMap
	if (strings.Contains(combined, "credential") || strings.Contains(combined, "password") || strings.Contains(combined, "secret") || strings.Contains(combined, "token")) &&
		strings.Contains(combined, "configmap") {
		return fmt.Errorf("%w: recommends storing credential in ConfigMap", ErrUnsafeRemediation)
	}

	// RCA-REMED-REQ-053 invariant 2: Never recommend literal value for a credential-like field
	if strings.Contains(combined, "value:") && (strings.Contains(combined, "password") || strings.Contains(combined, "secret") || strings.Contains(combined, "token")) {
		return fmt.Errorf("%w: recommends literal value for credential", ErrUnsafeRemediation)
	}

	// RCA-REMED-REQ-053 invariant 3: Never recommend disabling readOnly: true on a Secret
	if strings.Contains(combined, "readonly: false") || strings.Contains(combined, "disable readonly") {
		return fmt.Errorf("%w: recommends disabling readOnly on Secret", ErrUnsafeRemediation)
	}

	// RCA-REMED-REQ-053 invariant 4: Never recommend stringData for a workflow reading the object back
	if strings.Contains(combined, "use stringdata") && strings.Contains(combined, "read back") {
		return fmt.Errorf("%w: recommends stringData for read-back workflow", ErrUnsafeRemediation)
	}

	// RCA-REMED-REQ-053 invariant 5: Never recommend privileged: true
	if strings.Contains(combined, "privileged: true") || strings.Contains(combined, "enable privileged") {
		return fmt.Errorf("%w: recommends privileged: true", ErrUnsafeRemediation)
	}

	// RCA-REMED-REQ-053 invariant 6: Never recommend allowPrivilegeEscalation: true
	if strings.Contains(combined, "allowprivilegeescalation: true") || strings.Contains(combined, "enable privilege escalation") {
		return fmt.Errorf("%w: recommends allowPrivilegeEscalation: true", ErrUnsafeRemediation)
	}

	// RCA-REMED-REQ-053 invariant 7: Never recommend removing NetworkPolicy
	if strings.Contains(combined, "remove networkpolicy") || strings.Contains(combined, "delete networkpolicy") {
		return fmt.Errorf("%w: recommends removing NetworkPolicy", ErrUnsafeRemediation)
	}

	// RCA-REMED-REQ-053 invariant 8: Never recommend --insecure flags
	if strings.Contains(combined, "--insecure") || strings.Contains(combined, "insecure-skip-tls-verify") {
		return fmt.Errorf("%w: recommends --insecure flag", ErrUnsafeRemediation)
	}

	// RCA-REMED-REQ-052: Scan remediation text for secret-bearing field content
	for _, secret := range g.secretValues {
		secret = strings.TrimSpace(secret)
		if len(secret) > 3 && strings.Contains(combined, strings.ToLower(secret)) {
			return fmt.Errorf("%w: remediation leaks raw secret value", ErrUnsafeRemediation)
		}
	}

	return nil
}
