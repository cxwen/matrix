package pkg

import (
	"context"
	"fmt"
	crdv1 "github.com/cxwen/matrix/api/v1"
	"github.com/cxwen/matrix/common/constants"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
	"time"
)

type Matrix interface {
	Create(string, string, *crdv1.MatrixSpec) error
	Delete(string, string) error
}

type MatrixDedploy struct {
	context.Context
	client.Client
	Log logr.Logger
}

func (m *MatrixDedploy) Create(name string, namespace string, matrixConfig *crdv1.MatrixSpec) error {
	var wg sync.WaitGroup
	var waitEtcdReadyErr error
	var waitMasterRunningErr error

	m.setDefaultMatrix(name, matrixConfig)

	etcdClusterCrd := &crdv1.EtcdCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "crd.cxwen.com/v1",
			Kind: "EtcdCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-ec", name),
			Namespace: namespace,
		},
		Spec: *matrixConfig.Etcd,
	}

	err := m.Client.Create(m.Context, etcdClusterCrd)
	if err != nil {
		return fmt.Errorf("create %s etcd cluster failure, error: %v\n", etcdClusterCrd.Name, err)
	}

	wg.Add(1)
	go waitForEtcdClusterReady(m.Context, m.Client, &wg, &waitEtcdReadyErr, etcdClusterCrd.Name, namespace)
	wg.Wait()

	if waitEtcdReadyErr != nil {
		return fmt.Errorf("etcd cluster %s isn't ready, error: %v\n", etcdClusterCrd.Name, err)
	}

	masterCrd := &crdv1.Master{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "crd.cxwen.com/v1",
			Kind: "Master",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-km", name),
			Namespace: namespace,
		},
		Spec: *matrixConfig.Master,
	}

	err = m.Client.Create(m.Context, masterCrd)
	if err != nil {
		return fmt.Errorf("create %s master failure, error: %v\n", masterCrd.Name, err)
	}

	wg.Add(1)
	go waitForMasterRunning(m.Context, m.Client, &wg, &waitMasterRunningErr, etcdClusterCrd.Name, namespace)
	wg.Wait()
	if waitMasterRunningErr != nil {
		return fmt.Errorf("etcd cluster %s isn't ready, error: %v\n", etcdClusterCrd.Name, err)
	}

	dnsCrd := &crdv1.Dns{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "crd.cxwen.com/v1",
			Kind: "Dns",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s", name),
			Namespace: namespace,
		},
		Spec: *matrixConfig.Dns,
	}

	err = m.Client.Create(m.Context, dnsCrd)
	if err != nil {
		return fmt.Errorf("create %s dns failure, error: %v\n", dnsCrd.Name, err)
	}

	networkPluginCrd := &crdv1.NetworkPlugin{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "crd.cxwen.com/v1",
			Kind: "NetworkPlugin",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s", name),
			Namespace: namespace,
		},
		Spec: *matrixConfig.NetworkPlugin,
	}

	err = m.Client.Create(m.Context, networkPluginCrd)
	if err != nil {
		return fmt.Errorf("create %s networkplugin failure, error: %v\n", networkPluginCrd.Name, err)
	}

	return nil
}

func (m *MatrixDedploy) Delete(name string, namespace string) error {
	masterCrd := &crdv1.Master{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "crd.cxwen.com/v1",
			Kind: "Master",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-km", name),
			Namespace: namespace,
		},
	}

	err := m.Client.Delete(m.Context, masterCrd)
	if err != nil {
		return fmt.Errorf("delete %s master failure, error: %v\n", masterCrd.Name, err)
	}

	etcdClusterCrd := &crdv1.EtcdCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "crd.cxwen.com/v1",
			Kind: "EtcdCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-ec", name),
			Namespace: namespace,
		},
	}

	err = m.Client.Delete(m.Context, etcdClusterCrd)
	if err != nil {
		return fmt.Errorf("delete %s etcd cluster failure, error: %v\n", etcdClusterCrd.Name, err)
	}

	dnsCrd := &crdv1.Dns{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "crd.cxwen.com/v1",
			Kind: "Dns",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s", name),
			Namespace: namespace,
		},
	}

	err = m.Client.Delete(m.Context, dnsCrd)
	if err != nil {
		return fmt.Errorf("delete %s dns failure, error: %v\n", dnsCrd.Name, err)
	}

	networkPluginCrd := &crdv1.NetworkPlugin{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "crd.cxwen.com/v1",
			Kind: "NetworkPlugin",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s", name),
			Namespace: namespace,
		},
	}

	err = m.Client.Delete(m.Context, networkPluginCrd)
	if err != nil {
		return fmt.Errorf("delete %s networkplugin failure, error: %v\n", networkPluginCrd.Name, err)
	}

	return nil
}

func waitForEtcdClusterReady(ctx context.Context, runtimeclient client.Client, wg *sync.WaitGroup, err *error, etcdClusterName string, namespace string) {
	timeout := time.After(time.Minute * 5)

	for {
		select {
		case <-timeout:
			errNew := fmt.Errorf("check etcd ready timeout")
			err = &errNew
			break
		default:
			etcdCluster := &crdv1.EtcdCluster{}
			getOk := client.ObjectKey{Name: etcdClusterName, Namespace: namespace}
			getErr := runtimeclient.Get(ctx, getOk, etcdCluster)
			if getErr != nil && apierrors.IsNotFound(getErr) {
				time.Sleep(time.Second * 3)
				continue
			}

			if etcdCluster.Status.Phase == crdv1.EtcdReadyPhase {
				break
			}

			time.Sleep(time.Second * 2)
		}
	}

	wg.Done()
}

func waitForMasterRunning(ctx context.Context, runtimeclient client.Client, wg *sync.WaitGroup, err *error, masterName string, namespace string)  {
	timeout := time.After(time.Minute * 5)

	for {
		select {
		case <-timeout:
			errNew := fmt.Errorf("check etcd ready timeout")
			err = &errNew
			break
		default:
			master := &crdv1.Master{}
			getOk := client.ObjectKey{Name: masterName, Namespace: namespace}
			getErr := runtimeclient.Get(ctx, getOk, master)
			if getErr != nil && apierrors.IsNotFound(getErr) {
				time.Sleep(time.Second * 3)
				continue
			}

			if master.Status.Phase == crdv1.MasterRunningPhase {
				break
			}

			time.Sleep(time.Second * 2)
		}
	}

	wg.Done()
}

func (m *MatrixDedploy) setDefaultMatrix(name string, matrixConfig *crdv1.MatrixSpec) {
	if matrixConfig.Etcd == nil {
		matrixConfig.Etcd = &constants.DefaultEtcd.Spec
	} else {
		if matrixConfig.Etcd.ImageRegistry == "" {
			matrixConfig.Etcd.ImageRegistry = constants.DefaultEtcd.Spec.ImageRegistry
		}
		if matrixConfig.Etcd.Version == "" {
			matrixConfig.Etcd.Version = constants.DefaultEtcd.Spec.Version
		}
		if matrixConfig.Etcd.ImageRepo == "" {
			matrixConfig.Etcd.ImageRepo = constants.DefaultEtcd.Spec.ImageRepo
		}
		if matrixConfig.Etcd.Replicas == 0 {
			matrixConfig.Etcd.Replicas = constants.DefaultEtcd.Spec.Replicas
		}
		if matrixConfig.Etcd.StorageDir == "" {
			matrixConfig.Etcd.StorageDir = constants.DefaultEtcd.Spec.StorageDir
		}
		if matrixConfig.Etcd.StorageClass == "" {
			matrixConfig.Etcd.StorageClass = constants.DefaultEtcd.Spec.StorageClass
		}
		if matrixConfig.Etcd.StorageSize == "" {
			matrixConfig.Etcd.StorageSize = constants.DefaultEtcd.Spec.StorageSize
		}
	}

	if matrixConfig.Master == nil {
		matrixConfig.Master = &constants.DefaultMaster.Spec
	} else {
		if matrixConfig.Master.Version == "" {
			matrixConfig.Master.Version = constants.DefaultMaster.Spec.Version
		}
		if matrixConfig.Master.Replicas == 0 {
			matrixConfig.Master.Replicas = constants.DefaultMaster.Spec.Replicas
		}
		if matrixConfig.Master.ImageRepo.Apiserver == "" {
			matrixConfig.Master.ImageRepo.Apiserver = constants.DefaultMaster.Spec.ImageRepo.Apiserver
		}
		if matrixConfig.Master.ImageRepo.ControllerManager == "" {
			matrixConfig.Master.ImageRepo.ControllerManager = constants.DefaultMaster.Spec.ImageRepo.ControllerManager
		}
		if matrixConfig.Master.ImageRepo.Scheduler == "" {
			matrixConfig.Master.ImageRepo.Scheduler = constants.DefaultMaster.Spec.ImageRepo.Scheduler
		}
		if matrixConfig.Master.ImageRepo.Proxy == "" {
			matrixConfig.Master.ImageRepo.Proxy = constants.DefaultMaster.Spec.ImageRepo.Proxy
		}
		if matrixConfig.Master.EtcdCluster == "" {
			matrixConfig.Master.EtcdCluster = fmt.Sprintf("%s-ec", name)
		}
	}

	if matrixConfig.Dns == nil {
		matrixConfig.Dns = &constants.DefaultDns.Spec
	} else {
		if matrixConfig.Dns.ImageRepo == "" {
			matrixConfig.Dns.ImageRepo = constants.DefaultDns.Spec.ImageRepo
		}
		if matrixConfig.Dns.Replicas == 0 {
			matrixConfig.Dns.Replicas = constants.DefaultDns.Spec.Replicas
		}
		if matrixConfig.Dns.Version == "" {
			matrixConfig.Dns.Version = constants.DefaultDns.Spec.Version
		}
		if matrixConfig.Dns.ImageRegistry == "" {
			matrixConfig.Dns.ImageRegistry = constants.DefaultDns.Spec.ImageRegistry
		}
		if matrixConfig.Dns.Type == "" {
			matrixConfig.Dns.Type = constants.DefaultDns.Spec.Type
		}
	}

	if matrixConfig.NetworkPlugin == nil {
		matrixConfig.NetworkPlugin = &constants.DefaultNetworkPlugin.Spec
	} else {
		if matrixConfig.NetworkPlugin.Type == "" {
			matrixConfig.NetworkPlugin.Type = constants.DefaultNetworkPlugin.Spec.Type
			matrixConfig.NetworkPlugin.Calico = constants.DefaultNetworkPlugin.Spec.Calico
		}
	}
}

