package graph

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kubecheck/kubecheck/pkg/diagnostic"
	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/nir"
	"github.com/kubecheck/kubecheck/pkg/rule"
)

// Normative cross-resource diagnostic rule IDs per CROSS-RES-001 and STATIC_ANALYSIS_RULES.md.
const (
	RuleWorkloadSelectorSelfMismatch = "CROSS-WORKLOAD-SEL-001"
	RuleServiceSelectorNoMatch       = "CROSS-SVC-001"
	RuleServicePortMismatch          = "CROSS-SVC-PORT-001"
	RuleMissingConfigRef             = "CROSS-REF-001"
	RuleMissingIngressBackend        = "CROSS-ING-001"
	RuleMissingRBACRole              = "CROSS-RBAC-001"
	RuleMissingServiceAccount        = "CROSS-SA-001"
	RuleCyclicRelationship           = "CROSS-CYCLE-001"
)

// Analyze evaluates the relationship graph and emits canonical cross-resource diagnostic findings.
func Analyze(g *Graph) []diagnostic.Finding {
	if g == nil {
		return nil
	}

	var findings []diagnostic.Finding

	// 1. Workload Self-Selector Match Check (CROSS-WORKLOAD-SEL-001)
	findings = append(findings, checkWorkloadSelectors(g)...)

	// 2. Cycle Detection in Owner References (CROSS-CYCLE-001)
	findings = append(findings, detectCycles(g)...)

	// 3. Unresolved and Mismatched Edge Checks
	for _, edge := range g.Edges {
		switch edge.Type {
		case EdgeServiceSelectorMatch:
			if edge.Status == StatusUnresolved {
				findings = append(findings, diagnostic.Finding{
					RuleID:        RuleServiceSelectorNoMatch,
					Severity:      diagnostic.SeverityInfo,
					Message:       fmt.Sprintf("Service %q selector %v matches 0 workloads in the analyzed manifests; target pods may already exist in the cluster", edge.From.Name, edge.ToHint.Selector),
					Location:      edge.Location,
					EvidenceClass: diagnostic.EvidenceProbable,
					Details:       "Offline analysis cannot verify live cluster pods. Supply workload manifests if you intend for them to be co-validated.",
				})
			} else if edge.Status == StatusResolved && edge.TargetResource != nil {
				// Port matching check (CROSS-SVC-PORT-001)
				if portFindings := checkServicePortMatch(edge.SourceResource, edge.TargetResource); len(portFindings) > 0 {
					findings = append(findings, portFindings...)
				}
			}

		case EdgeConfigMapRef, EdgeSecretRef:
			if edge.Status == StatusUnresolved && !edge.Optional {
				findings = append(findings, diagnostic.Finding{
					RuleID:        RuleMissingConfigRef,
					Severity:      diagnostic.SeverityInfo,
					Message:       fmt.Sprintf("Referenced %s %q was not found in the analyzed manifests; it may exist in the cluster or be managed externally", edge.ToHint.Kind, edge.ToHint.Name),
					Location:      edge.Location,
					EvidenceClass: diagnostic.EvidenceProbable,
					Details:       "ConfigMaps and Secrets can be provisioned by external operators, Helm, or cluster state.",
				})
			}

		case EdgeIngressBackendServiceRef:
			if edge.Status == StatusUnresolved {
				findings = append(findings, diagnostic.Finding{
					RuleID:        RuleMissingIngressBackend,
					Severity:      diagnostic.SeverityInfo,
					Message:       fmt.Sprintf("Ingress references backend Service %q which was not found in the analyzed manifests; it may exist in the cluster", edge.ToHint.Name),
					Location:      edge.Location,
					EvidenceClass: diagnostic.EvidenceProbable,
					Details:       "Ensure the backend Service is included in the manifest set if local traffic routing validation is required.",
				})
			}

		case EdgeRoleRef:
			if edge.Status == StatusUnresolved {
				findings = append(findings, diagnostic.Finding{
					RuleID:        RuleMissingRBACRole,
					Severity:      diagnostic.SeverityInfo,
					Message:       fmt.Sprintf("RoleBinding references %s %q which was not found in the analyzed manifests; it may exist in the cluster", edge.ToHint.Kind, edge.ToHint.Name),
					Location:      edge.Location,
					EvidenceClass: diagnostic.EvidenceProbable,
					Details:       "ClusterRoles and Roles may be pre-existing cluster-wide RBAC policies.",
				})
			}

		case EdgeServiceAccountRef:
			if edge.Status == StatusUnresolved {
				findings = append(findings, diagnostic.Finding{
					RuleID:        RuleMissingServiceAccount,
					Severity:      diagnostic.SeverityInfo,
					Message:       fmt.Sprintf("Workload specifies custom ServiceAccount %q which was not found in the analyzed manifests; it may exist in the cluster", edge.ToHint.Name),
					Location:      edge.Location,
					EvidenceClass: diagnostic.EvidenceProbable,
					Details:       "Custom ServiceAccounts should be declared in the manifest stream if RBAC validation is needed.",
				})
			}
		}
	}

	return findings
}

func checkWorkloadSelectors(g *Graph) []diagnostic.Finding {
	var findings []diagnostic.Finding

	for _, node := range g.Nodes {
		res := node.Resource
		if res == nil || res.RootNode == nil {
			continue
		}

		// Only workloads with spec.selector and spec.template
		if res.ID.Kind != "Deployment" && res.ID.Kind != "StatefulSet" && res.ID.Kind != "DaemonSet" && res.ID.Kind != "Job" {
			continue
		}

		selNode := res.RootNode.Get("spec.selector.matchLabels")
		if selNode == nil || selNode.Kind != nir.KindMapping {
			continue
		}

		templateLabelsNode := res.RootNode.Get("spec.template.metadata.labels")
		templateLabels := extractLabels(templateLabelsNode)
		selLabels := extractLabels(selNode)

		if !MatchesSelector(selLabels, templateLabels) {
			findings = append(findings, diagnostic.Finding{
				RuleID:        RuleWorkloadSelectorSelfMismatch,
				Severity:      diagnostic.SeverityError,
				Message:       fmt.Sprintf("%s %q spec.selector %v does not match its own spec.template.metadata.labels %v", res.ID.Kind, res.ID.Name, selLabels, templateLabels),
				Location:      selNode.Location,
				EvidenceClass: diagnostic.EvidenceDeterministic,
				Details:       "Kubernetes rejects workloads whose selector does not match the template labels with an unrecoverable validation error.",
			})
		}
	}

	return findings
}

func checkServicePortMatch(svcRes, targetRes *inventory.Resource) []diagnostic.Finding {
	if svcRes == nil || targetRes == nil || svcRes.RootNode == nil {
		return nil
	}

	portsNode := svcRes.RootNode.Get("spec.ports")
	if portsNode == nil || portsNode.Kind != nir.KindSequence {
		return nil
	}

	// Gather all exposed container ports in target workload
	targetContainers := rule.ExtractContainers(targetRes)
	type exposedPort struct {
		number int64
		name   string
	}
	var exposed []exposedPort

	for _, c := range targetContainers {
		if pNode := c.ContainerNode.Get("ports"); pNode != nil && pNode.Kind == nir.KindSequence {
			for _, portEntry := range pNode.Items {
				num, _ := portEntry.GetInt("containerPort")
				name, _ := portEntry.GetString("name")
				exposed = append(exposed, exposedPort{number: num, name: name})
			}
		}
	}

	// If target has no container ports declared at all, skip check (ports may be implicit)
	if len(exposed) == 0 {
		return nil
	}

	var findings []diagnostic.Finding
	for _, portEntry := range portsNode.Items {
		var targetPortStr string
		var targetPortInt int64

		tpNode := portEntry.Get("targetPort")
		if tpNode != nil {
			if tpNode.ScalarType == nir.TypeInt {
				targetPortInt, _ = strconv.ParseInt(tpNode.Value, 10, 64)
			} else {
				targetPortStr = tpNode.Value
			}
		} else {
			// If targetPort omitted, defaults to spec.ports[*].port
			targetPortInt, _ = portEntry.GetInt("port")
		}

		matched := false
		for _, exp := range exposed {
			if targetPortInt > 0 && exp.number == targetPortInt {
				matched = true
				break
			}
			if targetPortStr != "" && exp.name == targetPortStr {
				matched = true
				break
			}
			if targetPortStr != "" {
				// Also check if string is numeric
				if parsed, err := strconv.ParseInt(targetPortStr, 10, 64); err == nil && exp.number == parsed {
					matched = true
					break
				}
			}
		}

		if !matched {
			tpDesc := fmt.Sprintf("%d", targetPortInt)
			if targetPortStr != "" {
				tpDesc = targetPortStr
			}
			portNum, _ := portEntry.GetInt("port")
			loc := portEntry.Location
			if tpNode != nil {
				loc = tpNode.Location
			}
			findings = append(findings, diagnostic.Finding{
				RuleID:        RuleServicePortMismatch,
				Severity:      diagnostic.SeverityWarning,
				Message:       fmt.Sprintf("Service %q port %d targets port %s, but matched %s %q does not expose a matching containerPort", svcRes.ID.Name, portNum, tpDesc, targetRes.ID.Kind, targetRes.ID.Name),
				Location:      loc,
				EvidenceClass: diagnostic.EvidenceProbable,
				Details:       "Traffic sent to this service port may be dropped because no container is listening on the expected targetPort.",
			})
		}
	}

	return findings
}

func detectCycles(g *Graph) []diagnostic.Finding {
	var findings []diagnostic.Finding

	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var checkNode func(nodeKey string, path []string)
	checkNode = func(nodeKey string, path []string) {
		visited[nodeKey] = true
		recStack[nodeKey] = true
		path = append(path, nodeKey)

		node := g.Nodes[nodeKey]
		if node != nil {
			for _, edge := range node.OutgoingEdges {
				if edge.Type != EdgeOwnerReference || edge.To == nil {
					continue
				}
				toKey := edge.To.ScopeKey()
				if !visited[toKey] {
					checkNode(toKey, path)
				} else if recStack[toKey] {
					// Cycle detected!
					cyclePath := append(path, toKey)
					findings = append(findings, diagnostic.Finding{
						RuleID:        RuleCyclicRelationship,
						Severity:      diagnostic.SeverityError,
						Message:       fmt.Sprintf("Cyclic owner relationship detected: %s", strings.Join(cyclePath, " -> ")),
						Location:      edge.Location,
						EvidenceClass: diagnostic.EvidenceDeterministic,
						Details:       "Cyclic ownership references produce unresolvable garbage-collection deadlocks in Kubernetes.",
					})
				}
			}
		}

		recStack[nodeKey] = false
	}

	for key := range g.Nodes {
		if !visited[key] {
			checkNode(key, nil)
		}
	}

	return findings
}
