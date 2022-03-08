// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package metadata

import (
	v1 "k8s.io/api/core/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/elastic/elastic-agent-autodiscover/kubernetes"
	"github.com/elastic/elastic-agent-libs/config"
	"github.com/elastic/elastic-agent-libs/mapstr"
)

type node struct {
	store    cache.Store
	resource *Resource
}

// NewNodeMetadataGenerator creates a metagen for service resources
func NewNodeMetadataGenerator(cfg *config.C, nodes cache.Store, client k8s.Interface) MetaGen {
	return &node{
		resource: NewResourceMetadataGenerator(cfg, client),
		store:    nodes,
	}
}

// Generate generates node metadata from a resource object
// Metadata map is in the following form:
// {
// 	  "kubernetes": {},
//    "some.ecs.field": "asdf"
// }
// All Kubernetes fields that need to be stored under kuberentes. prefix are populetad by
// GenerateK8s method while fields that are part of ECS are generated by GenerateECS method
func (n *node) Generate(obj kubernetes.Resource, opts ...FieldOptions) mapstr.M {
	ecsFields := n.GenerateECS(obj)
	meta := mapstr.M{
		"kubernetes": n.GenerateK8s(obj, opts...),
	}
	meta.DeepUpdate(ecsFields)
	return meta
}

// GenerateECS generates node ECS metadata from a resource object
func (n *node) GenerateECS(obj kubernetes.Resource) mapstr.M {
	return n.resource.GenerateECS(obj)
}

// GenerateK8s generates node metadata from a resource object
func (n *node) GenerateK8s(obj kubernetes.Resource, opts ...FieldOptions) mapstr.M {
	node, ok := obj.(*kubernetes.Node)
	if !ok {
		return nil
	}

	meta := n.resource.GenerateK8s("node", obj, opts...)
	// Add extra fields in here if need be
	hostname := getHostName(node)
	if hostname != "" {
		_, _ = meta.Put("node.hostname", hostname)
	}
	return meta
}

// GenerateFromName generates pod metadata from a service name
func (n *node) GenerateFromName(name string, opts ...FieldOptions) mapstr.M {
	if n.store == nil {
		return nil
	}

	if obj, ok, _ := n.store.GetByKey(name); ok {
		no, ok := obj.(*kubernetes.Node)
		if !ok {
			return nil
		}

		return n.GenerateK8s(no, opts...)
	}

	return nil
}

// getHostName returns the HostName address of the node
func getHostName(node *v1.Node) string {
	for _, adr := range node.Status.Addresses {
		if adr.Type == v1.NodeHostName {
			return adr.Address
		}
	}
	return ""
}
