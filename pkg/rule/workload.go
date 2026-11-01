package rule

import (
	"github.com/kubecheck/kubecheck/pkg/inventory"
	"github.com/kubecheck/kubecheck/pkg/nir"
)

// ContainerContext pairs an extracted container node with its enclosing pod spec and resource metadata.
type ContainerContext struct {
	Resource      *inventory.Resource
	PodSpecNode   *nir.Node
	ContainerNode *nir.Node
	Name          string
	Image         string
	IsInit        bool
	IsEphemeral   bool
}

// WorkloadKinds lists all Kubernetes standard workload kinds containing pod templates.
var WorkloadKinds = []string{
	"Pod",
	"Deployment",
	"StatefulSet",
	"DaemonSet",
	"ReplicaSet",
	"Job",
	"CronJob",
}

// ExtractPodSpec retrieves the `spec` node containing containers for any supported workload kind.
func ExtractPodSpec(res *inventory.Resource) *nir.Node {
	if res == nil || res.RootNode == nil {
		return nil
	}

	switch res.ID.Kind {
	case "Pod":
		return res.RootNode.Get("spec")
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
		return res.RootNode.Get("spec.template.spec")
	case "CronJob":
		return res.RootNode.Get("spec.jobTemplate.spec.template.spec")
	default:
		return nil
	}
}

// ExtractContainers extracts all standard and init containers from a workload resource.
func ExtractContainers(res *inventory.Resource) []ContainerContext {
	podSpec := ExtractPodSpec(res)
	if podSpec == nil {
		return nil
	}

	var containers []ContainerContext

	// Regular containers
	if cNode := podSpec.Get("containers"); cNode != nil && cNode.Kind == nir.KindSequence {
		for _, c := range cNode.Items {
			name, _ := c.GetString("name")
			image, _ := c.GetString("image")
			containers = append(containers, ContainerContext{
				Resource:      res,
				PodSpecNode:   podSpec,
				ContainerNode: c,
				Name:          name,
				Image:         image,
				IsInit:        false,
			})
		}
	}

	// Init containers
	if initNode := podSpec.Get("initContainers"); initNode != nil && initNode.Kind == nir.KindSequence {
		for _, c := range initNode.Items {
			name, _ := c.GetString("name")
			image, _ := c.GetString("image")
			containers = append(containers, ContainerContext{
				Resource:      res,
				PodSpecNode:   podSpec,
				ContainerNode: c,
				Name:          name,
				Image:         image,
				IsInit:        true,
			})
		}
	}

	return containers
}
