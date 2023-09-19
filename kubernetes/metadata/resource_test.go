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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/elastic/elastic-agent-autodiscover/kubernetes"
	"github.com/elastic/elastic-agent-libs/config"
	"github.com/elastic/elastic-agent-libs/mapstr"
	"github.com/elastic/go-ucfg"
)

const (
	uid       = "005f3b90-4b9d-12f8-acf0-31020a840133"
	defaultNs = "default"
	name      = "obj"
)

func TestResource_Generate(t *testing.T) {
	boolean := true
	tests := []struct {
		input  kubernetes.Resource
		output mapstr.M
		name   string
	}{
		{
			name: "test simple object",
			input: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					UID:       types.UID(uid),
					Namespace: defaultNs,
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
			},
			output: mapstr.M{
				"kubernetes": mapstr.M{
					"pod": mapstr.M{
						"name": name,
						"uid":  uid,
					},
					"labels": mapstr.M{
						"foo": "bar",
					},
					"namespace": defaultNs,
				},
			},
		},
		{
			name: "test object with owner reference",
			input: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					UID:       types.UID(uid),
					Namespace: defaultNs,
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps",
							Kind:       "Deployment",
							Name:       "owner",
							UID:        "005f3b90-4b9d-12f8-acf0-31020a840144",
							Controller: &boolean,
						},
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
			},
			output: mapstr.M{
				"kubernetes": mapstr.M{
					"pod": mapstr.M{
						"name": name,
						"uid":  uid,
					},
					"labels": mapstr.M{
						"foo": "bar",
					},
					"namespace": defaultNs,
					"deployment": mapstr.M{
						"name": "owner",
					},
				},
			},
		},
	}

	var cfg Config
	err := ucfg.New().Unpack(&cfg)
	require.NoError(t, err)
	metagen := &Resource{
		config: &cfg,
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.output, metagen.Generate("pod", test.input))
		})
	}
}

func TestNamespaceAwareResource_GenerateWithNamespace(t *testing.T) {
	client := k8sfake.NewSimpleClientset()
	tests := []struct {
		resourceName string
		input        kubernetes.Resource
		namespace    kubernetes.Resource
		output       mapstr.M
		name         string
	}{
		{
			name:         "test not namespaced kubernetes resource - PersistentVolume",
			resourceName: "persistentvolume",
			input: &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pvc-18705cfb-9fb8-441f-9b32-0d67a21af839",
					UID:  "020fd954-3674-496a-9e77-c25f0f2257ea",
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "PersistentVolume",
					APIVersion: "v1",
				},
			},
			namespace: &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: defaultNs,
					UID:  types.UID(uid),
					Labels: map[string]string{
						"nskey": "nsvalue",
					},
					Annotations: map[string]string{
						"ns.annotation": "value",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Namespace",
					APIVersion: "v1",
				},
			},
			output: mapstr.M{
				"kubernetes": mapstr.M{
					"persistentvolume": mapstr.M{
						"name": "pvc-18705cfb-9fb8-441f-9b32-0d67a21af839",
						"uid":  "020fd954-3674-496a-9e77-c25f0f2257ea",
					},
					"labels": mapstr.M{
						"foo": "bar",
					},
				},
			},
		},
		{
			name:         "test namespaced kubernetes resource",
			resourceName: "deployment",
			input: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: "default",
					UID:       "f33ca314-8cc5-48ea-90b7-3102c7430f75",
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
			},
			namespace: &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: defaultNs,
					UID:  types.UID(uid),
					Labels: map[string]string{
						"nskey": "nsvalue",
					},
					Annotations: map[string]string{
						"ns.annotation": "value",
					},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Namespace",
					APIVersion: "v1",
				},
			},
			output: mapstr.M{
				"kubernetes": mapstr.M{
					"deployment": mapstr.M{
						"name": name,
						"uid":  "f33ca314-8cc5-48ea-90b7-3102c7430f75",
					},
					"labels": mapstr.M{
						"foo": "bar",
					},
					"namespace":     "default",
					"namespace_uid": uid,
					"namespace_labels": mapstr.M{
						"nskey": "nsvalue",
					},
					"namespace_annotations": mapstr.M{
						"ns_annotation": "value",
					},
				},
			},
		},
	}

	for _, test := range tests {
		nsConfig, err := config.NewConfigFrom(map[string]interface{}{
			"include_annotations": []string{"ns.annotation"},
		})
		require.NoError(t, err)

		namespaces := cache.NewStore(cache.MetaNamespaceKeyFunc)
		err = namespaces.Add(test.namespace)
		require.NoError(t, err)
		nsMeta := NewNamespaceMetadataGenerator(nsConfig, namespaces, client)

		metagen := NewNamespaceAwareResourceMetadataGenerator(nsConfig, client, nsMeta)
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.output, metagen.Generate(test.resourceName, test.input))
		})
	}
}

func Test_generateMapSubset(t *testing.T) {

	Labelsdedot := map[string]string{
		"app.kubernetes.io/name": "no",
		"foo":                    "bar",
	}

	Labels := map[string]string{
		"nottomatch":       "no",
		"foo":              "bar",
		"foo1":             "bar1",
		"foo2":             "bar2",
		"foo-example":      "bar-example",
		"test":             "test1",
		"footest":          "footest1",
		"simplefo-example": "simplefo-example1",
	}
	//Validating wildcards
	key1 := []string{
		"foo*",
		"test",
	}
	expectedresult1 := mapstr.M{
		"foo":              "bar",
		"foo1":             "bar1",
		"foo2":             "bar2",
		"foo-example":      "bar-example",
		"test":             "test1",
		"footest":          "footest1",
		"simplefo-example": "simplefo-example1",
	}

	output1 := generateMapSubset(Labels, key1, false)

	//Validating ?
	key2 := []string{
		"0?1",
	}
	expectedresult2 := mapstr.M{
		"foo1": "bar1",
	}
	output2 := generateMapSubset(Labels, key2, false)

	//Validating start of a string
	key3 := []string{
		"^test",
	}
	expectedresult3 := mapstr.M{
		"test": "test1",
	}
	output3 := generateMapSubset(Labels, key3, false)

	//Validating end of a string
	key4 := []string{
		"test$",
	}
	expectedresult4 := mapstr.M{
		"test":    "test1",
		"footest": "footest1",
	}
	output4 := generateMapSubset(Labels, key4, false)

	//Exact matches
	key5 := []string{
		"t{2}",
	}
	expectedresult5 := mapstr.M{
		"nottomatch": "no",
	}
	output5 := generateMapSubset(Labels, key5, false)

	assert.Equal(t, expectedresult1, output1)
	assert.Equal(t, expectedresult2, output2)
	assert.Equal(t, expectedresult3, output3)
	assert.Equal(t, expectedresult4, output4)
	assert.Equal(t, expectedresult5, output5)

	//Dedot Validation
	key6 := []string{
		"app.kubernetes.io",
	}
	expectedresult6 := mapstr.M{
		"app_kubernetes_io/name": "no",
	}
	output6 := generateMapSubset(Labelsdedot, key6, true)
	assert.Equal(t, expectedresult6, output6)

}
