package store

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/postrender"
)

var _ postrender.PostRenderer = localImagePullPolicyPostRenderer{}

// localImagePullPolicyPostRenderer only fills an omitted policy for images
// whose Kubernetes default would otherwise be Always. Explicit chart values
// remain untouched so chart authors retain control over update semantics.
type localImagePullPolicyPostRenderer struct{}

func (localImagePullPolicyPostRenderer) Run(rendered *bytes.Buffer) (*bytes.Buffer, error) {
	decoder := yaml.NewDecoder(rendered)
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	defer encoder.Close()

	for {
		var document yaml.Node
		if err := decoder.Decode(&document); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode Helm manifest: %w", err)
		}
		if yamlDocumentRoot(&document) == nil {
			continue
		}
		patchPullPolicies(&document)
		if err := encoder.Encode(&document); err != nil {
			return nil, fmt.Errorf("encode Helm manifest: %w", err)
		}
	}
	return &output, nil
}

func patchPullPolicies(document *yaml.Node) {
	root := yamlDocumentRoot(document)
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	if scalarValue(mappingValue(root, "kind")) == "List" {
		items := mappingValue(root, "items")
		if items == nil || items.Kind != yaml.SequenceNode {
			return
		}
		for _, item := range items.Content {
			patchPullPolicies(item)
		}
		return
	}

	var podSpec *yaml.Node
	switch scalarValue(mappingValue(root, "kind")) {
	case "Pod":
		podSpec = nestedMapping(root, "spec")
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "ReplicationController", "Job":
		podSpec = nestedMapping(root, "spec", "template", "spec")
	case "CronJob":
		podSpec = nestedMapping(root, "spec", "jobTemplate", "spec", "template", "spec")
	default:
		return
	}
	if podSpec == nil {
		return
	}
	patchContainerList(podSpec, "initContainers")
	patchContainerList(podSpec, "containers")
	patchContainerList(podSpec, "ephemeralContainers")
}

func yamlDocumentRoot(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		return node.Content[0]
	}
	return node
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func nestedMapping(node *yaml.Node, path ...string) *yaml.Node {
	current := node
	for _, key := range path {
		current = mappingValue(current, key)
		if current == nil || current.Kind != yaml.MappingNode {
			return nil
		}
	}
	return current
}

func scalarValue(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

func patchContainerList(podSpec *yaml.Node, key string) {
	containers := mappingValue(podSpec, key)
	if containers == nil || containers.Kind != yaml.SequenceNode {
		return
	}
	for _, container := range containers.Content {
		if container.Kind != yaml.MappingNode || mappingValue(container, "imagePullPolicy") != nil {
			continue
		}
		image := scalarValue(mappingValue(container, "image"))
		if image == "" || !usesImplicitLatest(image) {
			continue
		}
		container.Content = append(container.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "imagePullPolicy"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "IfNotPresent"},
		)
	}
}

func usesImplicitLatest(image string) bool {
	if strings.Contains(image, "@") {
		return false
	}
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	return lastColon <= lastSlash || image[lastColon+1:] == "latest"
}
