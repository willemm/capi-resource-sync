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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/cluster-api/controllers/remote"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	capiv1 "willemm/capi-resource-sync/api/v1alpha1"
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

	log.Info("Getting cluster client", "cluster", clone.Spec.Target.Cluster)
	/*
	// Get the cluster secret
	cluster := &v1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: clone.Spec.Target.Cluster.Name, Namespace: clone.Spec.Target.Cluster.Namespace}, cluster); err != nil {
		return ctrl.Result{}, err
	}
	*/

	// Make client from the cluster secret
	cluster, err := remote.NewClusterClient(ctx, "", r.Client, client.ObjectKey{Name: clone.Spec.Target.Cluster.Name, Namespace: clone.Spec.Target.Cluster.Namespace})
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Getting source object", "source", clone.Spec.Source)
	// Get the source object to clone
	gv, err := schema.ParseGroupVersion(*clone.Spec.Source.APIGroup)
	if err != nil {
		log.Error(err, "Parsing APIGroup", "apigroup", clone.Spec.Source.APIGroup)
		// This is probably fatal so don't retry
		return ctrl.Result{}, nil
	}
	source := &unstructured.Unstructured{}
	source.SetGroupVersionKind(gv.WithKind(clone.Spec.Source.Kind))
	if err := r.Get(ctx, client.ObjectKey{Name: clone.Spec.Source.Name, Namespace: req.NamespacedName.Namespace}, source); err != nil {
		return ctrl.Result{}, err
	}

	if clone.Spec.Target.Name != "" {
		log.Info("Setting target name", "name", clone.Spec.Target.Name)
		source.SetName(clone.Spec.Target.Name)
	}
	if clone.Spec.Target.Namespace != "" {
		log.Info("Setting target name", "name", clone.Spec.Target.Namespace)
		source.SetNamespace(clone.Spec.Target.Namespace)
	}

	log.Info("Copying object to target cluster", "object", source)
	if err := cluster.Create(ctx, source); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("Done")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourceCloneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capiv1.ResourceClone{}).
		Complete(r)
}
