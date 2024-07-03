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
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	k8s "k8s.io/client-go/kubernetes"

	"github.com/elastic/elastic-agent-autodiscover/kubernetes"
	"github.com/elastic/elastic-agent-autodiscover/utils"
	"github.com/elastic/elastic-agent-libs/config"
	"github.com/elastic/elastic-agent-libs/mapstr"
	"github.com/elastic/elastic-agent-libs/safemapstr"
)

const deploymentType = "Deployment"

// Resource generates metadata for any kubernetes resource
type Resource struct {
	config      *Config
	clusterInfo ClusterInfo
	namespace   MetaGen
}

// NewResourceMetadataGenerator creates a metadata generator for a generic resource
func NewResourceMetadataGenerator(cfg *config.C, client k8s.Interface) *Resource {
	var c Config
	err := c.Unmarshal(cfg)
	if err != nil {
		return nil
	}

	r := &Resource{
		config: &c,
	}
	clusterInfo, err := GetKubernetesClusterIdentifier(cfg, client)
	if err == nil {
		r.clusterInfo = clusterInfo
	}
	return r
}

// NewNamespaceAwareResourceMetadataGenerator creates a metadata generator with informatuon about namespace
func NewNamespaceAwareResourceMetadataGenerator(cfg *config.C, client k8s.Interface, namespace MetaGen) *Resource {
	r := NewResourceMetadataGenerator(cfg, client)
	r.namespace = namespace
	return r
}

// Generate generates metadata from a resource object
// Generate method returns metadata in the following form:
//
//	{
//		  "kubernetes": {},
//	   "ecs.a.field": 42,
//	}
//
// This method should be called in top level and not as part of other metadata generators.
// For retrieving metadata without 'kubernetes.' prefix one should call GenerateK8s instead.
func (r *Resource) Generate(kind string, obj kubernetes.Resource, opts ...FieldOptions) mapstr.M {
	ecsFields := r.GenerateECS(obj)
	meta := mapstr.M{
		"kubernetes": r.GenerateK8s(kind, obj, opts...),
	}
	meta.DeepUpdate(ecsFields)
	return meta
}

// GenerateECS generates ECS metadata from a resource object
func (r *Resource) GenerateECS(obj kubernetes.Resource) mapstr.M {
	ecsMeta := mapstr.M{}
	if r.clusterInfo.URL != "" {
		_, _ = ecsMeta.Put("orchestrator.cluster.url", r.clusterInfo.URL)
	}
	if r.clusterInfo.Name != "" {
		_, _ = ecsMeta.Put("orchestrator.cluster.name", r.clusterInfo.Name)
	}
	return ecsMeta
}

// GenerateK8s takes a kind and an object and creates metadata for the same
func (r *Resource) GenerateK8s(kind string, obj kubernetes.Resource, options ...FieldOptions) mapstr.M {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil
	}

	var labelMap mapstr.M
	if len(r.config.IncludeLabels) == 0 {
		labelMap = GenerateMap(accessor.GetLabels(), r.config.LabelsDedot)
	} else {
		labelMap = generateMapSubset(accessor.GetLabels(), r.config.IncludeLabels, r.config.LabelsDedot, r.config.UseRegexInclude)
	}

	var labelMaptoExclude mapstr.M
	if len(r.config.ExcludeLabels) != 0 {
		labelMaptoExclude = generateMapSubset(accessor.GetLabels(), r.config.ExcludeLabels, r.config.LabelsDedot, r.config.UseRegexExclude)
	}

	// Exclude any labels that are present in the exclude_labels config
	for label := range labelMaptoExclude {
		_ = labelMap.Delete(label)
	}

	annotationsMap := generateMapSubset(accessor.GetAnnotations(), r.config.IncludeAnnotations, r.config.AnnotationsDedot, r.config.UseRegexInclude)

	meta := mapstr.M{
		strings.ToLower(kind): mapstr.M{
			"name": accessor.GetName(),
			"uid":  string(accessor.GetUID()),
		},
	}

	namespaceName := accessor.GetNamespace()
	if namespaceName != "" {
		_ = safemapstr.Put(meta, "namespace", namespaceName)

		if r.namespace != nil {
			nsMeta := r.namespace.GenerateFromName(namespaceName)
			if nsMeta != nil {
				meta.DeepUpdate(nsMeta)
			}
		}
	}

	// Add controller metadata if present
	for _, ref := range accessor.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller {
			switch ref.Kind {
			// grow this list as we keep adding more `state_*` metricsets
			case deploymentType,
				"ReplicaSet",
				"StatefulSet",
				"DaemonSet",
				"Job",
				"CronJob":
				_ = safemapstr.Put(meta, strings.ToLower(ref.Kind)+".name", ref.Name)
			}
		}
	}

	if len(labelMap) != 0 {
		_ = safemapstr.Put(meta, "labels", labelMap)
	}

	if len(annotationsMap) != 0 {
		_ = safemapstr.Put(meta, "annotations", annotationsMap)
	}

	for _, option := range options {
		option(meta)
	}

	return meta
}

func generateMapSubset(input map[string]string, keys []string, dedot bool, useregex bool) mapstr.M {
	output := mapstr.M{}
	if input == nil {
		return output
	}

	for _, key := range keys {
		//This is the part where use_regex is enabled
		if useregex {
			pattern, err := regexp.Compile(key)
			if err == nil {
				for label, value := range input {
					matched := pattern.MatchString(label)
					if matched {
						if dedot {
							dedotKey := utils.DeDot(label)
							_, _ = output.Put(dedotKey, value)
						} else {
							_ = safemapstr.Put(output, label, value)
						}
					}

				}
			}
		} else {
			value, ok := input[key]
			if ok {
				if dedot {
					dedotKey := utils.DeDot(key)
					_, _ = output.Put(dedotKey, value)
				} else {
					_ = safemapstr.Put(output, key, value)
				}
			}
		}
	}
	return output
}

func GenerateMap(input map[string]string, dedot bool) mapstr.M {
	output := mapstr.M{}
	if input == nil {
		return output
	}

	for k, v := range input {
		if dedot {
			label := utils.DeDot(k)
			_, _ = output.Put(label, v)
		} else {
			_ = safemapstr.Put(output, k, v)
		}
	}

	return output
}
