package utils

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var MatrixClient map[string]runtimeclient.Client

func GetRuntimeClient(kubeconfig string) (runtimeclient.Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = rbac.AddToScheme(scheme)
	_ = apiextensions.AddToScheme(scheme)

	client, err := runtimeclient.New(config, runtimeclient.Options{
		Scheme: scheme,
	})

	if err != nil {
		return nil, err
	}

	return client, nil
}
