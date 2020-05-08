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
	"github.com/cxwen/matrix/pkg"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	crdv1 "github.com/cxwen/matrix/api/v1"
	"github.com/cxwen/matrix/common/constants"
)

// EtcdClusterReconciler reconciles a EtcdCluster object
type EtcdClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=crd.cxwen.com,resources=etcdclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=crd.cxwen.com,resources=etcdclusters/status,verbs=get;update;patch

func (r *EtcdClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("etcdcluster", req.NamespacedName)

	log.V(1).Info("EtcdCluster reconcile triggering")
	etcdCluster := crdv1.EtcdCluster{}

	var err error
	if err = r.Get(ctx, req.NamespacedName, &etcdCluster); err != nil {
		log.Error(err, "unable to fetch etcdcluster")
		return ctrl.Result{}, ignoreNotFound(err)
	}

	etcdClusterFinalizer := constants.DefaultFinalizer
	if etcdCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		if ! containsString(etcdCluster.ObjectMeta.Finalizers, etcdClusterFinalizer) {
			etcdCluster.ObjectMeta.Finalizers = append(etcdCluster.ObjectMeta.Finalizers, etcdClusterFinalizer)
			return r.createEtcdCluster(ctx, &etcdCluster)
		}
	} else {
		if containsString(etcdCluster.ObjectMeta.Finalizers, etcdClusterFinalizer) {
			result, err := r.deleteEtcdCluster(ctx, &etcdCluster)
			if err != nil {
				return result, err
			}

			etcdCluster.ObjectMeta.Finalizers = removeString(etcdCluster.ObjectMeta.Finalizers, etcdClusterFinalizer)
			if err = r.Update(context.Background(), &etcdCluster); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *EtcdClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crdv1.EtcdCluster{}).
		Complete(r)
}

func (r *EtcdClusterReconciler) createEtcdCluster(ctx context.Context, etcdCluster *crdv1.EtcdCluster) (ctrl.Result, error) {
	var err error
	etcdClusterName := etcdCluster.Name
	namespace := etcdCluster.Namespace
	etcdCluster.Status.Phase = crdv1.EtcdPendingPhase
	err = r.Status().Update(ctx, etcdCluster)
	if err != nil {
		r.Log.Error(err, "update etcdcluster status phase failure", "etcdcluster", etcdCluster.Name)
		return ctrl.Result{Requeue:true}, err
	}

	etcdDeploy := &pkg.EctdCluster{Context:ctx, Client:r.Client, Log:r.Log}
	err = etcdDeploy.CreateEtcdCerts(etcdClusterName, namespace)
	if err != nil {
		r.Log.Error(err, "create certs failure", "etcdcluster", etcdCluster.Name)
		return ctrl.Result{Requeue:true}, err
	}

	err = etcdDeploy.CreateService(etcdClusterName, namespace)
	if err != nil {
		r.Log.Error(err, "create etcdcluster service failure", "etcdcluster", etcdCluster.Name)
		return ctrl.Result{Requeue:true}, err
	}

	replicas := etcdCluster.Spec.Replicas
	image := fmt.Sprintf("%s:%s",etcdCluster.Spec.ImageRepo, etcdCluster.Spec.Version)
	datadir := ""
	if etcdCluster.Spec.StorageClass == "local" {
		datadir = etcdCluster.Spec.StorageDir
	}

	if datadir == "" {
		datadir = constants.DefaultEtcdStorageDir
	}

	err = etcdDeploy.CreateEtcdStatefulSet(etcdClusterName, namespace, replicas, image, datadir)
	if err != nil {
		r.Log.Error(err, "create etcdcluster statefulset failure", "etcdcluster", etcdCluster.Name)
		return ctrl.Result{Requeue:true}, err
	}

	etcdCluster.Status.Phase = crdv1.EtcdInitializingPhase
	err = r.Status().Update(ctx, etcdCluster)
	if err != nil {
		r.Log.Error(err, "update etcdcluster status phase failure", "etcdcluster", etcdCluster.Name)
		return ctrl.Result{Requeue:true}, err
	}

	// waiting for etcd ready
	if err := etcdDeploy.CheckEtcdReady(etcdClusterName, namespace, etcdCluster.Spec.Replicas); err != nil {
		return ctrl.Result{Requeue:true}, err
	}

	etcdCluster.Status.Phase = crdv1.EtcdReadyPhase
	err = r.Status().Update(ctx, etcdCluster)
	if err != nil {
		r.Log.Error(err, "update etcdcluster status phase failure", "etcdcluster", etcdCluster.Name)
		return ctrl.Result{Requeue:true}, err
	}

	return ctrl.Result{}, nil
}

func (r *EtcdClusterReconciler) deleteEtcdCluster(ctx context.Context, etcdCluster *crdv1.EtcdCluster) (ctrl.Result, error) {
	var err error
	etcdClusterName := etcdCluster.Name
	namespace := etcdCluster.Namespace
	etcdCluster.Status.Phase = crdv1.EtcdTeminatingPhase
	err = r.Status().Update(ctx, etcdCluster)
	if err != nil {
		r.Log.Error(err, "update etcdcluster status phase failure", "etcdcluster", etcdCluster.Name)
		return ctrl.Result{Requeue:true}, err
	}

	etcdDeploy := &pkg.EctdCluster{Context:ctx, Client:r.Client, Log:r.Log}
	err = etcdDeploy.DeleteEtcd(etcdClusterName, namespace)
	if err != nil {
		r.Log.Error(err, "delete etcd failure", "etcdcluster", etcdCluster.Name)
		return ctrl.Result{Requeue:true}, err
	}

	return ctrl.Result{}, nil
}
