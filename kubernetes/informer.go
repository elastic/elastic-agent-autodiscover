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

package kubernetes

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func nodeSelector(options *metav1.ListOptions, opt WatchOptions) {
	if opt.Node != "" {
		options.FieldSelector = "spec.nodeName=" + opt.Node
	}
}

func nameSelector(options *metav1.ListOptions, name string) {
	if name != "" {
		options.FieldSelector = "metadata.name=" + name
	}
}

// NewInformer creates an informer for a given resource
func NewInformer(client kubernetes.Interface, resource Resource, opts WatchOptions, indexers cache.Indexers) (cache.SharedInformer, string, error) {
	var objType string

	var listwatch *cache.ListWatch
	ctx := context.TODO()
	switch resource.(type) {
	case *Pod:
		p := client.CoreV1().Pods(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				nodeSelector(&options, opts)
				return p.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				nodeSelector(&options, opts)
				return p.Watch(ctx, options)
			},
		}

		objType = "pod"
	case *Event:
		e := client.CoreV1().Events(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return e.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return e.Watch(ctx, options)
			},
		}

		objType = "event"
	case *Node:
		n := client.CoreV1().Nodes()
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				nameSelector(&options, opts.Node)
				return n.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				nameSelector(&options, opts.Node)
				return n.Watch(ctx, options)
			},
		}

		objType = "node"
	case *Namespace:
		ns := client.CoreV1().Namespaces()
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				nameSelector(&options, opts.Namespace)
				return ns.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				nameSelector(&options, opts.Namespace)
				return ns.Watch(ctx, options)
			},
		}

		objType = "namespace"
	case *Deployment:
		d := client.AppsV1().Deployments(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return d.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return d.Watch(ctx, options)
			},
		}

		objType = "deployment"
	case *ReplicaSet:
		rs := client.AppsV1().ReplicaSets(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return rs.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return rs.Watch(ctx, options)
			},
		}

		objType = "replicaset"
	case *StatefulSet:
		ss := client.AppsV1().StatefulSets(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return ss.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return ss.Watch(ctx, options)
			},
		}

		objType = "statefulset"
	case *DaemonSet:
		ss := client.AppsV1().DaemonSets(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return ss.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return ss.Watch(ctx, options)
			},
		}

		objType = "daemonset"
	case *Service:
		svc := client.CoreV1().Services(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return svc.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return svc.Watch(ctx, options)
			},
		}

		objType = "service"
	case *ServiceAccount:
		sa := client.CoreV1().ServiceAccounts(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return sa.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return sa.Watch(ctx, options)
			},
		}

		objType = "serviceAccount"
	case *CronJob:
		cronjob := client.BatchV1().CronJobs(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return cronjob.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return cronjob.Watch(ctx, options)
			},
		}

		objType = "cronjob"
	case *Job:
		job := client.BatchV1().Jobs(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return job.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return job.Watch(ctx, options)
			},
		}

		objType = "job"
	case *PersistentVolume:
		ss := client.CoreV1().PersistentVolumes()
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return ss.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return ss.Watch(ctx, options)
			},
		}

		objType = "persistentvolume"
	case *PersistentVolumeClaim:
		ss := client.CoreV1().PersistentVolumeClaims(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return ss.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return ss.Watch(ctx, options)
			},
		}

		objType = "persistentvolumeclaim"
	case *StorageClass:
		sc := client.StorageV1().StorageClasses()
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return sc.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return sc.Watch(ctx, options)
			},
		}

		objType = "storageclass"
	case *Role:
		r := client.RbacV1().Roles(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return r.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return r.Watch(ctx, options)
			},
		}

		objType = "role"

	case *RoleBinding:
		rb := client.RbacV1().RoleBindings(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return rb.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return rb.Watch(ctx, options)
			},
		}

		objType = "rolebinding"

	case *ClusterRole:
		cr := client.RbacV1().ClusterRoles()
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return cr.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return cr.Watch(ctx, options)
			},
		}

		objType = "clusterrole"

	case *ClusterRoleBinding:
		crb := client.RbacV1().ClusterRoleBindings()
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return crb.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return crb.Watch(ctx, options)
			},
		}

		objType = "clusterrolebinding"

	case *NetworkPolicy:
		np := client.ExtensionsV1beta1().NetworkPolicies(opts.Namespace)
		listwatch = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return np.List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return np.Watch(ctx, options)
			},
		}

		objType = "networkpolicy"

	default:
		return nil, "", fmt.Errorf("unsupported resource type for watching %T", resource)
	}

	if indexers == nil {
		indexers = cache.Indexers{}
	}
	return cache.NewSharedIndexInformer(listwatch, resource, opts.SyncTimeout, indexers), objType, nil
}

// NewMetadataInformer creates an informer for a given resource that only tracks the resource metadata.
func NewMetadataInformer(client metadata.Interface, gvr schema.GroupVersionResource, opts WatchOptions, indexers cache.Indexers) cache.SharedInformer {
	ctx := context.Background()
	if indexers == nil {
		indexers = cache.Indexers{}
	}
	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.Resource(gvr).List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return client.Resource(gvr).Watch(ctx, options)
			},
		},
		&metav1.PartialObjectMetadata{},
		opts.SyncTimeout,
		indexers,
	)
	return informer
}
