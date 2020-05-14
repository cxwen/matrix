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
	"regexp"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	crdv1 "github.com/cxwen/matrix/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// MasterReconciler reconciles a Master object
type MasterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=crd.cxwen.com,resources=masters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps;endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="extensions",resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=crd.cxwen.com,resources=masters/status,verbs=get;update;patch

func (r *MasterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("master", req.NamespacedName)

	log.V(1).Info("Master reconcile triggering")
	master := crdv1.Master{}

	var err error
	if err = r.Get(ctx, req.NamespacedName, &master); err != nil {
		if IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch master")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	masterDeploy := MasterDedploy{
		Context: ctx,
		Client: r.Client,
		Log: r.Log,
		MasterCrd: &master,
	}

	masterFinalizer := constants.DefaultFinalizer
	if master.ObjectMeta.DeletionTimestamp.IsZero() {
		if ! ContainsString(master.ObjectMeta.Finalizers, masterFinalizer) {
			master.ObjectMeta.Finalizers = append(master.ObjectMeta.Finalizers, masterFinalizer)
			if err := r.Update(ctx, &master); err != nil {
				return ctrl.Result{}, err
			}

			return r.createMaster(&masterDeploy, &master)
		}
	} else {
		if ContainsString(master.ObjectMeta.Finalizers, masterFinalizer) {
			result, err := r.deleteMaster(&masterDeploy, &master)
			if err != nil {
				return result, err
			}

			master.ObjectMeta.Finalizers = RemoveString(master.ObjectMeta.Finalizers, masterFinalizer)
			if err = r.Update(ctx, &master); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *MasterReconciler) createMaster(masterDeploy *MasterDedploy, master *crdv1.Master) (ctrl.Result, error) {
	name := master.Name
	namespace := master.Namespace
	version := master.Spec.Version
	replicas := master.Spec.Replicas
	imageRegistry := master.Spec.ImageRegistry
	imageRepo := master.Spec.ImageRepo
	etcdCluster := master.Spec.EtcdCluster

	master.Status.Phase = crdv1.MasterInitializingPhase
	err := r.Status().Update(masterDeploy.Context, master)
	if err != nil {
		r.Log.Error(err, "update master status phase failure", "master", name)
		return ctrl.Result{}, err
	}

	err = masterDeploy.CreateService(name, namespace)
	if err != nil {
		r.Log.Error(err, "create master svc failure", "name", name, "namespace", namespace)
		return ctrl.Result{}, err
	}

	err = masterDeploy.CreateCerts(name, namespace, master.Spec.Expose.Node)
	if err != nil {
		r.Log.Error(err, "create master certs failure", "name", name, "namespace", namespace)
		return ctrl.Result{}, err
	}

	err = masterDeploy.CreateKubeconfig(name, namespace)
	if err != nil {
		r.Log.Error(err, "create master kubeconfig failure", "name", name, "namespace", namespace)
		return ctrl.Result{}, err
	}
	master.Status.AdminKubeconfig = Base64Encode(masterDeploy.AdminKubeconfig)

	err = masterDeploy.CreateDeployment(name, namespace, version, replicas, imageRegistry, imageRepo, etcdCluster)
	if err != nil {
		r.Log.Error(err, "create master deployment failure", "name", name, "namespace", namespace)
		return ctrl.Result{}, err
	}

	err = masterDeploy.CheckMasterRunning(name, namespace)
	if err != nil {
		r.Log.Error(err, "check master deployment failure", "name", name, "namespace", namespace)
		return ctrl.Result{}, err
	}

	if master.Spec.Expose.Method == "NodePort" {
		masterSvc := &corev1.Service{}
		err := masterDeploy.Client.Get(masterDeploy.Context, client.ObjectKey{Name:master.Name, Namespace:master.Namespace}, masterSvc)
		if err != nil {
			if apierrors.IsNotFound(err) {
				time.Sleep(time.Second * 7)
				err = masterDeploy.Client.Get(masterDeploy.Context, client.ObjectKey{Name:master.Name, Namespace:master.Namespace}, masterSvc)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("get master svc failure, error: %v\n", err)
				}
			}
		}
		master.Spec.Expose.Port = strconv.Itoa(int(masterSvc.Spec.Ports[0].NodePort))

		replaceReg := regexp.MustCompile(`https://.*:6443`)
		nodePortServer := fmt.Sprintf("https://%s:%s", master.Spec.Expose.Node[0], master.Spec.Expose.Port)
		nodePortKubeconfig := replaceReg.ReplaceAll(masterDeploy.AdminKubeconfig, []byte(nodePortServer))
		master.Status.AdminKubeconfig = Base64Encode(nodePortKubeconfig)
	}

	initTry := 10
	for i:=0;i<initTry;i++ {
		r.Log.Info("try init matrix client", "name", name, "num", i)
		err = InitMatrixClient(masterDeploy.Client, masterDeploy.Context, name, master.Namespace)
		if err != nil {
			r.Log.Error(err, "init matrix client failure", "name", name, "namespace", namespace)
			time.Sleep(time.Second * 5)
		} else {
			break
		}
	}

	if MatrixClient[name] == nil {
		return ctrl.Result{}, fmt.Errorf("matrix client is nil")
	}
	masterDeploy.MatrixClient = MatrixClient[name]

	err = masterDeploy.MasterInit(version, imageRegistry, imageRepo, name)
	if err != nil {
		r.Log.Error(err, "init matrix cluster failure", "name", name, "namespace", namespace)
		return ctrl.Result{}, err
	}

	err = r.Client.Update(masterDeploy.Context, master)
	if err != nil {
		r.Log.Error(err, "update master failure", "master", name)
		return ctrl.Result{}, err
	}

	master.Status.Phase = crdv1.MasterReadyPhase
	err = r.Status().Update(masterDeploy.Context, master)
	if err != nil {
		r.Log.Error(err, "update master status phase failure", "master", name)
		return ctrl.Result{}, err
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
		return ctrl.Result{}, err
	}

	err = masterDeploy.DeleteMaster(name, namespace)
	if err != nil {
		r.Log.Error(err, "delete master failure", "name", name,  "namespace", namespace)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MasterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crdv1.Master{}).
		Complete(r)
}