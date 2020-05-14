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

// DnsReconciler reconciles a Dns object
type DnsReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=crd.cxwen.com,resources=dns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps;serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="extensions",resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=crd.cxwen.com,resources=dns/status,verbs=get;update;patch

func (r *DnsReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("dns", req.NamespacedName)

	log.V(1).Info("Dns reconcile triggering")
	dns := crdv1.Dns{}

	var err error
	if err = r.Get(ctx, req.NamespacedName, &dns); err != nil {
		if IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch dns")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	clientName := fmt.Sprintf("%s-km",dns.Name)
	err = InitMatrixClient(r.Client, ctx, clientName, dns.Namespace)
	if err != nil {
		r.Log.Error(err, "init matrix client failure", "name", dns.Name, "namespace", dns.Namespace)
		return ctrl.Result{}, err
	}

	dnsDeploy := DnsDedploy{
		Context: ctx,
		Client: r.Client,
		Log: r.Log,
		MatrixClient: MatrixClient[clientName],
	}

	if dnsDeploy.MatrixClient == nil {
		return ctrl.Result{}, fmt.Errorf("matrix client is nil")
	}

	dnsFinalizer := constants.DefaultFinalizer
	if dns.ObjectMeta.DeletionTimestamp.IsZero() {
		if ! ContainsString(dns.ObjectMeta.Finalizers, dnsFinalizer) {
			dns.ObjectMeta.Finalizers = append(dns.ObjectMeta.Finalizers, dnsFinalizer)
			if err := r.Update(ctx, &dns); err != nil {
				return ctrl.Result{}, err
			}

			return r.createDns(&dnsDeploy, &dns)
		}
	} else {
		if ContainsString(dns.ObjectMeta.Finalizers, dnsFinalizer) {
			//result, err := r.deleteDns(&dnsDeploy, &dns)
			//if err != nil {
			//	return result, err
			//}

			dns.ObjectMeta.Finalizers = RemoveString(dns.ObjectMeta.Finalizers, dnsFinalizer)
			if err = r.Update(ctx, &dns); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *DnsReconciler) createDns(dnsDeploy *DnsDedploy, dns *crdv1.Dns) (ctrl.Result, error) {
	dnsType := dns.Spec.Type
	replicas := dns.Spec.Replicas
	version := dns.Spec.Version
	imageRepo := dns.Spec.ImageRepo
	imageRegistry := dns.Spec.ImageRegistry

	dns.Status.Phase = crdv1.DnsInitializingPhase
	err := r.Status().Update(dnsDeploy.Context, dns)
	if err != nil {
		r.Log.Error(err, "update dns status phase failure", "dns", dns.Name)
		return ctrl.Result{}, err
	}

	err = dnsDeploy.Create(dnsType, replicas, version, imageRegistry, imageRepo)
	if err != nil {
		r.Log.Error(err,"create dns failure", "dns crd name", dns.Name, "dns crd namespace", dns.Namespace)
		return ctrl.Result{}, err
	}

	dns.Status.Phase = crdv1.DnsReadyPhase
	err = r.Status().Update(dnsDeploy.Context, dns)
	if err != nil {
		r.Log.Error(err, "update dns status phase failure", "dns", dns.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DnsReconciler) deleteDns(dnsDeploy *DnsDedploy, dns *crdv1.Dns) (ctrl.Result, error) {
	dns.Status.Phase = crdv1.DnsTeminatingPhase
	err := r.Status().Update(dnsDeploy.Context, dns)
	if err != nil {
		r.Log.Error(err, "update dns status phase failure when delete", "dns", dns.Name)
		return ctrl.Result{}, err
	}

	//err = dnsDeploy.Delete(dns.Spec.Type, dns.Spec.Version, dns.Spec.ImageRegistry, dns.Spec.ImageRepo)
	//if err != nil {
	//	r.Log.Error(err,"delete dns failure", "dns crd name", dns.Name, "dns crd namespace", dns.Namespace)
	//	return ctrl.Result{}, err
	//}

	return ctrl.Result{}, nil
}

func (r *DnsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crdv1.Dns{}).
		Complete(r)
}