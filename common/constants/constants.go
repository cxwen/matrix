package constants

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ServiceTypemeta = metav1.TypeMeta{
		APIVersion: "v1",
		Kind:       "Service",
	}

	ConfigmapTypemeta = metav1.TypeMeta{
		APIVersion: "v1",
		Kind:       "ConfigMap",
	}

	StatefulsetTypemeta = metav1.TypeMeta{
		APIVersion: "apps/v1",
		Kind:       "StatefulSet",
	}
)

const (
	DefaultEtcdStorageDir = "/data/etcd"
	DefaultFinalizer      = "crd.cxw.com"
)