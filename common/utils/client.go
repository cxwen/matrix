package utils

import (
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	crdclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var MatrixClient = make(map[string]runtimeclient.Client)

func GetRuntimeClient(kubeconfig []byte) (runtimeclient.Client, error) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = rbac.AddToScheme(scheme)
	_ = apiextensions.AddToScheme(scheme)

	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	client, err := runtimeclient.New(config, runtimeclient.Options{
		Scheme: scheme,
	})

	if err != nil {
		return nil, err
	}

	return client, nil
}

func GetCrdClient(kubeconfig []byte) (*crdclient.Clientset, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	crdClient, err := crdclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return crdClient, nil
}

func InitMatrixClient(mcClient runtimeclient.Client, ctx context.Context, matrixName string, namespace string) error {
	cmName := fmt.Sprintf("%s-kubeconfig", matrixName)
	if _, ok := MatrixClient[matrixName]; ! ok {
		configmap := &corev1.ConfigMap{}
		getOk := runtimeclient.ObjectKey{Name: cmName, Namespace: namespace}
		err := mcClient.Get(ctx, getOk, configmap)
		if err != nil {
			return err
		}

		if _, ok := configmap.Data["admin.conf"]; ok {
			matrixClient, err := GetRuntimeClient([]byte(configmap.Data["admin.conf"]))
			if err != nil {
				return err
			}
			MatrixClient[matrixName] = matrixClient
		} else {
			return fmt.Errorf("admin.conf key dosen't exist in %s configmap in %s", cmName, namespace)
		}

	}

	return nil
}
