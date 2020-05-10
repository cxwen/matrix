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
	"github.com/cxwen/matrix/common/constants"
	. "github.com/cxwen/matrix/pkg"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	crdv1 "github.com/cxwen/matrix/api/v1"
)

// MasterReconciler reconciles a Master object
type MasterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=crd.cxwen.com,resources=masters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=crd.cxwen.com,resources=masters/status,verbs=get;update;patch

func (r *MasterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("master", req.NamespacedName)

	log.V(1).Info("Master reconcile triggering")
	master := crdv1.Master{}

	var err error
	if err = r.Get(ctx, req.NamespacedName, &master); err != nil {
		log.Error(err, "unable to fetch master")
		return ctrl.Result{}, ignoreNotFound(err)
	}

	masterDeploy := MasterDedploy{
		Context: ctx,
		Client: r.Client,
		Log: r.Log,
		MasterCrd: &master,
	}

	masterFinalizer := constants.DefaultFinalizer
	if master.ObjectMeta.DeletionTimestamp.IsZero() {
		if ! containsString(master.ObjectMeta.Finalizers, masterFinalizer) {
			master.ObjectMeta.Finalizers = append(master.ObjectMeta.Finalizers, masterFinalizer)
			return r.createMaster(&masterDeploy, &master)
		}
	} else {
		if containsString(master.ObjectMeta.Finalizers, masterFinalizer) {
			result, err := r.deleteMaster(&masterDeploy, &master)
			if err != nil {
				return result, err
			}

			master.ObjectMeta.Finalizers = removeString(master.ObjectMeta.Finalizers, masterFinalizer)
			if err = r.Update(context.Background(), &master); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *MasterReconciler) createMaster(masterDeploy *MasterDedploy, master *crdv1.Master) (ctrl.Result, error) {
	var name = master.Name
	var namespace = master.Namespace
	var version = master.Spec.Version
	var replicas = master.Spec.Replicas
	var imageRegistry = master.Spec.ImageRegistry
	var imageRepo = &master.Spec.ImageRepo
	var etcdCluster = master.Spec.EtcdCluster

	master.Status.Phase = crdv1.MasterInitializingPhase
	err := r.Status().Update(masterDeploy.Context, master)
	if err != nil {
		r.Log.Error(err, "update master status phase failure", "master", name)
		return ctrl.Result{Requeue:true}, err
	}

	err = masterDeploy.CreateService(name, namespace)
	if err != nil {
		r.Log.Error(err, "create master svc failure", "name", name, "namespace", namespace)
		return ctrl.Result{Requeue:true}, err
	}

	err = masterDeploy.CreateCerts(name, namespace)
	if err != nil {
		r.Log.Error(err, "create master certs failure", "name", name, "namespace", namespace)
		return ctrl.Result{Requeue:true}, err
	}

	err = masterDeploy.CreateKubeconfig(name, namespace)
	if err != nil {
		r.Log.Error(err, "create master kubeconfig failure", "name", name, "namespace", namespace)
		return ctrl.Result{Requeue:true}, err
	}

	err = masterDeploy.CreateDeployment(name, namespace, version, replicas, imageRegistry, imageRepo, etcdCluster)
	if err != nil {
		r.Log.Error(err, "create master deployment failure", "name", name, "namespace", namespace)
		return ctrl.Result{Requeue:true}, err
	}

	err = masterDeploy.CheckMasterRunning(name, namespace)
	if err != nil {
		r.Log.Error(err, "check master deployment failure", "name", name, "namespace", namespace)
		return ctrl.Result{Requeue:true}, err
	}

	master.Status.Phase = crdv1.MasterRunningPhase
	err = r.Status().Update(masterDeploy.Context, master)
	if err != nil {
		r.Log.Error(err, "update master status phase failure", "master", name)
		return ctrl.Result{Requeue:true}, err
	}

	return ctrl.Result{}, nil
}

func (r *MasterReconciler) deleteMaster(masterDeploy *MasterDedploy, master *crdv1.Master) (ctrl.Result, error) {
	var err error
	name := master.Name
	namespace := master.Namespace
	master.Status.Phase = crdv1.MasterTeminatingPhase
	err = r.Status().Update(masterDeploy.Context, master)
	if err != nil {
		r.Log.Error(err, "update etcdcluster status phase failure", "master", name, "namespace", namespace)
		return ctrl.Result{Requeue:true}, err
	}

	err = masterDeploy.DeleteMaster(name, namespace)
	if err != nil {
		r.Log.Error(err, "delete master failure", "name", name,  "namespace", namespace)
		return ctrl.Result{Requeue:true}, err
	}

	return ctrl.Result{}, nil
}

func (r *MasterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crdv1.Master{}).
		Complete(r)
}