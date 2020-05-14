package constants

import (
	crdv1 "github.com/cxwen/matrix/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	DefaultEtcd = crdv1.EtcdCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "crd.cxwen.com/v1",
			Kind: "EtcdCluster",
		},
		Spec: crdv1.EtcdClusterSpec{
			Version: "3.3.17",
			Replicas: 1,
			ImageRegistry: DefaultImageRegistry,
			ImageRepo: DefaultImageProject + "/etcd-amd64",
			StorageDir: DefaultEtcdStorageDir,
		},
	}

	DefaultMaster = crdv1.Master{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "crd.cxwen.com/v1",
			Kind: "Master",
		},
		Spec: crdv1.MasterSpec{
			Version: "v1.15.12",
			Replicas: 1,
			ImageRegistry: DefaultImageRegistry,
			ImageRepo: &crdv1.ImageRepo{
				Apiserver: DefaultImageProject + "/kube-apiserver-amd64",
				ControllerManager: DefaultImageProject + "/kube-controller-manager-amd64",
				Scheduler: DefaultImageProject + "/kube-scheduler-amd64",
				Proxy: DefaultImageProject + "/kube-proxy-amd64",
			},
			Expose: &crdv1.Expose{
				Method: "NodePort",
				Node: []string{"127.0.0.1"},
				Port: "6443",
			},
		},
	}

	DefaultDns = crdv1.Dns{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "crd.cxwen.com/v1",
			Kind: "Dns",
		},
		Spec: crdv1.DnsSpec{
			Type: "coredns",
			Version: "1.6.5",
			Replicas: 2,
			ImageRegistry: DefaultImageRegistry,
			ImageRepo: DefaultImageProject + "/coredns-amd64",
		},
	}

	DefaultNetworkPlugin = crdv1.NetworkPlugin{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "crd.cxwen.com/v1",
			Kind: "NetworkPlugin",
		},
		Spec: crdv1.NetworkPluginSpec{
			Type: "calico",
			Calico: &crdv1.CalicoConfig{
				Version: DefaultCalicoVersion,
				ImageRegistry: DefaultImageRegistry,
				InstallCniImageRepo: DefaultInstallCniImageRepo,
				NodeImageRepo: DefaultNodeImageRepo,
				FlexvolDriverImageRepo: DefaultFlexvolDriverImageRepo,
				KubeControllerImageRepo: DefaultKubeControllerImageRepo,
				IpAutodetectionMethod: DefaultIpAutodetectionMethod,
				Ipv4poolIpip: DefaultIpv4poolIpip,
				Ipv4poolCidr: DefaultIpv4poolCidr,
			},
		},
	}
)
