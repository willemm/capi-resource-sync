/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/controllers/remote"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	capiv1 "willemm/capi-resource-sync/api/v1alpha1"
)

const (
	// resourceVersion of source object, annotation on target object
	AnnotationVersion = "capi.stater.com/sourceResourceVersion"
	// Cluster name of source object, annotation on target object
	AnnotationCluster = "capi.stater.com/sourceCluster"

	// Finalizer
	CloneFinalizer = "capi.stater.com/clone"
)

var (
	// Cluster name (via Env var SOURCE_CLUSTER)
	SourceCluster string
)

// ResourceCloneReconciler reconciles a ResourceClone object
type ResourceCloneReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=capi.stater.com,resources=resourceclones,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=capi.stater.com,resources=resourceclones/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=capi.stater.com,resources=resourceclones/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *ResourceCloneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Get the resourceclone object
	clone := &capiv1.ResourceClone{}
	if err := r.Get(ctx, req.NamespacedName, clone); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !clone.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(ctx, clone)
	}

	log.Info("Getting cluster client", "cluster", clone.Spec.Target.Cluster)

	// Make client from the cluster secret
	cluster, err := remote.NewClusterClient(ctx, "capi-resource-sync", r.Client, client.ObjectKey{Name: clone.Spec.Target.Cluster.Name, Namespace: clone.Spec.Target.Cluster.Namespace})
	if err != nil {
		return r.UpdateStatus(ctx, clone, "Failed", "", errors.Wrap(err, "get target cluster"))
	}

	log.Info("Getting source object", "source", clone.Spec.Source)
	// Get the source object to clone
	gv, err := schema.ParseGroupVersion(*clone.Spec.Source.APIGroup)
	if err != nil {
		log.Error(err, "Parsing APIGroup", "apigroup", clone.Spec.Source.APIGroup)
		// This is probably fatal so don't retry
		return r.UpdateStatus(ctx, clone, "Failed", fmt.Sprintf("APIGroup error: %v", err), nil)
	}
	source := &unstructured.Unstructured{}
	source.SetGroupVersionKind(gv.WithKind(clone.Spec.Source.Kind))
	if err := r.Get(ctx, client.ObjectKey{Name: clone.Spec.Source.Name, Namespace: req.Namespace}, source); err != nil {
		return r.UpdateStatus(ctx, clone, "Failed", "", errors.Wrap(err, "get source object"))
	}

	if clone.Spec.Target.Name != "" {
		log.Info("Setting target name", "name", clone.Spec.Target.Name)
		source.SetName(clone.Spec.Target.Name)
	}
	if clone.Spec.Target.Namespace != "" {
		log.Info("Setting target name", "name", clone.Spec.Target.Namespace)
		source.SetNamespace(clone.Spec.Target.Namespace)
	}
	source.SetOwnerReferences(nil)
	source.SetCreationTimestamp(metav1.Time{})
	source.SetDeletionTimestamp(nil)
	source.SetResourceVersion("")
	source.SetUID("")
	source.SetSelfLink("")
	source.SetFinalizers(nil)
	source.SetGeneration(0)
	source.SetFinalizers(nil)
	source.SetManagedFields(nil)
	unstructured.RemoveNestedField(source.Object, "status")

	// Set sourcerevision on target as annotation
	version := source.GetResourceVersion()
	annotations := source.GetAnnotations()
	annotations[AnnotationVersion] = version
	if SourceCluster != "" {
		annotations[AnnotationCluster] = SourceCluster
	}
	source.SetAnnotations(annotations)

	if !controllerutil.ContainsFinalizer(clone, CloneFinalizer) {
		controllerutil.AddFinalizer(clone, CloneFinalizer)
		if perr := r.Update(ctx, clone); perr != nil {
			return ctrl.Result{}, errors.Wrap(perr, "Add finalzer")
		}
	}

	// Get existing target object
	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(gv.WithKind(clone.Spec.Source.Kind))
	if err := cluster.Get(ctx, client.ObjectKey{Name: source.GetName(), Namespace: source.GetNamespace()}, target); err != nil {
		if !apierrors.IsNotFound(errors.Cause(err)) {
			return r.UpdateStatus(ctx, clone, "Failed", "", errors.Wrap(err, "get target object"))
		}

		log.Info("Creating object on target cluster", "object", source)
		if err := cluster.Create(ctx, source); err != nil {
			return r.UpdateStatus(ctx, clone, "Failed", "", errors.Wrap(err, "create target object"))
		}
		log.Info("Created")

		return r.UpdateStatus(ctx, clone, "Ready", "Created", nil)
	} else {
		// Don't update if not changed
		targetAnnotations := target.GetAnnotations()
		if version == targetAnnotations[AnnotationVersion] {
			log.Info("Source not changed, done")
			return ctrl.Result{}, nil
		}
		log.Info("Updating object on target cluster", "object", source)
		if err := cluster.Update(ctx, source); err != nil {
			return r.UpdateStatus(ctx, clone, "Failed", "", errors.Wrap(err, "update target object"))
		}
		log.Info("Updated")

		return r.UpdateStatus(ctx, clone, "Ready", "Updated", nil)
	}
}

func (r *ResourceCloneReconciler) UpdateStatus(ctx context.Context, clone *capiv1.ResourceClone, status, message string, err error) (ctrl.Result, error) {
	clone.Status.Status = status
	if err != nil {
		clone.Status.Message = fmt.Sprintf("Error: %v", err)
	} else {
		clone.Status.Message = message
	}
	clone.Status.LastUpdated = &metav1.Time{Time: time.Now()}
	if perr := r.Status().Update(ctx, clone); perr != nil {
		if err != nil {
			err = errors.Wrapf(err, "Update failed: %v", perr)
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, err
}

func (r *ResourceCloneReconciler) ReconcileDelete(ctx context.Context, clone *capiv1.ResourceClone) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(clone, CloneFinalizer) {
		log := log.FromContext(ctx)
		cluster, err := remote.NewClusterClient(ctx, "capi-resource-sync", r.Client, client.ObjectKey{Name: clone.Spec.Target.Cluster.Name, Namespace: clone.Spec.Target.Cluster.Namespace})
		if err != nil {
			// This is probably fatal, so give up
			controllerutil.RemoveFinalizer(clone, CloneFinalizer)
			if perr := r.Update(ctx, clone); perr != nil {
				return ctrl.Result{}, errors.Wrap(err, fmt.Sprintf("Remove finalzer error: %v", perr))
			}
			return ctrl.Result{}, err
		}

		gv, err := schema.ParseGroupVersion(*clone.Spec.Source.APIGroup)
		if err != nil {
			log.Error(err, "Parsing APIGroup", "apigroup", clone.Spec.Source.APIGroup)
			// This is probably fatal so don't retry
			return r.UpdateStatus(ctx, clone, "Failed", fmt.Sprintf("APIGroup error: %v", err), nil)
		}
		targetKey := client.ObjectKey{Name: clone.Spec.Target.Name, Namespace: clone.Spec.Target.Namespace}
		if targetKey.Name == "" {
			targetKey.Name = clone.Spec.Source.Name
		}
		if targetKey.Namespace == "" {
			targetKey.Namespace = clone.ObjectMeta.Namespace
		}
		target := &unstructured.Unstructured{}
		target.SetGroupVersionKind(gv.WithKind(clone.Spec.Source.Kind))
		if err := cluster.Get(ctx, targetKey, target); err != nil {
			if !apierrors.IsNotFound(errors.Cause(err)) {
				return r.UpdateStatus(ctx, clone, "Deleting", "", errors.Wrap(err, "get target object"))
			}
			controllerutil.RemoveFinalizer(clone, CloneFinalizer)
			if perr := r.Update(ctx, clone); perr != nil {
				return ctrl.Result{}, errors.Wrap(perr, "Remove finalzer")
			}
		}
		if err := cluster.Delete(ctx, target); err != nil {
			return r.UpdateStatus(ctx, clone, "Deleting", "", errors.Wrap(err, "delete target object"))
		}
		return r.UpdateStatus(ctx, clone, "Deleting", "Deleting target object", nil)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourceCloneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	SourceCluster = os.Getenv("SOURCE_CLUSTER")
	return ctrl.NewControllerManagedBy(mgr).
		For(&capiv1.ResourceClone{}).
		Complete(r)
}
