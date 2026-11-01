package schema

import (
	"fmt"
	"strconv"
	"strings"
)

// DefaultLifecycleEntries contains the normative deprecation and removal lifecycle table.
var DefaultLifecycleEntries = []LifecycleInfo{
	{Group: "extensions", Version: "v1beta1", Kind: "Deployment", DeprecatedIn: "1.9", RemovedIn: "1.16", ReplacementAPI: "apps/v1"},
	{Group: "extensions", Version: "v1beta1", Kind: "DaemonSet", DeprecatedIn: "1.9", RemovedIn: "1.16", ReplacementAPI: "apps/v1"},
	{Group: "extensions", Version: "v1beta1", Kind: "ReplicaSet", DeprecatedIn: "1.9", RemovedIn: "1.16", ReplacementAPI: "apps/v1"},
	{Group: "extensions", Version: "v1beta1", Kind: "Ingress", DeprecatedIn: "1.14", RemovedIn: "1.22", ReplacementAPI: "networking.k8s.io/v1"},
	{Group: "extensions", Version: "v1beta1", Kind: "NetworkPolicy", DeprecatedIn: "1.9", RemovedIn: "1.16", ReplacementAPI: "networking.k8s.io/v1"},
	{Group: "apps", Version: "v1beta1", Kind: "Deployment", DeprecatedIn: "1.9", RemovedIn: "1.16", ReplacementAPI: "apps/v1"},
	{Group: "apps", Version: "v1beta1", Kind: "StatefulSet", DeprecatedIn: "1.9", RemovedIn: "1.16", ReplacementAPI: "apps/v1"},
	{Group: "apps", Version: "v1beta2", Kind: "Deployment", DeprecatedIn: "1.9", RemovedIn: "1.16", ReplacementAPI: "apps/v1"},
	{Group: "networking.k8s.io", Version: "v1beta1", Kind: "Ingress", DeprecatedIn: "1.19", RemovedIn: "1.22", ReplacementAPI: "networking.k8s.io/v1"},
	{Group: "batch", Version: "v1beta1", Kind: "CronJob", DeprecatedIn: "1.21", RemovedIn: "1.25", ReplacementAPI: "batch/v1"},
	{Group: "policy", Version: "v1beta1", Kind: "PodDisruptionBudget", DeprecatedIn: "1.21", RemovedIn: "1.25", ReplacementAPI: "policy/v1"},
	{Group: "autoscaling", Version: "v2beta1", Kind: "HorizontalPodAutoscaler", DeprecatedIn: "1.22", RemovedIn: "1.25", ReplacementAPI: "autoscaling/v2"},
	{Group: "autoscaling", Version: "v2beta2", Kind: "HorizontalPodAutoscaler", DeprecatedIn: "1.23", RemovedIn: "1.26", ReplacementAPI: "autoscaling/v2"},
	{Group: "flowcontrol.apiserver.k8s.io", Version: "v1beta2", Kind: "FlowSchema", DeprecatedIn: "1.26", RemovedIn: "1.29", ReplacementAPI: "flowcontrol.apiserver.k8s.io/v1"},
	{Group: "flowcontrol.apiserver.k8s.io", Version: "v1beta2", Kind: "PriorityLevelConfiguration", DeprecatedIn: "1.26", RemovedIn: "1.29", ReplacementAPI: "flowcontrol.apiserver.k8s.io/v1"},
}

// Catalog holds all known resource schemas and lifecycle entries.
type Catalog struct {
	schemas   map[string]*ResourceSchema // key: [group/]kind or [group/]version/kind
	lifecycle []LifecycleInfo
}

// NewDefaultCatalog constructs a catalog prepopulated with standard Kubernetes schemas and deprecation data.
func NewDefaultCatalog() *Catalog {
	c := &Catalog{
		schemas:   make(map[string]*ResourceSchema),
		lifecycle: DefaultLifecycleEntries,
	}
	c.registerStandardSchemas()
	return c
}

// Register registers a ResourceSchema into the catalog.
func (c *Catalog) Register(rs *ResourceSchema) {
	// Key by full group/version/kind
	fullKey := fmt.Sprintf("%s/%s/%s", rs.Group, rs.Version, rs.Kind)
	c.schemas[fullKey] = rs

	// Also key by group/kind for default lookup
	gkKey := fmt.Sprintf("%s/%s", rs.Group, rs.Kind)
	if _, exists := c.schemas[gkKey]; !exists {
		c.schemas[gkKey] = rs
	}
}

// GetSchema returns the schema for the specified GVK.
func (c *Catalog) GetSchema(group, version, kind string) (*ResourceSchema, bool) {
	fullKey := fmt.Sprintf("%s/%s/%s", group, version, kind)
	if s, ok := c.schemas[fullKey]; ok {
		return s, true
	}
	gkKey := fmt.Sprintf("%s/%s", group, kind)
	if s, ok := c.schemas[gkKey]; ok {
		return s, true
	}
	return nil, false
}

// CheckLifecycle checks whether the API version is deprecated or removed in the target version.
func (c *Catalog) CheckLifecycle(group, version, kind, targetMinor string) (Finding, bool) {
	targetVer := ParseMinorVersion(targetMinor)
	for _, entry := range c.lifecycle {
		if entry.Group == group && entry.Version == version && (entry.Kind == "" || entry.Kind == kind) {
			removedVer := ParseMinorVersion(entry.RemovedIn)
			if removedVer > 0 && targetVer >= removedVer {
				return Finding{
					RuleID:        RuleRemovedAPI,
					Severity:      "error",
					Message:       fmt.Sprintf("%s for %s was removed in Kubernetes v%s; use %s instead", formatAPIVersion(group, version), kind, entry.RemovedIn, entry.ReplacementAPI),
					EvidenceClass: "deterministic",
					Details:       fmt.Sprintf("Target version: v%s, Removed in: v%s", targetMinor, entry.RemovedIn),
				}, true
			}

			deprecatedVer := ParseMinorVersion(entry.DeprecatedIn)
			if deprecatedVer > 0 && targetVer >= deprecatedVer {
				return Finding{
					RuleID:        RuleDeprecatedAPI,
					Severity:      "warning",
					Message:       fmt.Sprintf("%s for %s is deprecated since Kubernetes v%s; replace with %s", formatAPIVersion(group, version), kind, entry.DeprecatedIn, entry.ReplacementAPI),
					EvidenceClass: "deterministic",
					Details:       fmt.Sprintf("Target version: v%s, Deprecated in: v%s", targetMinor, entry.DeprecatedIn),
				}, true
			}
		}
	}
	return Finding{}, false
}

func formatAPIVersion(group, version string) string {
	if group == "" {
		return version
	}
	return group + "/" + version
}

// ParseMinorVersion converts "1.29" or "v1.29.1" to an integer minor version (e.g. 29).
func ParseMinorVersion(v string) int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) >= 2 {
		minor, err := strconv.Atoi(parts[1])
		if err == nil {
			return minor
		}
	}
	if len(parts) == 1 {
		val, err := strconv.Atoi(parts[0])
		if err == nil {
			return val
		}
	}
	return 0
}

func (c *Catalog) registerStandardSchemas() {
	metaSchema := &PropertySchema{
		Type: TypeObject,
		Properties: map[string]*PropertySchema{
			"name":                       {Type: TypeString},
			"namespace":                  {Type: TypeString},
			"labels":                     {Type: TypeObject, AdditionalProperties: true},
			"annotations":                {Type: TypeObject, AdditionalProperties: true},
			"generateName":               {Type: TypeString},
			"resourceVersion":            {Type: TypeString},
			"uid":                        {Type: TypeString},
			"generation":                 {Type: TypeInteger},
			"creationTimestamp":          {Type: TypeString},
			"deletionTimestamp":          {Type: TypeString},
			"deletionGracePeriodSeconds": {Type: TypeInteger},
			"ownerReferences": {
				Type: TypeArray,
				Items: &PropertySchema{
					Type:     TypeObject,
					Required: []string{"apiVersion", "kind", "name", "uid"},
					Properties: map[string]*PropertySchema{
						"apiVersion":         {Type: TypeString},
						"kind":               {Type: TypeString},
						"name":               {Type: TypeString},
						"uid":                {Type: TypeString},
						"controller":         {Type: TypeBoolean},
						"blockOwnerDeletion": {Type: TypeBoolean},
					},
				},
			},
			"finalizers": {Type: TypeArray, Items: &PropertySchema{Type: TypeString}},
		},
	}

	containerSchema := &PropertySchema{
		Type:     TypeObject,
		Required: []string{"name"},
		Properties: map[string]*PropertySchema{
			"name":            {Type: TypeString},
			"image":           {Type: TypeString},
			"command":         {Type: TypeArray, Items: &PropertySchema{Type: TypeString}},
			"args":            {Type: TypeArray, Items: &PropertySchema{Type: TypeString}},
			"workingDir":      {Type: TypeString},
			"imagePullPolicy": {Type: TypeString, Enum: []string{"Always", "Never", "IfNotPresent"}},
			"ports": {
				Type: TypeArray,
				Items: &PropertySchema{
					Type:     TypeObject,
					Required: []string{"containerPort"},
					Properties: map[string]*PropertySchema{
						"name":          {Type: TypeString},
						"containerPort": {Type: TypeInteger},
						"hostPort":      {Type: TypeInteger},
						"protocol":      {Type: TypeString, Enum: []string{"TCP", "UDP", "SCTP"}},
						"hostIP":        {Type: TypeString},
					},
				},
			},
			"env": {
				Type: TypeArray,
				Items: &PropertySchema{
					Type:     TypeObject,
					Required: []string{"name"},
					Properties: map[string]*PropertySchema{
						"name":      {Type: TypeString},
						"value":     {Type: TypeString},
						"valueFrom": {Type: TypeObject, AdditionalProperties: true},
					},
				},
			},
			"envFrom": {
				Type: TypeArray,
				Items: &PropertySchema{
					Type: TypeObject,
					Properties: map[string]*PropertySchema{
						"prefix":       {Type: TypeString},
						"configMapRef": {Type: TypeObject, Required: []string{"name"}, Properties: map[string]*PropertySchema{"name": {Type: TypeString}, "optional": {Type: TypeBoolean}}},
						"secretRef":    {Type: TypeObject, Required: []string{"name"}, Properties: map[string]*PropertySchema{"name": {Type: TypeString}, "optional": {Type: TypeBoolean}}},
					},
				},
			},
			"resources": {
				Type: TypeObject,
				Properties: map[string]*PropertySchema{
					"limits":   {Type: TypeObject, AdditionalProperties: true},
					"requests": {Type: TypeObject, AdditionalProperties: true},
				},
			},
			"volumeMounts": {
				Type: TypeArray,
				Items: &PropertySchema{
					Type:     TypeObject,
					Required: []string{"name", "mountPath"},
					Properties: map[string]*PropertySchema{
						"name":             {Type: TypeString},
						"mountPath":        {Type: TypeString},
						"readOnly":         {Type: TypeBoolean},
						"subPath":          {Type: TypeString},
						"mountPropagation": {Type: TypeString},
					},
				},
			},
			"livenessProbe":  probeSchema(),
			"readinessProbe": probeSchema(),
			"startupProbe":   probeSchema(),
			"securityContext": {
				Type: TypeObject,
				Properties: map[string]*PropertySchema{
					"privileged":               {Type: TypeBoolean},
					"runAsUser":                {Type: TypeInteger},
					"runAsGroup":               {Type: TypeInteger},
					"runAsNonRoot":             {Type: TypeBoolean},
					"readOnlyRootFilesystem":   {Type: TypeBoolean},
					"allowPrivilegeEscalation": {Type: TypeBoolean},
					"capabilities": {
						Type: TypeObject,
						Properties: map[string]*PropertySchema{
							"add":  {Type: TypeArray, Items: &PropertySchema{Type: TypeString}},
							"drop": {Type: TypeArray, Items: &PropertySchema{Type: TypeString}},
						},
					},
					"seccompProfile": {
						Type: TypeObject,
						Properties: map[string]*PropertySchema{
							"type":             {Type: TypeString, Enum: []string{"RuntimeDefault", "Localhost", "Unconfined"}},
							"localhostProfile": {Type: TypeString},
						},
					},
				},
			},
		},
	}

	podSpecSchema := &PropertySchema{
		Type:     TypeObject,
		Required: []string{"containers"},
		Properties: map[string]*PropertySchema{
			"containers":                    {Type: TypeArray, Items: containerSchema},
			"initContainers":                {Type: TypeArray, Items: containerSchema},
			"ephemeralContainers":           {Type: TypeArray, Items: containerSchema},
			"restartPolicy":                 {Type: TypeString, Enum: []string{"Always", "OnFailure", "Never"}},
			"terminationGracePeriodSeconds": {Type: TypeInteger},
			"activeDeadlineSeconds":         {Type: TypeInteger},
			"dnsPolicy":                     {Type: TypeString, Enum: []string{"ClusterFirstWithHostNet", "ClusterFirst", "Default", "None"}},
			"nodeSelector":                  {Type: TypeObject, AdditionalProperties: true},
			"serviceAccountName":            {Type: TypeString},
			"serviceAccount":                {Type: TypeString},
			"automountServiceAccountToken":  {Type: TypeBoolean},
			"hostNetwork":                   {Type: TypeBoolean},
			"hostPID":                       {Type: TypeBoolean},
			"hostIPC":                       {Type: TypeBoolean},
			"priorityClassName":             {Type: TypeString},
			"volumes": {
				Type: TypeArray,
				Items: &PropertySchema{
					Type:     TypeObject,
					Required: []string{"name"},
					Properties: map[string]*PropertySchema{
						"name":                  {Type: TypeString},
						"configMap":             {Type: TypeObject, AdditionalProperties: true},
						"secret":                {Type: TypeObject, AdditionalProperties: true},
						"emptyDir":              {Type: TypeObject, AdditionalProperties: true},
						"persistentVolumeClaim": {Type: TypeObject, Required: []string{"claimName"}, Properties: map[string]*PropertySchema{"claimName": {Type: TypeString}, "readOnly": {Type: TypeBoolean}}},
						"hostPath":              {Type: TypeObject, Required: []string{"path"}, Properties: map[string]*PropertySchema{"path": {Type: TypeString}, "type": {Type: TypeString}}},
					},
				},
			},
			"securityContext": {
				Type: TypeObject,
				Properties: map[string]*PropertySchema{
					"runAsUser":          {Type: TypeInteger},
					"runAsGroup":         {Type: TypeInteger},
					"runAsNonRoot":       {Type: TypeBoolean},
					"fsGroup":            {Type: TypeInteger},
					"supplementalGroups": {Type: TypeArray, Items: &PropertySchema{Type: TypeInteger}},
					"seccompProfile": {
						Type: TypeObject,
						Properties: map[string]*PropertySchema{
							"type":             {Type: TypeString, Enum: []string{"RuntimeDefault", "Localhost", "Unconfined"}},
							"localhostProfile": {Type: TypeString},
						},
					},
				},
			},
		},
	}

	podTemplateSchema := &PropertySchema{
		Type:     TypeObject,
		Required: []string{"spec"},
		Properties: map[string]*PropertySchema{
			"metadata": metaSchema,
			"spec":     podSpecSchema,
		},
	}

	labelSelectorSchema := &PropertySchema{
		Type: TypeObject,
		Properties: map[string]*PropertySchema{
			"matchLabels": {Type: TypeObject, AdditionalProperties: true},
			"matchExpressions": {
				Type: TypeArray,
				Items: &PropertySchema{
					Type:     TypeObject,
					Required: []string{"key", "operator"},
					Properties: map[string]*PropertySchema{
						"key":      {Type: TypeString},
						"operator": {Type: TypeString, Enum: []string{"In", "NotIn", "Exists", "DoesNotExist"}},
						"values":   {Type: TypeArray, Items: &PropertySchema{Type: TypeString}},
					},
				},
			},
		},
	}

	// 1. Pod (v1)
	c.Register(&ResourceSchema{
		Group: "", Version: "v1", Kind: "Pod",
		Root: &PropertySchema{
			Type:     TypeObject,
			Required: []string{"apiVersion", "kind", "metadata", "spec"},
			Properties: map[string]*PropertySchema{
				"apiVersion": {Type: TypeString},
				"kind":       {Type: TypeString},
				"metadata":   metaSchema,
				"spec":       podSpecSchema,
			},
		},
	})

	// 2. Service (v1)
	c.Register(&ResourceSchema{
		Group: "", Version: "v1", Kind: "Service",
		Root: &PropertySchema{
			Type:     TypeObject,
			Required: []string{"apiVersion", "kind", "metadata"},
			Properties: map[string]*PropertySchema{
				"apiVersion": {Type: TypeString},
				"kind":       {Type: TypeString},
				"metadata":   metaSchema,
				"spec": {
					Type: TypeObject,
					Properties: map[string]*PropertySchema{
						"type":                     {Type: TypeString, Enum: []string{"ClusterIP", "NodePort", "LoadBalancer", "ExternalName"}},
						"selector":                 {Type: TypeObject, AdditionalProperties: true},
						"clusterIP":                {Type: TypeString},
						"externalIPs":              {Type: TypeArray, Items: &PropertySchema{Type: TypeString}},
						"loadBalancerIP":           {Type: TypeString},
						"sessionAffinity":          {Type: TypeString, Enum: []string{"ClientIP", "None"}},
						"publishNotReadyAddresses": {Type: TypeBoolean},
						"ports": {
							Type: TypeArray,
							Items: &PropertySchema{
								Type:     TypeObject,
								Required: []string{"port"},
								Properties: map[string]*PropertySchema{
									"name":        {Type: TypeString},
									"protocol":    {Type: TypeString, Enum: []string{"TCP", "UDP", "SCTP"}},
									"port":        {Type: TypeInteger},
									"targetPort":  {Type: TypeAny}, // can be int or string
									"nodePort":    {Type: TypeInteger},
									"appProtocol": {Type: TypeString},
								},
							},
						},
					},
				},
			},
		},
	})

	// 3. ConfigMap (v1)
	c.Register(&ResourceSchema{
		Group: "", Version: "v1", Kind: "ConfigMap",
		Root: &PropertySchema{
			Type:     TypeObject,
			Required: []string{"apiVersion", "kind", "metadata"},
			Properties: map[string]*PropertySchema{
				"apiVersion": {Type: TypeString},
				"kind":       {Type: TypeString},
				"metadata":   metaSchema,
				"data":       {Type: TypeObject, AdditionalProperties: true},
				"binaryData": {Type: TypeObject, AdditionalProperties: true},
				"immutable":  {Type: TypeBoolean},
			},
		},
	})

	// 4. Secret (v1)
	c.Register(&ResourceSchema{
		Group: "", Version: "v1", Kind: "Secret",
		Root: &PropertySchema{
			Type:     TypeObject,
			Required: []string{"apiVersion", "kind", "metadata"},
			Properties: map[string]*PropertySchema{
				"apiVersion": {Type: TypeString},
				"kind":       {Type: TypeString},
				"metadata":   metaSchema,
				"data":       {Type: TypeObject, AdditionalProperties: true},
				"stringData": {Type: TypeObject, AdditionalProperties: true},
				"type":       {Type: TypeString},
				"immutable":  {Type: TypeBoolean},
			},
		},
	})

	// 5. Namespace (v1)
	c.Register(&ResourceSchema{
		Group: "", Version: "v1", Kind: "Namespace", ClusterWide: true,
		Root: &PropertySchema{
			Type:     TypeObject,
			Required: []string{"apiVersion", "kind", "metadata"},
			Properties: map[string]*PropertySchema{
				"apiVersion": {Type: TypeString},
				"kind":       {Type: TypeString},
				"metadata":   metaSchema,
				"spec":       {Type: TypeObject, Properties: map[string]*PropertySchema{"finalizers": {Type: TypeArray, Items: &PropertySchema{Type: TypeString}}}},
			},
		},
	})

	// 6. Deployment (apps/v1)
	c.Register(&ResourceSchema{
		Group: "apps", Version: "v1", Kind: "Deployment",
		Root: &PropertySchema{
			Type:     TypeObject,
			Required: []string{"apiVersion", "kind", "metadata", "spec"},
			Properties: map[string]*PropertySchema{
				"apiVersion": {Type: TypeString},
				"kind":       {Type: TypeString},
				"metadata":   metaSchema,
				"spec": {
					Type:     TypeObject,
					Required: []string{"selector", "template"},
					Properties: map[string]*PropertySchema{
						"replicas":                {Type: TypeInteger},
						"selector":                labelSelectorSchema,
						"template":                podTemplateSchema,
						"minReadySeconds":         {Type: TypeInteger},
						"revisionHistoryLimit":    {Type: TypeInteger},
						"paused":                  {Type: TypeBoolean},
						"progressDeadlineSeconds": {Type: TypeInteger},
						"strategy": {
							Type: TypeObject,
							Properties: map[string]*PropertySchema{
								"type":          {Type: TypeString, Enum: []string{"Recreate", "RollingUpdate"}},
								"rollingUpdate": {Type: TypeObject, AdditionalProperties: true},
							},
						},
					},
				},
			},
		},
	})

	// 7. StatefulSet (apps/v1)
	c.Register(&ResourceSchema{
		Group: "apps", Version: "v1", Kind: "StatefulSet",
		Root: &PropertySchema{
			Type:     TypeObject,
			Required: []string{"apiVersion", "kind", "metadata", "spec"},
			Properties: map[string]*PropertySchema{
				"apiVersion": {Type: TypeString},
				"kind":       {Type: TypeString},
				"metadata":   metaSchema,
				"spec": {
					Type:     TypeObject,
					Required: []string{"selector", "template", "serviceName"},
					Properties: map[string]*PropertySchema{
						"replicas":             {Type: TypeInteger},
						"serviceName":          {Type: TypeString},
						"selector":             labelSelectorSchema,
						"template":             podTemplateSchema,
						"podManagementPolicy":  {Type: TypeString, Enum: []string{"OrderedReady", "Parallel"}},
						"volumeClaimTemplates": {Type: TypeArray, Items: &PropertySchema{Type: TypeObject, AdditionalProperties: true}},
						"minReadySeconds":      {Type: TypeInteger},
					},
				},
			},
		},
	})

	// 8. DaemonSet (apps/v1)
	c.Register(&ResourceSchema{
		Group: "apps", Version: "v1", Kind: "DaemonSet",
		Root: &PropertySchema{
			Type:     TypeObject,
			Required: []string{"apiVersion", "kind", "metadata", "spec"},
			Properties: map[string]*PropertySchema{
				"apiVersion": {Type: TypeString},
				"kind":       {Type: TypeString},
				"metadata":   metaSchema,
				"spec": {
					Type:     TypeObject,
					Required: []string{"selector", "template"},
					Properties: map[string]*PropertySchema{
						"selector":             labelSelectorSchema,
						"template":             podTemplateSchema,
						"minReadySeconds":      {Type: TypeInteger},
						"revisionHistoryLimit": {Type: TypeInteger},
					},
				},
			},
		},
	})

	// 9. Job (batch/v1)
	c.Register(&ResourceSchema{
		Group: "batch", Version: "v1", Kind: "Job",
		Root: &PropertySchema{
			Type:     TypeObject,
			Required: []string{"apiVersion", "kind", "metadata", "spec"},
			Properties: map[string]*PropertySchema{
				"apiVersion": {Type: TypeString},
				"kind":       {Type: TypeString},
				"metadata":   metaSchema,
				"spec": {
					Type:     TypeObject,
					Required: []string{"template"},
					Properties: map[string]*PropertySchema{
						"parallelism":             {Type: TypeInteger},
						"completions":             {Type: TypeInteger},
						"activeDeadlineSeconds":   {Type: TypeInteger},
						"backoffLimit":            {Type: TypeInteger},
						"selector":                labelSelectorSchema,
						"template":                podTemplateSchema,
						"ttlSecondsAfterFinished": {Type: TypeInteger},
						"suspend":                 {Type: TypeBoolean},
					},
				},
			},
		},
	})

	// 10. CronJob (batch/v1)
	c.Register(&ResourceSchema{
		Group: "batch", Version: "v1", Kind: "CronJob",
		Root: &PropertySchema{
			Type:     TypeObject,
			Required: []string{"apiVersion", "kind", "metadata", "spec"},
			Properties: map[string]*PropertySchema{
				"apiVersion": {Type: TypeString},
				"kind":       {Type: TypeString},
				"metadata":   metaSchema,
				"spec": {
					Type:     TypeObject,
					Required: []string{"schedule", "jobTemplate"},
					Properties: map[string]*PropertySchema{
						"schedule":                   {Type: TypeString},
						"timeZone":                   {Type: TypeString},
						"startingDeadlineSeconds":    {Type: TypeInteger},
						"concurrencyPolicy":          {Type: TypeString, Enum: []string{"Allow", "Forbid", "Replace"}},
						"suspend":                    {Type: TypeBoolean},
						"successfulJobsHistoryLimit": {Type: TypeInteger},
						"failedJobsHistoryLimit":     {Type: TypeInteger},
						"jobTemplate": {
							Type:     TypeObject,
							Required: []string{"spec"},
							Properties: map[string]*PropertySchema{
								"metadata": metaSchema,
								"spec": {
									Type:     TypeObject,
									Required: []string{"template"},
									Properties: map[string]*PropertySchema{
										"template": podTemplateSchema,
									},
								},
							},
						},
					},
				},
			},
		},
	})

	// 11. Ingress (networking.k8s.io/v1)
	c.Register(&ResourceSchema{
		Group: "networking.k8s.io", Version: "v1", Kind: "Ingress",
		Root: &PropertySchema{
			Type:     TypeObject,
			Required: []string{"apiVersion", "kind", "metadata"},
			Properties: map[string]*PropertySchema{
				"apiVersion": {Type: TypeString},
				"kind":       {Type: TypeString},
				"metadata":   metaSchema,
				"spec": {
					Type: TypeObject,
					Properties: map[string]*PropertySchema{
						"ingressClassName": {Type: TypeString},
						"defaultBackend":   {Type: TypeObject, AdditionalProperties: true},
						"tls": {
							Type: TypeArray,
							Items: &PropertySchema{
								Type: TypeObject,
								Properties: map[string]*PropertySchema{
									"hosts":      {Type: TypeArray, Items: &PropertySchema{Type: TypeString}},
									"secretName": {Type: TypeString},
								},
							},
						},
						"rules": {
							Type: TypeArray,
							Items: &PropertySchema{
								Type: TypeObject,
								Properties: map[string]*PropertySchema{
									"host": {Type: TypeString},
									"http": {
										Type:     TypeObject,
										Required: []string{"paths"},
										Properties: map[string]*PropertySchema{
											"paths": {
												Type: TypeArray,
												Items: &PropertySchema{
													Type:     TypeObject,
													Required: []string{"pathType", "backend"},
													Properties: map[string]*PropertySchema{
														"path":     {Type: TypeString},
														"pathType": {Type: TypeString, Enum: []string{"Exact", "Prefix", "ImplementationSpecific"}},
														"backend":  {Type: TypeObject, AdditionalProperties: true},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
}

func probeSchema() *PropertySchema {
	return &PropertySchema{
		Type: TypeObject,
		Properties: map[string]*PropertySchema{
			"initialDelaySeconds":           {Type: TypeInteger},
			"timeoutSeconds":                {Type: TypeInteger},
			"periodSeconds":                 {Type: TypeInteger},
			"successThreshold":              {Type: TypeInteger},
			"failureThreshold":              {Type: TypeInteger},
			"terminationGracePeriodSeconds": {Type: TypeInteger},
			"exec": {
				Type:     TypeObject,
				Required: []string{"command"},
				Properties: map[string]*PropertySchema{
					"command": {Type: TypeArray, Items: &PropertySchema{Type: TypeString}},
				},
			},
			"httpGet": {
				Type:     TypeObject,
				Required: []string{"port"},
				Properties: map[string]*PropertySchema{
					"path":        {Type: TypeString},
					"port":        {Type: TypeAny}, // int or string
					"host":        {Type: TypeString},
					"scheme":      {Type: TypeString, Enum: []string{"HTTP", "HTTPS"}},
					"httpHeaders": {Type: TypeArray, Items: &PropertySchema{Type: TypeObject, Required: []string{"name", "value"}, Properties: map[string]*PropertySchema{"name": {Type: TypeString}, "value": {Type: TypeString}}}},
				},
			},
			"tcpSocket": {
				Type:     TypeObject,
				Required: []string{"port"},
				Properties: map[string]*PropertySchema{
					"port": {Type: TypeAny},
					"host": {Type: TypeString},
				},
			},
			"grpc": {
				Type:     TypeObject,
				Required: []string{"port"},
				Properties: map[string]*PropertySchema{
					"port":    {Type: TypeInteger},
					"service": {Type: TypeString},
				},
			},
		},
	}
}
