package security

import (
	"fmt"
	"strings"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/nir"
	"github.com/kubecheck/kubecheck/pkg/rule"
)

// PSSProfile defines Kubernetes Pod Security Standard profiles per SECURITY_AND_RBAC.md §5.
type PSSProfile string

const (
	ProfilePrivileged PSSProfile = "privileged"
	ProfileBaseline   PSSProfile = "baseline"
	ProfileRestricted PSSProfile = "restricted"
)

// Rule ID for Pod Security Standards findings
const (
	RulePodSecurityViolation  = "SEC-PSS-001"
	RuleNamespacePSSUnchecked = "SEC-PSS-002"
)

var baselineAllowedCapabilities = map[string]bool{
	"AUDIT_WRITE":      true,
	"CHOWN":            true,
	"DAC_OVERRIDE":     true,
	"FOWNER":           true,
	"FSETID":           true,
	"KILL":             true,
	"MKNOD":            true,
	"NET_BIND_SERVICE": true,
	"SETFCAP":          true,
	"SETGID":           true,
	"SETPCAP":          true,
	"SETUID":           true,
	"SYS_CHROOT":       true,
}

var restrictedAllowedVolumes = map[string]bool{
	"configMap":             true,
	"csi":                   true,
	"downwardAPI":           true,
	"emptyDir":              true,
	"ephemeral":             true,
	"persistentVolumeClaim": true,
	"projected":             true,
	"secret":                true,
}

// PSSEvaluator evaluates workload resources against a target Pod Security Standards profile.
type PSSEvaluator struct {
	defaultProfile PSSProfile
}

// NewPSSEvaluator creates a new PSS evaluator with a default profile.
func NewPSSEvaluator(defaultProfile PSSProfile) *PSSEvaluator {
	return &PSSEvaluator{defaultProfile: defaultProfile}
}

// Evaluate evaluates a resource against the target profile.
// Per K8S-SEC-REQ-011 and CONF-LIMITS-REQ-034, PSS findings use severity 'info' and evidence
// class 'context-dependent', explicitly stating that admission depends on cluster PSA configuration.
func (e *PSSEvaluator) Evaluate(res *inventory.Resource, profile PSSProfile) []diagnostic.Finding {
	if res == nil {
		return nil
	}

	// Normalizing profile
	normalizedProfile := PSSProfile(strings.ToLower(string(profile)))
	if normalizedProfile == "" {
		normalizedProfile = e.defaultProfile
	}
	if normalizedProfile == "" || normalizedProfile == ProfilePrivileged {
		return nil // Privileged allows everything
	}

	podSpec := rule.ExtractPodSpec(res)
	if podSpec == nil {
		return nil
	}

	var findings []diagnostic.Finding

	// Helper to add finding
	addViolation := func(control string, loc nir.Location, details string) {
		findings = append(findings, diagnostic.Finding{
			RuleID:        RulePodSecurityViolation,
			Severity:      diagnostic.SeverityInfo,
			Message:       fmt.Sprintf("%s %q violates %s Pod Security Standard (%s); admission outcome depends on cluster PSA configuration", res.ID.Kind, res.ID.Name, normalizedProfile, control),
			Location:      loc,
			EvidenceClass: diagnostic.EvidenceContextDependent,
			Details:       details,
		})
	}

	// 1. Check Baseline controls
	// Host namespaces
	if hostNet := podSpec.Get("hostNetwork"); hostNet != nil && hostNet.Value == "true" {
		addViolation("Host Namespaces: hostNetwork", hostNet.Location, "hostNetwork must not be set to true under baseline or restricted profiles.")
	}
	if hostPID := podSpec.Get("hostPID"); hostPID != nil && hostPID.Value == "true" {
		addViolation("Host Namespaces: hostPID", hostPID.Location, "hostPID must not be set to true under baseline or restricted profiles.")
	}
	if hostIPC := podSpec.Get("hostIPC"); hostIPC != nil && hostIPC.Value == "true" {
		addViolation("Host Namespaces: hostIPC", hostIPC.Location, "hostIPC must not be set to true under baseline or restricted profiles.")
	}

	// Volumes (hostPath disallowed in Baseline & Restricted)
	if volumesNode := podSpec.Get("volumes"); volumesNode != nil && volumesNode.Kind == nir.KindSequence {
		for _, vol := range volumesNode.Items {
			if vol.Kind != nir.KindMapping {
				continue
			}
			volName, _ := vol.GetString("name")
			if hostPath := vol.Get("hostPath"); hostPath != nil {
				addViolation("Volumes: hostPath", hostPath.Location, fmt.Sprintf("Volume %q specifies hostPath which is disallowed under %s profile.", volName, normalizedProfile))
			}

			// In restricted profile, only specific volume types are permitted
			if normalizedProfile == ProfileRestricted {
				for _, entry := range vol.Fields {
					k := entry.Key.Value
					if k == "name" {
						continue
					}
					if !restrictedAllowedVolumes[k] {
						addViolation(fmt.Sprintf("Volumes: %s", k), entry.Key.Location, fmt.Sprintf("Volume %q uses volume type %q disallowed under restricted profile.", volName, k))
					}
				}
			}
		}
	}

	containers := rule.ExtractContainers(res)
	for _, c := range containers {
		cNode := c.ContainerNode

		// Baseline: Privileged container disallowed
		if priv := cNode.Get("securityContext.privileged"); priv != nil && priv.Value == "true" {
			addViolation("Privileged Containers", priv.Location, fmt.Sprintf("Container %q runs as privileged, which is disallowed under %s profile.", c.Name, normalizedProfile))
		}

		// Baseline: Capabilities add must only contain allowed capabilities
		if capsAdd := cNode.Get("securityContext.capabilities.add"); capsAdd != nil && capsAdd.Kind == nir.KindSequence {
			for _, item := range capsAdd.Items {
				capName := strings.ToUpper(strings.TrimPrefix(item.Value, "CAP_"))
				if !baselineAllowedCapabilities[capName] {
					addViolation(fmt.Sprintf("Capabilities: %s", item.Value), item.Location, fmt.Sprintf("Container %q adds non-baseline capability %q.", c.Name, item.Value))
				}
			}
		}

		// Restricted-only checks
		if normalizedProfile == ProfileRestricted {
			// Require allowPrivilegeEscalation: false
			esc := cNode.Get("securityContext.allowPrivilegeEscalation")
			if esc == nil || esc.Value != "false" {
				loc := cNode.Location
				if esc != nil {
					loc = esc.Location
				}
				addViolation("Privilege Escalation", loc, fmt.Sprintf("Container %q must explicitly set allowPrivilegeEscalation: false under restricted profile.", c.Name))
			}

			// Require runAsNonRoot: true or non-zero runAsUser
			runAsNonRoot := cNode.Get("securityContext.runAsNonRoot")
			podRunAsNonRoot := podSpec.Get("securityContext.runAsNonRoot")
			hasNonRoot := (runAsNonRoot != nil && runAsNonRoot.Value == "true") || (podRunAsNonRoot != nil && podRunAsNonRoot.Value == "true")

			runAsUser := cNode.Get("securityContext.runAsUser")
			podRunAsUser := podSpec.Get("securityContext.runAsUser")
			hasNonZeroUser := (runAsUser != nil && runAsUser.Value != "0" && runAsUser.Value != "") || (podRunAsUser != nil && podRunAsUser.Value != "0" && podRunAsUser.Value != "")

			if !hasNonRoot && !hasNonZeroUser {
				addViolation("Running as Non-Root", cNode.Location, fmt.Sprintf("Container %q must set runAsNonRoot: true or a non-zero runAsUser under restricted profile.", c.Name))
			}

			// Require dropping ALL capabilities
			capsDrop := cNode.Get("securityContext.capabilities.drop")
			droppedAll := false
			if capsDrop != nil && capsDrop.Kind == nir.KindSequence {
				for _, item := range capsDrop.Items {
					if strings.ToUpper(item.Value) == "ALL" {
						droppedAll = true
						break
					}
				}
			}
			if !droppedAll {
				loc := cNode.Location
				if capsDrop != nil {
					loc = capsDrop.Location
				}
				addViolation("Capabilities Drop ALL", loc, fmt.Sprintf("Container %q must include 'ALL' in securityContext.capabilities.drop under restricted profile.", c.Name))
			}

			// Require seccompProfile (RuntimeDefault or Localhost)
			seccomp := cNode.Get("securityContext.seccompProfile")
			if seccomp == nil {
				seccomp = podSpec.Get("securityContext.seccompProfile")
			}
			if seccomp == nil {
				addViolation("Seccomp Profile", cNode.Location, fmt.Sprintf("Container %q must define a seccompProfile (RuntimeDefault or Localhost) under restricted profile.", c.Name))
			} else {
				stype, _ := seccomp.GetString("type")
				if stype != "RuntimeDefault" && stype != "Localhost" {
					addViolation("Seccomp Profile Type", seccomp.Location, fmt.Sprintf("Container %q seccomp profile type %q is not permitted under restricted profile (must be RuntimeDefault or Localhost).", c.Name, stype))
				}
			}
		}
	}

	return findings
}
