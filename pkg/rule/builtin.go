package rule

import (
	"fmt"
	"strings"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/nir"
)

// Standard diagnostic rule IDs per STATIC_ANALYSIS_RULES.md.
const (
	RuleContainerImageTag      = "WL-CONTAINER-001"
	RuleResourceLimitsRequests = "WL-RES-001"
	RuleContainerProbes        = "WL-PROBE-001"
	RulePrivilegedContainer    = "SEC-CTX-001"
	RulePrivilegeEscalation    = "SEC-CTX-002"
	RuleServiceMissingSelector = "NET-SVC-001"
)

// DefaultRules returns all standard built-in rules for the diagnostic engine.
func DefaultRules() []*Rule {
	return []*Rule{
		ruleContainerImageTag(),
		ruleResourceLimitsRequests(),
		ruleContainerProbes(),
		rulePrivilegedContainer(),
		rulePrivilegeEscalation(),
		ruleServiceMissingSelector(),
	}
}

// 1. WL-CONTAINER-001: Image tag is missing or set to :latest
func ruleContainerImageTag() *Rule {
	return &Rule{
		ID:            RuleContainerImageTag,
		Title:         "Container image unpinned or using latest tag",
		Description:   "Images should use an immutable digest or pinned semantic version tag rather than :latest or no tag",
		Category:      CategoryWorkload,
		Severity:      diagnostic.SeverityWarning,
		EvidenceClass: diagnostic.EvidenceProbable,
		ApplicableTo:  WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			var findings []diagnostic.Finding
			containers := ExtractContainers(res)

			for _, c := range containers {
				img := strings.TrimSpace(c.Image)
				if img == "" {
					continue // schema missing required image check handles this
				}

				// Digest references (e.g. repo@sha256:...) are pinned and safe
				if strings.Contains(img, "@sha256:") {
					continue
				}

				lastColon := strings.LastIndex(img, ":")
				lastSlash := strings.LastIndex(img, "/")

				isUnpinned := false
				if lastColon == -1 || lastColon < lastSlash {
					// No tag specified (e.g. "nginx", "myregistry.io/app")
					isUnpinned = true
				} else {
					tag := img[lastColon+1:]
					if tag == "latest" {
						isUnpinned = true
					}
				}

				if isUnpinned {
					imgNode := c.ContainerNode.Get("image")
					loc := c.ContainerNode.Location
					if imgNode != nil {
						loc = imgNode.Location
					}
					findings = append(findings, diagnostic.Finding{
						RuleID:        RuleContainerImageTag,
						Severity:      diagnostic.SeverityWarning,
						Message:       fmt.Sprintf("Container %q uses unpinned or :latest image %q; pin to a specific version or digest", c.Name, c.Image),
						Location:      loc,
						EvidenceClass: diagnostic.EvidenceProbable,
						Details:       "Unpinned image tags lead to non-reproducible deployments and unexpected upgrades in production.",
					})
				}
			}

			return findings
		},
	}
}

// 2. WL-RES-001: Container missing resource limits or requests
func ruleResourceLimitsRequests() *Rule {
	return &Rule{
		ID:            RuleResourceLimitsRequests,
		Title:         "Container missing resource requests or limits",
		Description:   "Containers should specify CPU and memory requests and limits to guarantee predictable scheduling and prevent noisy neighbors",
		Category:      CategoryWorkload,
		Severity:      diagnostic.SeverityWarning,
		EvidenceClass: diagnostic.EvidenceProbable,
		ApplicableTo:  WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			var findings []diagnostic.Finding
			containers := ExtractContainers(res)

			for _, c := range containers {
				resNode := c.ContainerNode.Get("resources")
				hasRequests := false
				hasLimits := false

				if resNode != nil && resNode.Kind == nir.KindMapping {
					if reqNode := resNode.Get("requests"); reqNode != nil && len(reqNode.Fields) > 0 {
						hasRequests = true
					}
					if limNode := resNode.Get("limits"); limNode != nil && len(limNode.Fields) > 0 {
						hasLimits = true
					}
				}

				if !hasRequests || !hasLimits {
					missing := "requests and limits"
					if hasRequests && !hasLimits {
						missing = "limits"
					} else if !hasRequests && hasLimits {
						missing = "requests"
					}

					loc := c.ContainerNode.Location
					if resNode != nil {
						loc = resNode.Location
					}

					findings = append(findings, diagnostic.Finding{
						RuleID:        RuleResourceLimitsRequests,
						Severity:      diagnostic.SeverityWarning,
						Message:       fmt.Sprintf("Container %q is missing resource %s", c.Name, missing),
						Location:      loc,
						EvidenceClass: diagnostic.EvidenceProbable,
						Details:       "Resource limits and requests are required to enable predictable QoS and fair node sharing.",
					})
				}
			}

			return findings
		},
	}
}

// 3. WL-PROBE-001: Container missing probes in long-running services
func ruleContainerProbes() *Rule {
	return &Rule{
		ID:            RuleContainerProbes,
		Title:         "Container missing liveness or readiness probe",
		Description:   "Long-running workloads should define readiness and liveness probes to ensure traffic isolation and self-healing",
		Category:      CategoryWorkload,
		Severity:      diagnostic.SeverityInfo,
		EvidenceClass: diagnostic.EvidenceProbable,
		ApplicableTo:  []string{"Pod", "Deployment", "StatefulSet", "DaemonSet"},
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			var findings []diagnostic.Finding
			containers := ExtractContainers(res)

			for _, c := range containers {
				if c.IsInit {
					continue // init containers do not have probes
				}

				hasLiveness := c.ContainerNode.Get("livenessProbe") != nil
				hasReadiness := c.ContainerNode.Get("readinessProbe") != nil

				if !hasLiveness || !hasReadiness {
					missing := "liveness and readiness probes"
					if hasLiveness && !hasReadiness {
						missing = "readinessProbe"
					} else if !hasLiveness && hasReadiness {
						missing = "livenessProbe"
					}

					findings = append(findings, diagnostic.Finding{
						RuleID:        RuleContainerProbes,
						Severity:      diagnostic.SeverityInfo,
						Message:       fmt.Sprintf("Container %q is missing %s", c.Name, missing),
						Location:      c.ContainerNode.Location,
						EvidenceClass: diagnostic.EvidenceProbable,
						Details:       "Readiness probes prevent unhealthy pods from receiving traffic; liveness probes trigger restarts on deadlocks.",
					})
				}
			}

			return findings
		},
	}
}

// 4. SEC-CTX-001: Privileged container execution
func rulePrivilegedContainer() *Rule {
	return &Rule{
		ID:            RulePrivilegedContainer,
		Title:         "Privileged container execution prohibited",
		Description:   "Containers must not run in privileged mode as it grants full root access to the host node",
		Category:      CategorySecurity,
		Severity:      diagnostic.SeverityError,
		EvidenceClass: diagnostic.EvidenceDeterministic,
		ApplicableTo:  WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			var findings []diagnostic.Finding
			containers := ExtractContainers(res)

			for _, c := range containers {
				privNode := c.ContainerNode.Get("securityContext.privileged")
				if privNode != nil && privNode.Kind == nir.KindScalar && privNode.Value == "true" {
					findings = append(findings, diagnostic.Finding{
						RuleID:        RulePrivilegedContainer,
						Severity:      diagnostic.SeverityError,
						Message:       fmt.Sprintf("Container %q runs in privileged mode; this compromises host node security", c.Name),
						Location:      privNode.Location,
						EvidenceClass: diagnostic.EvidenceDeterministic,
						Details:       "Running containers with privileged: true bypasses container isolation boundaries.",
					})
				}
			}

			return findings
		},
	}
}

// 5. SEC-CTX-002: Allow privilege escalation enabled
func rulePrivilegeEscalation() *Rule {
	return &Rule{
		ID:            RulePrivilegeEscalation,
		Title:         "Privilege escalation allowed in container",
		Description:   "allowPrivilegeEscalation should be explicitly set to false to prevent setuid binaries from escalating permissions",
		Category:      CategorySecurity,
		Severity:      diagnostic.SeverityWarning,
		EvidenceClass: diagnostic.EvidenceDeterministic,
		ApplicableTo:  WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			var findings []diagnostic.Finding
			containers := ExtractContainers(res)

			for _, c := range containers {
				escNode := c.ContainerNode.Get("securityContext.allowPrivilegeEscalation")
				if escNode != nil && escNode.Kind == nir.KindScalar && escNode.Value == "true" {
					findings = append(findings, diagnostic.Finding{
						RuleID:        RulePrivilegeEscalation,
						Severity:      diagnostic.SeverityWarning,
						Message:       fmt.Sprintf("Container %q explicitly allows privilege escalation", c.Name),
						Location:      escNode.Location,
						EvidenceClass: diagnostic.EvidenceDeterministic,
						Details:       "Set allowPrivilegeEscalation: false to prevent processes from gaining more privileges than their parent.",
					})
				}
			}

			return findings
		},
	}
}

// 6. NET-SVC-001: Service missing selector
func ruleServiceMissingSelector() *Rule {
	return &Rule{
		ID:            RuleServiceMissingSelector,
		Title:         "Service missing pod selector",
		Description:   "Services of type ClusterIP, NodePort, or LoadBalancer should specify a selector unless intentionally managing endpoints manually",
		Category:      CategoryNetwork,
		Severity:      diagnostic.SeverityWarning,
		EvidenceClass: diagnostic.EvidenceProbable,
		ApplicableTo:  []string{"Service"},
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			if res.RootNode == nil {
				return nil
			}

			svcType := "ClusterIP"
			if tNode := res.RootNode.Get("spec.type"); tNode != nil && tNode.Kind == nir.KindScalar {
				svcType = tNode.Value
			}

			if svcType == "ExternalName" {
				return nil
			}

			clusterIP := ""
			if cipNode := res.RootNode.Get("spec.clusterIP"); cipNode != nil && cipNode.Kind == nir.KindScalar {
				clusterIP = cipNode.Value
			}
			if clusterIP == "None" {
				// Headless service; could be manual endpoints
				return nil
			}

			selNode := res.RootNode.Get("spec.selector")
			if selNode == nil || len(selNode.Fields) == 0 {
				loc := res.Location
				if specNode := res.RootNode.Get("spec"); specNode != nil {
					loc = specNode.Location
				}
				return []diagnostic.Finding{
					{
						RuleID:        RuleServiceMissingSelector,
						Severity:      diagnostic.SeverityWarning,
						Message:       fmt.Sprintf("Service %q (%s) has no selector specified; endpoints must be maintained manually", res.ID.Name, svcType),
						Location:      loc,
						EvidenceClass: diagnostic.EvidenceProbable,
						Details:       "Services without selectors do not automatically route traffic to backend pods.",
					},
				}
			}

			return nil
		},
	}
}
