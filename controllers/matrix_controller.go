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

// MatrixReconciler reconciles a Matrix object
type MatrixReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=crd.cxwen.com,resources=matrices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=crd.cxwen.com,resources=matrices/status,verbs=get;update;patch

func (r *MatrixReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("matrix", req.NamespacedName)

	log.V(1).Info("Matrix reconcile triggering")
	matrix := crdv1.Matrix{}

	var err error
	if err = r.Get(ctx, req.NamespacedName, &matrix); err != nil {
		log.Error(err, "unable to fetch matrix")
		return ctrl.Result{}, ignoreNotFound(err)
	}

	matrixDeploy := MatrixDedploy{
		Context: ctx,
		Client: r.Client,
		Log: r.Log,
	}

	matrixFinalizer := constants.DefaultFinalizer
	if matrix.ObjectMeta.DeletionTimestamp.IsZero() {
		if ! containsString(matrix.ObjectMeta.Finalizers, matrixFinalizer) {
			matrix.ObjectMeta.Finalizers = append(matrix.ObjectMeta.Finalizers, matrixFinalizer)
			return r.createMatrix(&matrixDeploy, &matrix)
		}
	} else {
		if containsString(matrix.ObjectMeta.Finalizers, matrixFinalizer) {
			result, err := r.deleteMatrix(&matrixDeploy, &matrix)
			if err != nil {
				return result, err
			}

			matrix.ObjectMeta.Finalizers = removeString(matrix.ObjectMeta.Finalizers, matrixFinalizer)
			if err = r.Update(context.Background(), &matrix); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil

	return ctrl.Result{}, nil
}

func (r *MatrixReconciler) createMatrix(matrixDeploy *MatrixDedploy, matrix *crdv1.Matrix) (ctrl.Result, error) {

	return ctrl.Result{}, nil
}

func (r *MatrixReconciler) deleteMatrix(matrixDeploy *MatrixDedploy, matrix *crdv1.Matrix) (ctrl.Result, error) {

	return ctrl.Result{}, nil
}

func (r *MatrixReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crdv1.Matrix{}).
		Complete(r)
}
