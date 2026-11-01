package security

import (
	"fmt"
	"strings"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/nir"
	"github.com/kubecheck/kubecheck/pkg/rule"
)

// Normative security diagnostic rule IDs per STATIC_ANALYSIS_RULES.md §12.
const (
	RuleCapSysAdmin            = "SEC-CTX-003"
	RuleCapDropNotAll          = "SEC-CTX-004"
	RuleHostNetwork            = "SEC-CTX-005"
	RuleHostPID                = "SEC-CTX-006"
	RuleHostIPC                = "SEC-CTX-007"
	RuleHostPathRootOrEtc      = "SEC-CTX-008"
	RuleHostPathRuntimeSocket  = "SEC-CTX-009"
	RuleHostPathKubelet        = "SEC-CTX-010"
	RuleHostPathGeneral        = "SEC-CTX-011"
	RuleRunAsNonRootWithZero   = "SEC-CTX-014"
	RuleRunAsRootUser          = "SEC-CTX-016"
	RuleReadOnlyRootFSUnset    = "SEC-CTX-017"
	RuleSeccompUnconfined      = "SEC-CTX-018"
	RuleRBACWildcardVerbs      = "SEC-RBAC-004"
	RuleRBACWildcardResources  = "SEC-RBAC-005"
	RuleRBACWildcardAPIGroups  = "SEC-RBAC-006"
	RuleRBACSystemMasters      = "SEC-RBAC-012"
	RuleRBACClusterAdminSA     = "SEC-RBAC-013"
	RuleSecretRefWithNamespace = "SEC-SECRET-001"
	RuleLiteralSecretEnv       = "SEC-SECRET-006"
)

// Known container runtime socket paths per SEC-CTX-009
var runtimeSocketPaths = map[string]bool{
	"/var/run/docker.sock":                true,
	"/run/containerd/containerd.sock":     true,
	"/run/crio/crio.sock":                 true,
	"/var/run/crio/crio.sock":             true,
	"/run/k3s/containerd/containerd.sock": true,
}

func getString(n *nir.Node, path string) string {
	if n == nil {
		return ""
	}
	s, _ := n.GetString(path)
	return s
}

// Rules returns all standard security and RBAC diagnostic rules.
func Rules() []*rule.Rule {
	return []*rule.Rule{
		ruleCapSysAdmin(),
		ruleCapDropNotAll(),
		ruleHostNetwork(),
		ruleHostPID(),
		ruleHostIPC(),
		ruleHostPathMounts(),
		ruleRunAsRootConflicts(),
		ruleReadOnlyRootFS(),
		ruleSeccompProfile(),
		ruleRBACWildcards(),
		ruleRBACBindings(),
		ruleSecretReferences(),
		ruleLiteralSecretEnv(),
	}
}

// 1. SEC-CTX-003: capabilities.add includes SYS_ADMIN
func ruleCapSysAdmin() *rule.Rule {
	return &rule.Rule{
		ID:            RuleCapSysAdmin,
		Title:         "Dangerous Linux capability CAP_SYS_ADMIN added",
		Description:   "Containers must not add CAP_SYS_ADMIN as it provides near-complete root equivalent access to the host kernel",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityWarning,
		EvidenceClass: diagnostic.EvidenceDeterministic,
		ApplicableTo:  rule.WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			var findings []diagnostic.Finding
			for _, c := range rule.ExtractContainers(res) {
				capsAdd := c.ContainerNode.Get("securityContext.capabilities.add")
				if capsAdd != nil && capsAdd.Kind == nir.KindSequence {
					for _, item := range capsAdd.Items {
						val := strings.ToUpper(strings.TrimPrefix(item.Value, "CAP_"))
						if val == "SYS_ADMIN" {
							findings = append(findings, diagnostic.Finding{
								RuleID:        RuleCapSysAdmin,
								Severity:      diagnostic.SeverityWarning,
								Message:       fmt.Sprintf("Container %q grants dangerous capability %q (CAP_SYS_ADMIN)", c.Name, item.Value),
								Location:      item.Location,
								EvidenceClass: diagnostic.EvidenceDeterministic,
								Details:       "CAP_SYS_ADMIN includes a vast range of privileged kernel operations and allows container breakouts.",
							})
						}
					}
				}
			}
			return findings
		},
	}
}

// 2. SEC-CTX-004: capabilities.drop does not drop ALL
func ruleCapDropNotAll() *rule.Rule {
	return &rule.Rule{
		ID:            RuleCapDropNotAll,
		Title:         "Capabilities drop absent or does not include ALL",
		Description:   "Containers should drop ALL capabilities and only add back necessary privileges",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityInfo,
		EvidenceClass: diagnostic.EvidenceContextDependent,
		ApplicableTo:  rule.WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			var findings []diagnostic.Finding
			for _, c := range rule.ExtractContainers(res) {
				capsDrop := c.ContainerNode.Get("securityContext.capabilities.drop")
				hasDropAll := false
				if capsDrop != nil && capsDrop.Kind == nir.KindSequence {
					for _, item := range capsDrop.Items {
						if strings.ToUpper(item.Value) == "ALL" {
							hasDropAll = true
							break
						}
					}
				}
				if !hasDropAll {
					loc := c.ContainerNode.Location
					if capsDrop != nil {
						loc = capsDrop.Location
					}
					findings = append(findings, diagnostic.Finding{
						RuleID:        RuleCapDropNotAll,
						Severity:      diagnostic.SeverityInfo,
						Message:       fmt.Sprintf("Container %q does not drop ALL capabilities in securityContext", c.Name),
						Location:      loc,
						EvidenceClass: diagnostic.EvidenceContextDependent,
						Details:       "Specify capabilities.drop: ['ALL'] to adhere to the principle of least privilege.",
					})
				}
			}
			return findings
		},
	}
}

// 3. SEC-CTX-005: hostNetwork enabled
func ruleHostNetwork() *rule.Rule {
	return &rule.Rule{
		ID:            RuleHostNetwork,
		Title:         "Host network namespace shared",
		Description:   "hostNetwork enables raw access to host network interfaces and loopback traffic",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityWarning,
		EvidenceClass: diagnostic.EvidenceDeterministic,
		ApplicableTo:  rule.WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			podSpec := rule.ExtractPodSpec(res)
			if podSpec == nil {
				return nil
			}
			hostNet := podSpec.Get("hostNetwork")
			if hostNet != nil && hostNet.Value == "true" {
				return []diagnostic.Finding{{
					RuleID:        RuleHostNetwork,
					Severity:      diagnostic.SeverityWarning,
					Message:       fmt.Sprintf("%s %q enables hostNetwork: true", res.ID.Kind, res.ID.Name),
					Location:      hostNet.Location,
					EvidenceClass: diagnostic.EvidenceDeterministic,
					Details:       "Pods with hostNetwork can sniff host traffic and bind to host ports, bypassing NetworkPolicies.",
				}}
			}
			return nil
		},
	}
}

// 4. SEC-CTX-006: hostPID enabled
func ruleHostPID() *rule.Rule {
	return &rule.Rule{
		ID:            RuleHostPID,
		Title:         "Host PID namespace shared",
		Description:   "hostPID shares the host process table, allowing containers to view and signal processes outside the container",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityWarning,
		EvidenceClass: diagnostic.EvidenceDeterministic,
		ApplicableTo:  rule.WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			podSpec := rule.ExtractPodSpec(res)
			if podSpec == nil {
				return nil
			}
			hostPID := podSpec.Get("hostPID")
			if hostPID != nil && hostPID.Value == "true" {
				return []diagnostic.Finding{{
					RuleID:        RuleHostPID,
					Severity:      diagnostic.SeverityWarning,
					Message:       fmt.Sprintf("%s %q enables hostPID: true", res.ID.Kind, res.ID.Name),
					Location:      hostPID.Location,
					EvidenceClass: diagnostic.EvidenceDeterministic,
					Details:       "Sharing the host PID namespace exposes all host processes and allows inter-process signaling.",
				}}
			}
			return nil
		},
	}
}

// 5. SEC-CTX-007: hostIPC enabled
func ruleHostIPC() *rule.Rule {
	return &rule.Rule{
		ID:            RuleHostIPC,
		Title:         "Host IPC namespace shared",
		Description:   "hostIPC shares IPC mechanisms like shared memory segments with the host and other pods",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityWarning,
		EvidenceClass: diagnostic.EvidenceDeterministic,
		ApplicableTo:  rule.WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			podSpec := rule.ExtractPodSpec(res)
			if podSpec == nil {
				return nil
			}
			hostIPC := podSpec.Get("hostIPC")
			if hostIPC != nil && hostIPC.Value == "true" {
				return []diagnostic.Finding{{
					RuleID:        RuleHostIPC,
					Severity:      diagnostic.SeverityWarning,
					Message:       fmt.Sprintf("%s %q enables hostIPC: true", res.ID.Kind, res.ID.Name),
					Location:      hostIPC.Location,
					EvidenceClass: diagnostic.EvidenceDeterministic,
					Details:       "Sharing the host IPC namespace allows access to shared memory segments of other host processes.",
				}}
			}
			return nil
		},
	}
}

// 6. SEC-CTX-008, SEC-CTX-009, SEC-CTX-010, SEC-CTX-011: hostPath volume evaluations
func ruleHostPathMounts() *rule.Rule {
	return &rule.Rule{
		ID:            RuleHostPathGeneral,
		Title:         "Host path volume mount security analysis",
		Description:   "Evaluates hostPath volumes against critical host locations and container runtime sockets",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityWarning,
		EvidenceClass: diagnostic.EvidenceDeterministic,
		ApplicableTo:  rule.WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			podSpec := rule.ExtractPodSpec(res)
			if podSpec == nil {
				return nil
			}
			volumesNode := podSpec.Get("volumes")
			if volumesNode == nil || volumesNode.Kind != nir.KindSequence {
				return nil
			}

			var findings []diagnostic.Finding
			for _, vol := range volumesNode.Items {
				if vol.Kind != nir.KindMapping {
					continue
				}
				hostPathNode := vol.Get("hostPath")
				if hostPathNode == nil {
					continue
				}
				volName := getString(vol, "name")
				pathNode := hostPathNode.Get("path")
				pathVal := ""
				loc := hostPathNode.Location
				if pathNode != nil {
					pathVal = strings.TrimSpace(pathNode.Value)
					loc = pathNode.Location
				}

				cleanPath := strings.TrimRight(pathVal, "/")
				if cleanPath == "" {
					cleanPath = "/"
				}

				// SEC-CTX-009: Container runtime socket mount
				if runtimeSocketPaths[pathVal] || runtimeSocketPaths[cleanPath] {
					findings = append(findings, diagnostic.Finding{
						RuleID:        RuleHostPathRuntimeSocket,
						Severity:      diagnostic.SeverityError,
						Message:       fmt.Sprintf("Volume %q mounts container runtime socket %q; this allows full host root takeover", volName, pathVal),
						Location:      loc,
						EvidenceClass: diagnostic.EvidenceDeterministic,
						Details:       "Mounting the container runtime socket permits commanding the container runtime to launch privileged containers with host root access.",
					})
					continue
				}

				// SEC-CTX-008: Host root or /etc mount
				if cleanPath == "/" || cleanPath == "/etc" {
					findings = append(findings, diagnostic.Finding{
						RuleID:        RuleHostPathRootOrEtc,
						Severity:      diagnostic.SeverityError,
						Message:       fmt.Sprintf("Volume %q mounts host sensitive system directory %q", volName, pathVal),
						Location:      loc,
						EvidenceClass: diagnostic.EvidenceDeterministic,
						Details:       "Mounting / or /etc allows reading and mutating critical host credentials and configurations.",
					})
					continue
				}

				// SEC-CTX-010: /var/lib/kubelet mount
				if strings.HasPrefix(cleanPath, "/var/lib/kubelet") {
					findings = append(findings, diagnostic.Finding{
						RuleID:        RuleHostPathKubelet,
						Severity:      diagnostic.SeverityError,
						Message:       fmt.Sprintf("Volume %q mounts /var/lib/kubelet directory %q", volName, pathVal),
						Location:      loc,
						EvidenceClass: diagnostic.EvidenceDeterministic,
						Details:       "Mounting kubelet directories exposes volume tokens and node credentials.",
					})
					continue
				}

				// SEC-CTX-011: Any other hostPath
				findings = append(findings, diagnostic.Finding{
					RuleID:        RuleHostPathGeneral,
					Severity:      diagnostic.SeverityWarning,
					Message:       fmt.Sprintf("Volume %q mounts host path %q; hostPath volumes bypass container filesystem isolation", volName, pathVal),
					Location:      loc,
					EvidenceClass: diagnostic.EvidenceProbable,
					Details:       "Avoid hostPath volumes; use PVC, emptyDir, or Secret/ConfigMap volumes instead.",
				})
			}
			return findings
		},
	}
}

// 7. SEC-CTX-014 & SEC-CTX-016: runAsUser and runAsNonRoot conflicts
func ruleRunAsRootConflicts() *rule.Rule {
	return &rule.Rule{
		ID:            RuleRunAsNonRootWithZero,
		Title:         "Root user execution or configuration conflict",
		Description:   "Verifies that runAsUser does not run as root (UID 0) or conflict with runAsNonRoot",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityError,
		EvidenceClass: diagnostic.EvidenceDeterministic,
		ApplicableTo:  rule.WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			var findings []diagnostic.Finding
			podSpec := rule.ExtractPodSpec(res)
			podRunAsNonRoot := podSpec.Get("securityContext.runAsNonRoot")
			podRunAsUser := podSpec.Get("securityContext.runAsUser")

			for _, c := range rule.ExtractContainers(res) {
				cRunAsNonRoot := c.ContainerNode.Get("securityContext.runAsNonRoot")
				cRunAsUser := c.ContainerNode.Get("securityContext.runAsUser")

				effectiveNonRoot := false
				if cRunAsNonRoot != nil && cRunAsNonRoot.Value == "true" {
					effectiveNonRoot = true
				} else if cRunAsNonRoot == nil && podRunAsNonRoot != nil && podRunAsNonRoot.Value == "true" {
					effectiveNonRoot = true
				}

				effectiveUserNode := cRunAsUser
				if effectiveUserNode == nil {
					effectiveUserNode = podRunAsUser
				}

				if effectiveUserNode != nil && effectiveUserNode.Value == "0" {
					if effectiveNonRoot {
						// SEC-CTX-014: Contradiction! runAsNonRoot: true with runAsUser: 0
						findings = append(findings, diagnostic.Finding{
							RuleID:        RuleRunAsNonRootWithZero,
							Severity:      diagnostic.SeverityError,
							Message:       fmt.Sprintf("Container %q specifies runAsNonRoot: true but sets runAsUser: 0 (root)", c.Name),
							Location:      effectiveUserNode.Location,
							EvidenceClass: diagnostic.EvidenceDeterministic,
							Details:       "This contradictory configuration will cause the kubelet container admission to fail.",
						})
					} else {
						// SEC-CTX-016: Running as root
						findings = append(findings, diagnostic.Finding{
							RuleID:        RuleRunAsRootUser,
							Severity:      diagnostic.SeverityWarning,
							Message:       fmt.Sprintf("Container %q explicitly sets runAsUser: 0 (root)", c.Name),
							Location:      effectiveUserNode.Location,
							EvidenceClass: diagnostic.EvidenceProbable,
							Details:       "Running containers as root (UID 0) increases the blast radius of any container escape.",
						})
					}
				}
			}
			return findings
		},
	}
}

// 8. SEC-CTX-017: readOnlyRootFilesystem unset or false
func ruleReadOnlyRootFS() *rule.Rule {
	return &rule.Rule{
		ID:            RuleReadOnlyRootFSUnset,
		Title:         "Read-only root filesystem not enforced",
		Description:   "readOnlyRootFilesystem: true protects against in-container malware persistence",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityInfo,
		EvidenceClass: diagnostic.EvidenceContextDependent,
		ApplicableTo:  rule.WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			var findings []diagnostic.Finding
			for _, c := range rule.ExtractContainers(res) {
				roNode := c.ContainerNode.Get("securityContext.readOnlyRootFilesystem")
				if roNode == nil || roNode.Value != "true" {
					loc := c.ContainerNode.Location
					if roNode != nil {
						loc = roNode.Location
					}
					findings = append(findings, diagnostic.Finding{
						RuleID:        RuleReadOnlyRootFSUnset,
						Severity:      diagnostic.SeverityInfo,
						Message:       fmt.Sprintf("Container %q does not set readOnlyRootFilesystem: true", c.Name),
						Location:      loc,
						EvidenceClass: diagnostic.EvidenceContextDependent,
						Details:       "An immutable root filesystem prevents attackers from writing malicious binaries to disk.",
					})
				}
			}
			return findings
		},
	}
}

// 9. SEC-CTX-018: seccompProfile.type Unconfined
func ruleSeccompProfile() *rule.Rule {
	return &rule.Rule{
		ID:            RuleSeccompUnconfined,
		Title:         "Seccomp confinement disabled (Unconfined)",
		Description:   "Unconfined seccomp profiles allow containers to invoke all Linux system calls",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityWarning,
		EvidenceClass: diagnostic.EvidenceDeterministic,
		ApplicableTo:  rule.WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			var findings []diagnostic.Finding
			podSpec := rule.ExtractPodSpec(res)
			if podSpec == nil {
				return nil
			}

			checkNode := func(n *nir.Node, owner string) {
				if n == nil {
					return
				}
				stype := getString(n, "type")
				if strings.EqualFold(stype, "Unconfined") {
					findings = append(findings, diagnostic.Finding{
						RuleID:        RuleSeccompUnconfined,
						Severity:      diagnostic.SeverityWarning,
						Message:       fmt.Sprintf("%s explicitly sets seccompProfile.type to Unconfined", owner),
						Location:      n.Location,
						EvidenceClass: diagnostic.EvidenceDeterministic,
						Details:       "Use RuntimeDefault or Localhost seccomp profiles to filter dangerous system calls.",
					})
				}
			}

			checkNode(podSpec.Get("securityContext.seccompProfile"), fmt.Sprintf("%s %q Pod spec", res.ID.Kind, res.ID.Name))
			for _, c := range rule.ExtractContainers(res) {
				checkNode(c.ContainerNode.Get("securityContext.seccompProfile"), fmt.Sprintf("Container %q", c.Name))
			}
			return findings
		},
	}
}

// 10. SEC-RBAC-004, SEC-RBAC-005, SEC-RBAC-006: Wildcard verbs, resources, apiGroups
func ruleRBACWildcards() *rule.Rule {
	return &rule.Rule{
		ID:            RuleRBACWildcardVerbs,
		Title:         "Wildcard permissions in RBAC role rules",
		Description:   "Role and ClusterRole rules must not use wildcard '*' in verbs, resources, or apiGroups",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityWarning,
		EvidenceClass: diagnostic.EvidenceDeterministic,
		ApplicableTo:  []string{"Role", "ClusterRole"},
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			if res.RootNode == nil {
				return nil
			}
			rulesNode := res.RootNode.Get("rules")
			if rulesNode == nil || rulesNode.Kind != nir.KindSequence {
				return nil
			}

			var findings []diagnostic.Finding
			for _, rNode := range rulesNode.Items {
				if rNode.Kind != nir.KindMapping {
					continue
				}

				checkWildcardSeq := func(fieldName, ruleID string) {
					seq := rNode.Get(fieldName)
					if seq != nil && seq.Kind == nir.KindSequence {
						for _, item := range seq.Items {
							if item.Value == "*" {
								findings = append(findings, diagnostic.Finding{
									RuleID:        ruleID,
									Severity:      diagnostic.SeverityWarning,
									Message:       fmt.Sprintf("%s %q rule grants wildcard '*' for %s", res.ID.Kind, res.ID.Name, fieldName),
									Location:      item.Location,
									EvidenceClass: diagnostic.EvidenceDeterministic,
									Details:       "Wildcard permissions violate the principle of least privilege.",
								})
							}
						}
					}
				}

				checkWildcardSeq("verbs", RuleRBACWildcardVerbs)
				checkWildcardSeq("resources", RuleRBACWildcardResources)
				checkWildcardSeq("apiGroups", RuleRBACWildcardAPIGroups)
			}
			return findings
		},
	}
}

// 11. SEC-RBAC-012 & SEC-RBAC-013: High-privilege ClusterRoleBinding targets
func ruleRBACBindings() *rule.Rule {
	return &rule.Rule{
		ID:            RuleRBACSystemMasters,
		Title:         "Dangerous ClusterRoleBinding targets",
		Description:   "Prohibits binding to system:masters group or binding cluster-admin to ServiceAccounts",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityError,
		EvidenceClass: diagnostic.EvidenceDeterministic,
		ApplicableTo:  []string{"ClusterRoleBinding", "RoleBinding"},
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			if res.RootNode == nil {
				return nil
			}
			subjectsNode := res.RootNode.Get("subjects")
			if subjectsNode == nil || subjectsNode.Kind != nir.KindSequence {
				return nil
			}

			roleRefName := getString(res.RootNode, "roleRef.name")

			var findings []diagnostic.Finding
			for _, sub := range subjectsNode.Items {
				if sub.Kind != nir.KindMapping {
					continue
				}
				sKind := getString(sub, "kind")
				sName := getString(sub, "name")

				// SEC-RBAC-012: Binding to system:masters
				if sKind == "Group" && sName == "system:masters" {
					findings = append(findings, diagnostic.Finding{
						RuleID:        RuleRBACSystemMasters,
						Severity:      diagnostic.SeverityError,
						Message:       fmt.Sprintf("%s %q binds to superuser break-glass group 'system:masters'", res.ID.Kind, res.ID.Name),
						Location:      sub.Location,
						EvidenceClass: diagnostic.EvidenceDeterministic,
						Details:       "system:masters bypasses all RBAC authorizations and cannot be restricted by policy.",
					})
				}

				// SEC-RBAC-013: ClusterRoleBinding binding cluster-admin to a ServiceAccount
				if res.ID.Kind == "ClusterRoleBinding" && roleRefName == "cluster-admin" && sKind == "ServiceAccount" {
					findings = append(findings, diagnostic.Finding{
						RuleID:        RuleRBACClusterAdminSA,
						Severity:      diagnostic.SeverityWarning,
						Message:       fmt.Sprintf("ClusterRoleBinding %q grants full 'cluster-admin' privileges to ServiceAccount %q", res.ID.Name, sName),
						Location:      sub.Location,
						EvidenceClass: diagnostic.EvidenceProbable,
						Details:       "Avoid granting cluster-admin to ServiceAccounts; scope privileges to specific namespaces or roles.",
					})
				}
			}
			return findings
		},
	}
}

// 12. SEC-SECRET-001: secretKeyRef / secretRef with invalid 'namespace' field
func ruleSecretReferences() *rule.Rule {
	return &rule.Rule{
		ID:            RuleSecretRefWithNamespace,
		Title:         "Secret reference contains disallowed 'namespace' field",
		Description:   "Secret references in Pods are strictly intra-namespace and cannot specify a namespace field",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityError,
		EvidenceClass: diagnostic.EvidenceDeterministic,
		ApplicableTo:  rule.WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			var findings []diagnostic.Finding
			for _, c := range rule.ExtractContainers(res) {
				// env[*].valueFrom.secretKeyRef.namespace
				envNode := c.ContainerNode.Get("env")
				if envNode != nil && envNode.Kind == nir.KindSequence {
					for _, envItem := range envNode.Items {
						if envItem.Kind != nir.KindMapping {
							continue
						}
						nsNode := envItem.Get("valueFrom.secretKeyRef.namespace")
						if nsNode != nil {
							envName := getString(envItem, "name")
							findings = append(findings, diagnostic.Finding{
								RuleID:        RuleSecretRefWithNamespace,
								Severity:      diagnostic.SeverityError,
								Message:       fmt.Sprintf("secretKeyRef for env %q in container %q invalidly specifies 'namespace' field", envName, c.Name),
								Location:      nsNode.Location,
								EvidenceClass: diagnostic.EvidenceDeterministic,
								Details:       "Kubernetes secretKeyRef has no namespace field; secrets must reside in the workload's own namespace.",
							})
						}
					}
				}

				// envFrom[*].secretRef.namespace
				envFromNode := c.ContainerNode.Get("envFrom")
				if envFromNode != nil && envFromNode.Kind == nir.KindSequence {
					for _, item := range envFromNode.Items {
						if item.Kind != nir.KindMapping {
							continue
						}
						nsNode := item.Get("secretRef.namespace")
						if nsNode != nil {
							findings = append(findings, diagnostic.Finding{
								RuleID:        RuleSecretRefWithNamespace,
								Severity:      diagnostic.SeverityError,
								Message:       fmt.Sprintf("secretRef in envFrom of container %q invalidly specifies 'namespace' field", c.Name),
								Location:      nsNode.Location,
								EvidenceClass: diagnostic.EvidenceDeterministic,
								Details:       "Kubernetes secretRef has no namespace field; cross-namespace secret references are rejected by API server.",
							})
						}
					}
				}
			}
			return findings
		},
	}
}

// 13. SEC-SECRET-006: Literal sensitive value embedded in env
func ruleLiteralSecretEnv() *rule.Rule {
	sensitivePatterns := []string{"PASSWORD", "TOKEN", "SECRET", "API_KEY", "APIKEY", "PRIVATE_KEY", "AUTH"}

	return &rule.Rule{
		ID:            RuleLiteralSecretEnv,
		Title:         "Literal sensitive value in container environment variable",
		Description:   "Sensitive values like passwords and API tokens should use valueFrom.secretKeyRef instead of plaintext literals",
		Category:      rule.CategorySecurity,
		Severity:      diagnostic.SeverityWarning,
		EvidenceClass: diagnostic.EvidenceProbable,
		ApplicableTo:  rule.WorkloadKinds,
		Evaluate: func(res *inventory.Resource, inv *inventory.Inventory) []diagnostic.Finding {
			var findings []diagnostic.Finding
			for _, c := range rule.ExtractContainers(res) {
				envNode := c.ContainerNode.Get("env")
				if envNode == nil || envNode.Kind != nir.KindSequence {
					continue
				}

				for _, envItem := range envNode.Items {
					if envItem.Kind != nir.KindMapping {
						continue
					}
					name := getString(envItem, "name")
					valNode := envItem.Get("value")
					if valNode == nil || valNode.Kind != nir.KindScalar || valNode.Value == "" {
						continue
					}

					upperName := strings.ToUpper(name)
					isSensitive := false
					for _, pat := range sensitivePatterns {
						if strings.Contains(upperName, pat) {
							isSensitive = true
							break
						}
					}

					if isSensitive {
						findings = append(findings, diagnostic.Finding{
							RuleID:        RuleLiteralSecretEnv,
							Severity:      diagnostic.SeverityWarning,
							Message:       fmt.Sprintf("Environment variable %q in container %q appears to store a literal secret value", name, c.Name),
							Location:      valNode.Location,
							EvidenceClass: diagnostic.EvidenceProbable,
							Details:       "Store credentials in a Kubernetes Secret and reference via valueFrom.secretKeyRef.",
						})
					}
				}
			}
			return findings
		},
	}
}
