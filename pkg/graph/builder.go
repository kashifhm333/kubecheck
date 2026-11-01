package graph

import (
	"fmt"
	"sort"

	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/nir"
	"github.com/kubecheck/kubecheck/pkg/rule"
)

// Builder constructs a relationship graph from an admitted inventory.
type Builder struct {
	inv   *inventory.Inventory
	graph *Graph
}

// NewBuilder initializes a Builder.
func NewBuilder(inv *inventory.Inventory) *Builder {
	return &Builder{
		inv:   inv,
		graph: NewGraph(),
	}
}

// Build indexes nodes and resolves all typed reference edges.
func (b *Builder) Build() *Graph {
	if b.inv == nil {
		return b.graph
	}

	// Step 1: Populate all nodes and secondary indexes
	for _, res := range b.inv.Resources {
		scopeKey := res.ID.ScopeKey()
		node := &Node{
			Resource: res,
		}
		b.graph.Nodes[scopeKey] = node
		b.graph.byKind[res.ID.Kind] = append(b.graph.byKind[res.ID.Kind], node)

		ns := res.ID.Namespace
		if ns == "" {
			ns = "_cluster_"
		}
		b.graph.byNS[ns] = append(b.graph.byNS[ns], node)
	}

	// Step 2: Extract reference edges from each resource
	for _, res := range b.inv.Resources {
		b.extractEdges(res)
	}

	// Step 3: Sort edges deterministically per CROSS-RES-REQ-008
	sort.Slice(b.graph.Edges, func(i, j int) bool {
		ei, ej := b.graph.Edges[i], b.graph.Edges[j]
		if ei.From.String() != ej.From.String() {
			return ei.From.String() < ej.From.String()
		}
		if ei.Type != ej.Type {
			return ei.Type < ej.Type
		}
		if ei.Location.FilePath != ej.Location.FilePath {
			return ei.Location.FilePath < ej.Location.FilePath
		}
		return ei.Location.Line < ej.Location.Line
	})

	return b.graph
}

func (b *Builder) addEdge(e *Edge) {
	b.graph.Edges = append(b.graph.Edges, e)

	fromKey := e.From.ScopeKey()
	if fromNode, ok := b.graph.Nodes[fromKey]; ok {
		fromNode.OutgoingEdges = append(fromNode.OutgoingEdges, e)
		e.SourceResource = fromNode.Resource
	}

	if e.To != nil {
		toKey := e.To.ScopeKey()
		if toNode, ok := b.graph.Nodes[toKey]; ok {
			toNode.IncomingEdges = append(toNode.IncomingEdges, e)
			e.TargetResource = toNode.Resource
			e.Status = StatusResolved
		} else {
			e.Status = StatusUnresolved
		}
	} else {
		e.Status = StatusUnresolved
	}
}

func (b *Builder) extractEdges(res *inventory.Resource) {
	// 1. Owner References
	b.extractOwnerReferences(res)

	// 2. Pod and Workload references (volumes, env, serviceAccount)
	b.extractPodReferences(res)

	// 3. Service Selectors
	if res.ID.Kind == "Service" {
		b.extractServiceSelectors(res)
	}

	// 4. Ingress Backends & TLS
	if res.ID.Kind == "Ingress" {
		b.extractIngressReferences(res)
	}

	// 5. RBAC bindings (RoleBinding, ClusterRoleBinding)
	if res.ID.Kind == "RoleBinding" || res.ID.Kind == "ClusterRoleBinding" {
		b.extractRBACReferences(res)
	}
}

func (b *Builder) extractOwnerReferences(res *inventory.Resource) {
	if res.RootNode == nil {
		return
	}
	ownerRefs := res.RootNode.Get("metadata.ownerReferences")
	if ownerRefs == nil || ownerRefs.Kind != nir.KindSequence {
		return
	}

	for _, refNode := range ownerRefs.Items {
		name, _ := refNode.GetString("name")
		kind, _ := refNode.GetString("kind")
		apiVer, _ := refNode.GetString("apiVersion")
		group, version, _ := inventory.ParseAPIVersion(apiVer)

		if name == "" || kind == "" {
			continue
		}

		targetID := inventory.ResourceID{
			Group:     group,
			Version:   version,
			Kind:      kind,
			Namespace: res.ID.Namespace,
			Name:      name,
		}

		b.addEdge(&Edge{
			ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeOwnerReference),
			Type:     EdgeOwnerReference,
			From:     res.ID,
			To:       &targetID,
			ToHint:   EdgeTargetHint{Group: group, Kind: kind, Namespace: res.ID.Namespace, Name: name},
			Location: refNode.Location,
		})
	}
}

func (b *Builder) extractPodReferences(res *inventory.Resource) {
	podSpec := rule.ExtractPodSpec(res)
	if podSpec == nil {
		return
	}

	ns := res.ID.Namespace

	// ServiceAccount reference
	saName, ok := podSpec.GetString("serviceAccountName")
	if !ok {
		saName, _ = podSpec.GetString("serviceAccount")
	}
	if saName != "" && saName != "default" {
		targetID := inventory.ResourceID{
			Kind:      "ServiceAccount",
			Namespace: ns,
			Name:      saName,
		}
		saNode := podSpec.Get("serviceAccountName")
		loc := podSpec.Location
		if saNode != nil {
			loc = saNode.Location
		}
		b.addEdge(&Edge{
			ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeServiceAccountRef),
			Type:     EdgeServiceAccountRef,
			From:     res.ID,
			To:       &targetID,
			ToHint:   EdgeTargetHint{Kind: "ServiceAccount", Namespace: ns, Name: saName},
			Location: loc,
		})
	}

	// Volume references
	if volumes := podSpec.Get("volumes"); volumes != nil && volumes.Kind == nir.KindSequence {
		for _, v := range volumes.Items {
			// ConfigMap volume
			if cm := v.Get("configMap"); cm != nil {
				cmName, _ := cm.GetString("name")
				opt, _ := cm.GetBool("optional")
				if cmName != "" {
					targetID := inventory.ResourceID{Kind: "ConfigMap", Namespace: ns, Name: cmName}
					b.addEdge(&Edge{
						ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeConfigMapRef),
						Type:     EdgeConfigMapRef,
						From:     res.ID,
						To:       &targetID,
						ToHint:   EdgeTargetHint{Kind: "ConfigMap", Namespace: ns, Name: cmName},
						Location: cm.Location,
						Optional: opt,
					})
				}
			}

			// Secret volume
			if sec := v.Get("secret"); sec != nil {
				secName, _ := sec.GetString("secretName")
				opt, _ := sec.GetBool("optional")
				if secName != "" {
					targetID := inventory.ResourceID{Kind: "Secret", Namespace: ns, Name: secName}
					b.addEdge(&Edge{
						ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeSecretRef),
						Type:     EdgeSecretRef,
						From:     res.ID,
						To:       &targetID,
						ToHint:   EdgeTargetHint{Kind: "Secret", Namespace: ns, Name: secName},
						Location: sec.Location,
						Optional: opt,
					})
				}
			}

			// PVC volume
			if pvc := v.Get("persistentVolumeClaim"); pvc != nil {
				claimName, _ := pvc.GetString("claimName")
				if claimName != "" {
					targetID := inventory.ResourceID{Kind: "PersistentVolumeClaim", Namespace: ns, Name: claimName}
					b.addEdge(&Edge{
						ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgePVCRef),
						Type:     EdgePVCRef,
						From:     res.ID,
						To:       &targetID,
						ToHint:   EdgeTargetHint{Kind: "PersistentVolumeClaim", Namespace: ns, Name: claimName},
						Location: pvc.Location,
					})
				}
			}
		}
	}

	// Containers env and envFrom references
	containers := rule.ExtractContainers(res)
	for _, c := range containers {
		// envFrom
		if envFrom := c.ContainerNode.Get("envFrom"); envFrom != nil && envFrom.Kind == nir.KindSequence {
			for _, ef := range envFrom.Items {
				if cmRef := ef.Get("configMapRef"); cmRef != nil {
					cmName, _ := cmRef.GetString("name")
					opt, _ := cmRef.GetBool("optional")
					if cmName != "" {
						targetID := inventory.ResourceID{Kind: "ConfigMap", Namespace: ns, Name: cmName}
						b.addEdge(&Edge{
							ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeConfigMapRef),
							Type:     EdgeConfigMapRef,
							From:     res.ID,
							To:       &targetID,
							ToHint:   EdgeTargetHint{Kind: "ConfigMap", Namespace: ns, Name: cmName},
							Location: cmRef.Location,
							Optional: opt,
						})
					}
				}
				if secRef := ef.Get("secretRef"); secRef != nil {
					secName, _ := secRef.GetString("name")
					opt, _ := secRef.GetBool("optional")
					if secName != "" {
						targetID := inventory.ResourceID{Kind: "Secret", Namespace: ns, Name: secName}
						b.addEdge(&Edge{
							ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeSecretRef),
							Type:     EdgeSecretRef,
							From:     res.ID,
							To:       &targetID,
							ToHint:   EdgeTargetHint{Kind: "Secret", Namespace: ns, Name: secName},
							Location: secRef.Location,
							Optional: opt,
						})
					}
				}
			}
		}

		// env[*].valueFrom
		if envList := c.ContainerNode.Get("env"); envList != nil && envList.Kind == nir.KindSequence {
			for _, e := range envList.Items {
				if vf := e.Get("valueFrom"); vf != nil {
					if cmKey := vf.Get("configMapKeyRef"); cmKey != nil {
						cmName, _ := cmKey.GetString("name")
						opt, _ := cmKey.GetBool("optional")
						if cmName != "" {
							targetID := inventory.ResourceID{Kind: "ConfigMap", Namespace: ns, Name: cmName}
							b.addEdge(&Edge{
								ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeConfigMapRef),
								Type:     EdgeConfigMapRef,
								From:     res.ID,
								To:       &targetID,
								ToHint:   EdgeTargetHint{Kind: "ConfigMap", Namespace: ns, Name: cmName},
								Location: cmKey.Location,
								Optional: opt,
							})
						}
					}
					if secKey := vf.Get("secretKeyRef"); secKey != nil {
						secName, _ := secKey.GetString("name")
						opt, _ := secKey.GetBool("optional")
						if secName != "" {
							targetID := inventory.ResourceID{Kind: "Secret", Namespace: ns, Name: secName}
							b.addEdge(&Edge{
								ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeSecretRef),
								Type:     EdgeSecretRef,
								From:     res.ID,
								To:       &targetID,
								ToHint:   EdgeTargetHint{Kind: "Secret", Namespace: ns, Name: secName},
								Location: secKey.Location,
								Optional: opt,
							})
						}
					}
				}
			}
		}
	}
}

func (b *Builder) extractServiceSelectors(res *inventory.Resource) {
	if res.RootNode == nil {
		return
	}
	selNode := res.RootNode.Get("spec.selector")
	if selNode == nil || selNode.Kind != nir.KindMapping || len(selNode.Fields) == 0 {
		return
	}

	selector := make(map[string]string)
	for _, f := range selNode.Fields {
		if f.Key != nil && f.Value != nil {
			selector[f.Key.Value] = f.Value.Value
		}
	}

	matchedAny := false
	ns := res.ID.Namespace

	// Match against all workloads in the same namespace
	for _, wKind := range rule.WorkloadKinds {
		for _, targetNode := range b.graph.byKind[wKind] {
			targetRes := targetNode.Resource
			if targetRes.ID.Namespace != ns {
				continue
			}

			// Extract pod template labels
			var labels map[string]string
			if targetRes.ID.Kind == "Pod" {
				labels = targetRes.Labels
			} else if tm := targetRes.RootNode.Get("spec.template.metadata.labels"); tm != nil {
				labels = extractLabels(tm)
			}

			if MatchesSelector(selector, labels) {
				matchedAny = true
				targetID := targetRes.ID
				b.addEdge(&Edge{
					ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeServiceSelectorMatch),
					Type:     EdgeServiceSelectorMatch,
					From:     res.ID,
					To:       &targetID,
					ToHint:   EdgeTargetHint{Kind: targetRes.ID.Kind, Namespace: ns, Name: targetRes.ID.Name, Selector: selector},
					Location: selNode.Location,
				})
			}
		}
	}

	if !matchedAny {
		// Record unresolved selector edge
		b.addEdge(&Edge{
			ID:       fmt.Sprintf("%s->Pod(%s)", res.ID, EdgeServiceSelectorMatch),
			Type:     EdgeServiceSelectorMatch,
			From:     res.ID,
			To:       nil,
			ToHint:   EdgeTargetHint{Kind: "Pod", Namespace: ns, Selector: selector},
			Location: selNode.Location,
		})
	}
}

func (b *Builder) extractIngressReferences(res *inventory.Resource) {
	if res.RootNode == nil {
		return
	}
	ns := res.ID.Namespace

	// Default backend
	if db := res.RootNode.Get("spec.defaultBackend.service.name"); db != nil && db.Value != "" {
		targetID := inventory.ResourceID{Kind: "Service", Namespace: ns, Name: db.Value}
		b.addEdge(&Edge{
			ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeIngressBackendServiceRef),
			Type:     EdgeIngressBackendServiceRef,
			From:     res.ID,
			To:       &targetID,
			ToHint:   EdgeTargetHint{Kind: "Service", Namespace: ns, Name: db.Value},
			Location: db.Location,
		})
	}

	// Rules paths backends
	if rules := res.RootNode.Get("spec.rules"); rules != nil && rules.Kind == nir.KindSequence {
		for _, r := range rules.Items {
			if paths := r.Get("http.paths"); paths != nil && paths.Kind == nir.KindSequence {
				for _, p := range paths.Items {
					if svcName := p.Get("backend.service.name"); svcName != nil && svcName.Value != "" {
						targetID := inventory.ResourceID{Kind: "Service", Namespace: ns, Name: svcName.Value}
						b.addEdge(&Edge{
							ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeIngressBackendServiceRef),
							Type:     EdgeIngressBackendServiceRef,
							From:     res.ID,
							To:       &targetID,
							ToHint:   EdgeTargetHint{Kind: "Service", Namespace: ns, Name: svcName.Value},
							Location: svcName.Location,
						})
					}
				}
			}
		}
	}

	// TLS secrets
	if tlsList := res.RootNode.Get("spec.tls"); tlsList != nil && tlsList.Kind == nir.KindSequence {
		for _, tls := range tlsList.Items {
			if secName := tls.Get("secretName"); secName != nil && secName.Value != "" {
				targetID := inventory.ResourceID{Kind: "Secret", Namespace: ns, Name: secName.Value}
				b.addEdge(&Edge{
					ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeSecretRef),
					Type:     EdgeSecretRef,
					From:     res.ID,
					To:       &targetID,
					ToHint:   EdgeTargetHint{Kind: "Secret", Namespace: ns, Name: secName.Value},
					Location: secName.Location,
				})
			}
		}
	}
}

func (b *Builder) extractRBACReferences(res *inventory.Resource) {
	if res.RootNode == nil {
		return
	}
	roleRef := res.RootNode.Get("roleRef")
	if roleRef != nil {
		rKind, _ := roleRef.GetString("kind")
		rName, _ := roleRef.GetString("name")
		if rName != "" && rKind != "" {
			ns := res.ID.Namespace
			if rKind == "ClusterRole" {
				ns = "" // ClusterRole is cluster-scoped
			}
			targetID := inventory.ResourceID{
				Kind:      rKind,
				Namespace: ns,
				Name:      rName,
			}
			b.addEdge(&Edge{
				ID:       fmt.Sprintf("%s->%s(%s)", res.ID, targetID, EdgeRoleRef),
				Type:     EdgeRoleRef,
				From:     res.ID,
				To:       &targetID,
				ToHint:   EdgeTargetHint{Kind: rKind, Namespace: ns, Name: rName},
				Location: roleRef.Location,
			})
		}
	}
}

// MatchesSelector checks if all key-values in selector exist in targetLabels.
func MatchesSelector(selector, targetLabels map[string]string) bool {
	if len(selector) == 0 || len(targetLabels) == 0 {
		return false
	}
	for k, v := range selector {
		if targetVal, exists := targetLabels[k]; !exists || targetVal != v {
			return false
		}
	}
	return true
}

func extractLabels(n *nir.Node) map[string]string {
	if n == nil || n.Kind != nir.KindMapping {
		return nil
	}
	m := make(map[string]string)
	for _, f := range n.Fields {
		if f.Key != nil && f.Value != nil {
			m[f.Key.Value] = f.Value.Value
		}
	}
	return m
}
