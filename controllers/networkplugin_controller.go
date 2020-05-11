/*
Copyright 2020 cxwen.

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
	"github.com/cxwen/matrix/common/constants"
	. "github.com/cxwen/matrix/common/utils"
	. "github.com/cxwen/matrix/pkg"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	crdv1 "github.com/cxwen/matrix/api/v1"
)

// NetworkPluginReconciler reconciles a NetworkPlugin object
type NetworkPluginReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=crd.cxwen.com,resources=networkplugins,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=crd.cxwen.com,resources=networkplugins/status,verbs=get;update;patch

func (r *NetworkPluginReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("networkplugin", req.NamespacedName)

	log.V(1).Info("NetworkPlugin reconcile triggering")
	networkplugin := crdv1.NetworkPlugin{}

	var err error
	if err = r.Get(ctx, req.NamespacedName, &networkplugin); err != nil {
		log.Error(err, "unable to fetch networkplugin")
		return ctrl.Result{}, ignoreNotFound(err)
	}

	networkpluginDeploy := NetworkPluginDedploy{
		Context: ctx,
		Client: r.Client,
		Log: r.Log,
		MatrixClient: MatrixClient[networkplugin.Name],
	}

	networkpluginFinalizer := constants.DefaultFinalizer
	if networkplugin.ObjectMeta.DeletionTimestamp.IsZero() {
		if ! containsString(networkplugin.ObjectMeta.Finalizers, networkpluginFinalizer) {
			networkplugin.ObjectMeta.Finalizers = append(networkplugin.ObjectMeta.Finalizers, networkpluginFinalizer)
			return r.createNetworkPlugin(&networkpluginDeploy, &networkplugin)
		}
	} else {
		if containsString(networkplugin.ObjectMeta.Finalizers, networkpluginFinalizer) {
			result, err := r.deleteNetworkPlugin(&networkpluginDeploy, &networkplugin)
			if err != nil {
				return result, err
			}

			networkplugin.ObjectMeta.Finalizers = removeString(networkplugin.ObjectMeta.Finalizers, networkpluginFinalizer)
			if err = r.Update(context.Background(), &networkplugin); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *NetworkPluginReconciler) createNetworkPlugin(networkPluginDeploy *NetworkPluginDedploy, networkPlugin *crdv1.NetworkPlugin) (ctrl.Result, error) {
	networkPluginType := networkPlugin.Spec.Type

	networkPlugin.Status.Phase = crdv1.NetworkPluginInitializingPhase
	err := r.Status().Update(networkPluginDeploy.Context, networkPlugin)
	if err != nil {
		r.Log.Error(err, "update dns status phase failure before create", "network plugin", networkPlugin.Name)
		return ctrl.Result{Requeue:true}, err
	}

	switch networkPluginType {
	case "calico":
		if networkPlugin.Spec.Calico == nil {
			if err != nil {
				return ctrl.Result{Requeue:false}, fmt.Errorf("calico config is null")
			}
		}

		err = networkPluginDeploy.CreateCalico(networkPlugin.Spec.Calico)
		if err != nil {
			return ctrl.Result{Requeue:true}, err
		}
	}

	networkPlugin.Status.Phase = crdv1.NetworkPluginRunningPhase
	err = r.Status().Update(networkPluginDeploy.Context, networkPlugin)
	if err != nil {
		r.Log.Error(err, "update dns status phase failure after create", "network plugin", networkPlugin.Name)
		return ctrl.Result{Requeue:true}, err
	}

	return ctrl.Result{}, nil
}

func (r *NetworkPluginReconciler) deleteNetworkPlugin(networkPluginDeploy *NetworkPluginDedploy, networkPlugin *crdv1.NetworkPlugin) (ctrl.Result, error) {
	networkPluginType := networkPlugin.Spec.Type

	networkPlugin.Status.Phase = crdv1.NetworkPluginTeminatingPhase
	err := r.Status().Update(networkPluginDeploy.Context, networkPlugin)
	if err != nil {
		r.Log.Error(err, "update dns status phase failure before delete", "network plugin", networkPlugin.Name)
		return ctrl.Result{Requeue:true}, err
	}

	switch networkPluginType {
	case "calico":
		if networkPlugin.Spec.Calico == nil {
			if err != nil {
				return ctrl.Result{Requeue:false}, fmt.Errorf("calico config is null")
			}
		}

		err = networkPluginDeploy.DeleteCalico(networkPlugin.Spec.Calico)
		if err != nil {
			return ctrl.Result{Requeue:true}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *NetworkPluginReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crdv1.NetworkPlugin{}).
		Complete(r)
}
