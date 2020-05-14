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
	"github.com/cxwen/matrix/common/utils"
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

// +kubebuilder:rbac:groups=crd.cxwen.com,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list
// +kubebuilder:rbac:groups=crd.cxwen.com,resources=matrices/status,verbs=get;update;patch

func (r *MatrixReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("matrix", req.NamespacedName)

	log.V(1).Info("Matrix reconcile triggering")
	matrix := crdv1.Matrix{}

	var err error
	if err = r.Get(ctx, req.NamespacedName, &matrix); err != nil {
		if utils.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch matrix")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	matrixDeploy := MatrixDedploy{
		Context: ctx,
		Client: r.Client,
		Log: log,
	}

	matrixFinalizer := constants.DefaultFinalizer
	if matrix.ObjectMeta.DeletionTimestamp.IsZero() {
		if ! utils.ContainsString(matrix.ObjectMeta.Finalizers, matrixFinalizer) {
			matrix.ObjectMeta.Finalizers = append(matrix.ObjectMeta.Finalizers, matrixFinalizer)
			if err = r.Update(ctx, &matrix); err != nil {
				return ctrl.Result{}, err
			}

			return r.createMatrix(&matrixDeploy, &matrix)
		}
	} else {
		if utils.ContainsString(matrix.ObjectMeta.Finalizers, matrixFinalizer) {
			result, err := r.deleteMatrix(&matrixDeploy, &matrix)
			if err != nil {
				return result, err
			}

			matrix.ObjectMeta.Finalizers = utils.RemoveString(matrix.ObjectMeta.Finalizers, matrixFinalizer)
			if err = r.Update(ctx, &matrix); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *MatrixReconciler) createMatrix(matrixDeploy *MatrixDedploy, matrix *crdv1.Matrix) (ctrl.Result, error) {
	matrix.Status.Phase = crdv1.MatrixInitializingPhase
	err := r.Status().Update(matrixDeploy.Context, matrix)
	if err != nil {
		r.Log.Error(err, "update matirx status phase failure", "matirx", matrix.Name)
		return ctrl.Result{}, err
	}

	err = matrixDeploy.Create(matrix.Name, matrix.Namespace, &matrix.Spec)
	if err != nil {
		r.Log.Error(err, "create matirx failure", "matirx", matrix.Name)
		return ctrl.Result{}, err
	}

	err = matrixDeploy.Update(matrix.Name, matrix.Namespace, matrix)
	if err != nil {
		r.Log.Error(err, "update matirx spec value failure", "matirx", matrix.Name)
		return ctrl.Result{}, err
	}

	err = r.Update(matrixDeploy.Context, matrix)
	if err != nil {
		r.Log.Error(err, "update matirx failure", "matirx", matrix.Name)
		return ctrl.Result{}, err
	}

	matrix.Status.Phase = crdv1.MatrixReadyPhase
	err = r.Status().Update(matrixDeploy.Context, matrix)
	if err != nil {
		r.Log.Error(err, "update matirx status phase failure", "matirx", matrix.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MatrixReconciler) deleteMatrix(matrixDeploy *MatrixDedploy, matrix *crdv1.Matrix) (ctrl.Result, error) {
	matrix.Status.Phase = crdv1.MatrixTeminatingPhase
	err := r.Status().Update(matrixDeploy.Context, matrix)
	if err != nil {
		r.Log.Error(err, "update matirx status phase failure", "matirx", matrix.Name)
		return ctrl.Result{}, err
	}

	err = matrixDeploy.Delete(matrix.Name, matrix.Namespace)
	if err != nil {
		r.Log.Error(err, "create matirx failure", "matirx", matrix.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MatrixReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crdv1.Matrix{}).
		Complete(r)
}

