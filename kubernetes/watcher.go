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
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/elastic/elastic-agent-libs/logp"
)

const (
	add    = "add"
	update = "update"
	delete = "delete"
)

var (
	accessor = meta.NewAccessor()
)

// Watcher watches Kubernetes resources events
type Watcher interface {
	// Start watching Kubernetes API for new events after resources were listed
	Start() error

	// Stop watching Kubernetes API for new events
	Stop()

	// AddEventHandler add event handlers for corresponding event type watched
	AddEventHandler(ResourceEventHandler)

	// GetEventHandler returns the event handlers for corresponding event type watched
	GetEventHandler() ResourceEventHandler

	// Store returns the store object for the watcher
	Store() cache.Store

	// Client returns the kubernetes client object used by the watcher
	Client() kubernetes.Interface

	// CachedObject returns the old object before change during the last updated event
	CachedObject() runtime.Object
}

// WatchOptions controls watch behaviors
type WatchOptions struct {
	// SyncTimeout is a timeout for listing historical resources
	SyncTimeout time.Duration
	// Node is used for filtering watched resource to given node, use "" for all nodes
	Node string
	// Namespace is used for filtering watched resource to given namespace, use "" for all namespaces
	Namespace string
	// IsUpdated allows registering a func that allows the invoker of the Watch to decide what amounts to an update
	// vs what does not.
	IsUpdated func(old, new interface{}) bool
	// HonorReSyncs allows resync events to be requeued on the worker
	HonorReSyncs bool
}

type item struct {
	object    interface{}
	objectRaw interface{}
	state     string
}

type watcher struct {
	client       kubernetes.Interface
	informer     cache.SharedInformer
	store        cache.Store
	queue        workqueue.Interface
	ctx          context.Context
	stop         context.CancelFunc
	handler      ResourceEventHandler
	logger       *logp.Logger
	cachedObject runtime.Object
}

// NewWatcher initializes the watcher client to provide a events handler for
// resource from the cluster (filtered to the given node)
// Note: This watcher won't emit workqueue metrics. Use NewNamedWatcher to provide an explicit queue name.
func NewWatcher(client kubernetes.Interface, resource Resource, opts WatchOptions, indexers cache.Indexers, logger *logp.Logger) (Watcher, error) {
	return NewNamedWatcher("", client, resource, opts, indexers, logger)
}

// NewNamedWatcher initializes the watcher client to provide an events handler for
// resource from the cluster (filtered to the given node) and also allows to name the k8s
// client's workqueue that is used by the watcher. Workqueue name is important for exposing workqueue
// metrics, if it is empty, its metrics will not be logged by the k8s client.
func NewNamedWatcher(name string, client kubernetes.Interface, resource Resource, opts WatchOptions, indexers cache.Indexers, logger *logp.Logger) (Watcher, error) {
	informer, _, err := NewInformer(client, resource, opts, indexers)
	if err != nil {
		return nil, err
	}
	return NewNamedWatcherWithInformer(name, client, resource, informer, logger, opts)
}

// NewNamedWatcherWithInformer initializes the watcher client to provide an events handler for
// resource from the cluster (filtered to the given node) and also allows to name the k8s
// client's workqueue that is used by the watcher. Workqueue name is important for exposing workqueue
// metrics, if it is empty, its metrics will not be logged by the k8s client.
// This function requires the underlying informer to be passed by the caller.
func NewNamedWatcherWithInformer(
	name string,
	client kubernetes.Interface,
	resource Resource,
	informer cache.SharedInformer,
	logger *logp.Logger,
	opts WatchOptions,
) (Watcher, error) {
	var store cache.Store
	var queue workqueue.Interface
	var cachedObject runtime.Object

	store = informer.GetStore()
	queue = workqueue.NewNamed(name)

	if opts.IsUpdated == nil {
		opts.IsUpdated = func(o, n interface{}) bool {
			old, _ := accessor.ResourceVersion(o.(runtime.Object))
			new, _ := accessor.ResourceVersion(n.(runtime.Object))

			// Only enqueue changes that have a different resource versions to avoid processing resyncs.
			return old != new
		}
	}

	ctx, cancel := context.WithCancel(context.TODO())
	w := &watcher{
		client:       client,
		informer:     informer,
		store:        store,
		queue:        queue,
		ctx:          ctx,
		cachedObject: cachedObject,
		stop:         cancel,
		logger:       logger.Named("kubernetes"),
		handler:      NoOpEventHandlerFuncs{},
	}

	_, err := w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			w.enqueue(o, add)
		},
		DeleteFunc: func(o interface{}) {
			w.enqueue(o, delete)
		},
		UpdateFunc: func(o, n interface{}) {
			if opts.IsUpdated(o, n) {
				w.enqueue(n, update)
			} else if opts.HonorReSyncs {
				// HonorReSyncs ensure that at the time when the kubernetes client does a "resync", i.e, a full list of all
				// objects we make sure that autodiscover processes them. Why is this necessary? An effective control loop works
				// based on two state changes, a list and a watch. A watch is triggered each time the state of the system changes.
				// However, there is no guarantee that all events from a watch are processed by the receiver. To ensure that missed events
				// are properly handled, a period re-list is done to ensure that every state within the system is effectively handled.
				// In this case, we are making sure that we are enqueueing an "add" event because, an runner that is already in Running
				// state should just be deduped by autodiscover and not stop/started periodically as would be the case with an update.
				w.enqueue(n, add)
			}

			//We check the type of resource and only if it is namespace or node return the cacheObject
			switch resource.(type) {
			case *Namespace:
				w.cacheObject(o)
			case *Node:
				w.cacheObject(o)
			}
		},
	})
	if err != nil {
		return nil, err
	}

	return w, nil
}

// NewMetadataWatcher initializes a metadata-only watcher client to provide an events handler for
// resource from the cluster (filtered to the given node).
// Event handlers defined on this watcher receive PartialObjectMetadata resources.
// Note: This watcher won't emit workqueue metrics. Use NewNamedWatcher to provide an explicit queue name.
func NewMetadataWatcher(
	client kubernetes.Interface,
	metadataClient metadata.Interface,
	gvr schema.GroupVersionResource,
	opts WatchOptions,
	indexers cache.Indexers,
	transformFunc cache.TransformFunc,
	logger *logp.Logger,
) (Watcher, error) {
	return NewNamedMetadataWatcher("", client, metadataClient, gvr, opts, indexers, transformFunc, logger)
}

// NewNamedMetadataWatcher initializes a metadata-only watcher client to provide an events handler for
// resource from the cluster (filtered to the given node) and also allows to name the k8s
// client's workqueue that is used by the watcher. Workqueue name is important for exposing workqueue
// metrics, if it is empty, its metrics will not be logged by the k8s client.
// Event handlers defined on this watcher receive PartialObjectMetadata resources.
func NewNamedMetadataWatcher(
	name string,
	client kubernetes.Interface,
	metadataClient metadata.Interface,
	gvr schema.GroupVersionResource,
	opts WatchOptions,
	indexers cache.Indexers,
	transformFunc cache.TransformFunc,
	logger *logp.Logger,
) (Watcher, error) {
	informer := NewMetadataInformer(metadataClient, gvr, opts, indexers)

	if transformFunc != nil {
		err := informer.SetTransform(transformFunc)
		if err != nil {
			return nil, err
		}
	}

	return NewNamedWatcherWithInformer(name, client, &v1.PartialObjectMetadata{}, informer, logger, opts)
}

// AddEventHandler adds a resource handler to process each request that is coming into the watcher
func (w *watcher) AddEventHandler(h ResourceEventHandler) {
	w.handler = h
}

// GetEventHandler returns the watcher's event handler
func (w *watcher) GetEventHandler() ResourceEventHandler {
	return w.handler
}

// Store returns the store object for the resource that is being watched
func (w *watcher) Store() cache.Store {
	return w.store
}

// Client returns the kubernetes client object used by the watcher
func (w *watcher) Client() kubernetes.Interface {
	return w.client
}

// CachedObject returns the old object in cache during the last updated event
func (w *watcher) CachedObject() runtime.Object {
	return w.cachedObject
}

// Start watching pods
func (w *watcher) Start() error {
	go w.informer.Run(w.ctx.Done())

	if !cache.WaitForCacheSync(w.ctx.Done(), w.informer.HasSynced) {
		return fmt.Errorf("kubernetes informer unable to sync cache")
	}

	w.logger.Debugf("cache sync done")

	// Wrap the process function with wait.Until so that if the controller crashes, it starts up again after a second.
	go wait.Until(func() {
		for w.process(w.ctx) {
		}
	}, time.Second*1, w.ctx.Done())

	return nil
}

func (w *watcher) Stop() {
	w.queue.ShutDown()
	w.stop()
}

// enqueue takes the most recent object that was received, figures out the namespace/name of the object
// and adds it to the work queue for processing.
func (w *watcher) enqueue(obj interface{}, state string) {
	// DeletionHandlingMetaNamespaceKeyFunc that we get a key only if the resource's state is not Unknown.
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	if deleted, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		w.logger.Debugf("Enqueued DeletedFinalStateUnknown contained object: %+v", deleted.Obj)
		obj = deleted.Obj
	}
	w.queue.Add(&item{key, obj, state})
}

// cacheObject updates watcher with the old version of cache objects before change during update events
func (w *watcher) cacheObject(o interface{}) {
	if old, ok := o.(runtime.Object); !ok {
		utilruntime.HandleError(fmt.Errorf("expected object in cache got %#v", o))
	} else {
		w.cachedObject = old
	}
}

// process gets the top of the work queue and processes the object that is received.
func (w *watcher) process(_ context.Context) bool {
	obj, quit := w.queue.Get()
	if quit {
		return false
	}
	defer w.queue.Done(obj)

	var entry *item
	var ok bool
	if entry, ok = obj.(*item); !ok {
		utilruntime.HandleError(fmt.Errorf("expected *item in workqueue but got %#v", obj))
		return true
	}

	key, ok := entry.object.(string)
	if !ok {
		return false
	}

	o, exists, err := w.store.GetByKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("getting object %#v from cache: %w", obj, err))
		return true
	}
	if !exists {
		if entry.state == delete {
			w.logger.Debugf("Object %+v was not found in the store, deleting anyway!", key)
			// delete anyway in order to clean states
			w.handler.OnDelete(entry.objectRaw)
		}
		return true
	}

	switch entry.state {
	case add:
		w.handler.OnAdd(o)
	case update:
		w.handler.OnUpdate(o)
	case delete:
		w.handler.OnDelete(o)
	}

	return true
}
