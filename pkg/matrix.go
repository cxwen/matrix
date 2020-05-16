package pkg

import (
	"context"
	"fmt"
	crdv1 "github.com/cxwen/matrix/api/v1"
	"github.com/cxwen/matrix/common/constants"
	. "github.com/cxwen/matrix/common/utils"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
	"time"
)

type Matrix interface {
	Create(string, string, *crdv1.MatrixSpec) error
	Delete(string, string) error
	Update(string, string, *crdv1.Matrix) error
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
	m.setDefaultEtcdcluster(name, matrixConfig, etcdClusterCrd)

	err := m.Client.Create(m.Context, etcdClusterCrd)
	if IgnoreAlreadyExist(err) != nil {
		return fmt.Errorf("create %s etcd cluster failure, error: %v\n", etcdClusterCrd.Name, err)
	}

	m.Log.Info("etcd cluster create success", "etcd name", etcdClusterCrd.Name, "namespace", namespace)
	m.Log.Info("waiting for etcd cluster ready", "name", etcdClusterCrd.Name, "namespace", namespace)
	wg.Add(1)
	go waitForEtcdClusterReady(m.Context, m.Client, &wg, &waitEtcdReadyErr, etcdClusterCrd.Name, namespace)
	wg.Wait()

	if waitEtcdReadyErr != nil {
		return fmt.Errorf("etcd cluster %s isn't ready, error: %v\n", etcdClusterCrd.Name, err)
	}
	m.Log.Info("etcd cluster is ready", "name", etcdClusterCrd.Name, "namespace", namespace)

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

	err = m.setDefaultMaster(name, matrixConfig, masterCrd)
	if err != nil {
		return fmt.Errorf("setDefaultMaster failure, error: %v\n", err)
	}

	err = m.Client.Create(m.Context, masterCrd)
	if IgnoreAlreadyExist(err) != nil {
		return fmt.Errorf("create %s master failure, error: %v\n", masterCrd.Name, err)
	}
	m.Log.Info("master create success", "name", masterCrd.Name, "namespace", namespace)
	m.Log.Info("waiting for master running", "name", masterCrd.Name, "namespace", namespace)

	wg.Add(1)
	go waitForMasterRunning(m.Log, m.Context, m.Client, &wg, &waitMasterRunningErr, masterCrd.Name, namespace)
	wg.Wait()
	if waitMasterRunningErr != nil {
		return fmt.Errorf("master deployment %s isn't ready, error: %v\n", etcdClusterCrd.Name, err)
	}

	m.Log.Info("master is ready", "name", masterCrd.Name, "namespace", namespace)

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
	m.setDefaultDns(name, matrixConfig, dnsCrd)

	err = m.Client.Create(m.Context, dnsCrd)
	if IgnoreAlreadyExist(err) != nil {
		return fmt.Errorf("create %s dns failure, error: %v\n", dnsCrd.Name, err)
	}
	m.Log.Info("dns create success", "name", dnsCrd.Name, "namespace", namespace)

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
	m.setDefaultNetworkPlugin(name, matrixConfig, networkPluginCrd)

	err = m.Client.Create(m.Context, networkPluginCrd)
	if IgnoreAlreadyExist(err) != nil {
		return fmt.Errorf("create %s networkplugin failure, error: %v\n", networkPluginCrd.Name, err)
	}
	m.Log.Info("networkplugin create success", "name", dnsCrd.Name, "namespace", namespace, "type", dnsCrd.Spec.Type)

	return nil
}

func (m *MatrixDedploy) Delete(name string, namespace string) error {
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

	err := m.Client.Delete(m.Context, dnsCrd)
	if IgnoreNotFound(err) != nil {
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
	if IgnoreNotFound(err) != nil {
		return fmt.Errorf("delete %s networkplugin failure, error: %v\n", networkPluginCrd.Name, err)
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
	}

	err = m.Client.Delete(m.Context, masterCrd)
	if IgnoreNotFound(err) != nil {
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
	if IgnoreNotFound(err) != nil {
		return fmt.Errorf("delete %s etcd cluster failure, error: %v\n", etcdClusterCrd.Name, err)
	}

	return nil
}

func (m *MatrixDedploy) Update(name string, namespace string, matrix *crdv1.Matrix) error {
	masterCrd := &crdv1.Master{}
	err := m.Client.Get(m.Context, client.ObjectKey{Name:fmt.Sprintf("%s-km", name), Namespace:namespace}, masterCrd)
	if IgnoreNotFound(err) != nil {
		return fmt.Errorf("get %s master failure, error: %v\n", masterCrd.Name, err)
	}

	matrix.Spec.Master = &masterCrd.Spec

	etcdClusterCrd := &crdv1.EtcdCluster{}
	err = m.Client.Get(m.Context, client.ObjectKey{Name:fmt.Sprintf("%s-ec", name), Namespace:namespace}, etcdClusterCrd)
	if IgnoreNotFound(err) != nil {
		return fmt.Errorf("get %s etcd cluster failure, error: %v\n", etcdClusterCrd.Name, err)
	}

	matrix.Spec.Etcd = &etcdClusterCrd.Spec

	dnsCrd := &crdv1.Dns{}
	err = m.Client.Get(m.Context, client.ObjectKey{Name:fmt.Sprintf("%s", name), Namespace:namespace}, dnsCrd)
	if IgnoreNotFound(err) != nil {
		return fmt.Errorf("get %s dns failure, error: %v\n", dnsCrd.Name, err)
	}

	matrix.Spec.Dns = &dnsCrd.Spec

	networkPluginCrd := &crdv1.NetworkPlugin{}
	err = m.Client.Get(m.Context, client.ObjectKey{Name:fmt.Sprintf("%s", name), Namespace:namespace}, networkPluginCrd)
	if IgnoreNotFound(err) != nil {
		return fmt.Errorf("get %s networkplugin failure, error: %v\n", networkPluginCrd.Name, err)
	}

	if matrix.Spec.NetworkPlugin == nil {
		matrix.Spec.NetworkPlugin = &crdv1.NetworkPluginSpec{}
	}
	matrix.Spec.NetworkPlugin.Type = networkPluginCrd.Spec.Type
	matrix.Spec.NetworkPlugin.Calico = networkPluginCrd.Spec.Calico

	return nil
}

func waitForEtcdClusterReady(ctx context.Context, runtimeclient client.Client, wg *sync.WaitGroup, err *error, etcdClusterName string, namespace string) {
	timeout := time.After(time.Minute * 5)
	defer wg.Done()

	for {
		select {
		case <-timeout:
			errNew := fmt.Errorf("check etcd ready timeout")
			err = &errNew
			return
		default:
			etcdCluster := &crdv1.EtcdCluster{}
			getOk := client.ObjectKey{Name: etcdClusterName, Namespace: namespace}
			getErr := runtimeclient.Get(ctx, getOk, etcdCluster)
			if getErr != nil && apierrors.IsNotFound(getErr) {
				time.Sleep(time.Second * 3)
				continue
			}

			if etcdCluster.Status.Phase == crdv1.EtcdReadyPhase {
				return
			}

			time.Sleep(time.Second * 2)
		}
	}
}

func waitForMasterRunning(log logr.Logger, ctx context.Context, runtimeclient client.Client, wg *sync.WaitGroup, err *error, masterName string, namespace string) {
	timeout := time.After(time.Minute * 3)
	defer wg.Done()
	for {
		select {
		case <-timeout:
			errNew := fmt.Errorf("check master ready timeout")
			err = &errNew
			log.Error(*err,"check master status not found error", "name", masterName, "namespace", namespace)
			return
		default:
			master := &crdv1.Master{}
			getOk := client.ObjectKey{Name: masterName, Namespace: namespace}
			getErr := runtimeclient.Get(ctx, getOk, master)
			if getErr != nil && apierrors.IsNotFound(getErr) {
				log.Info("check master status not found error", "name", masterName, "namespace", namespace)
				time.Sleep(time.Second * 3)
				continue
			}

			log.Info("check master status", "name", masterName, "phase", master.Status.Phase)
			if master.Status.Phase == crdv1.MasterReadyPhase {
				return
			}

			time.Sleep(time.Second * 5)
		}
	}
}

func (m *MatrixDedploy) setDefaultDns(name string, mc *crdv1.MatrixSpec, dns *crdv1.Dns) {
	if mc.Etcd == nil {
		dns.Spec = crdv1.DnsSpec{}
	} else {
		dns.Spec = *mc.Dns
	}
	if dns.Spec.ImageRepo == "" {
		dns.Spec.ImageRepo = constants.DefaultDns.Spec.ImageRepo
	}
	if dns.Spec.Replicas == 0 {
		dns.Spec.Replicas = constants.DefaultDns.Spec.Replicas
	}
	if dns.Spec.Version == "" {
		dns.Spec.Version = constants.DefaultDns.Spec.Version
	}
	if dns.Spec.ImageRegistry == "" {
		dns.Spec.ImageRegistry = constants.DefaultDns.Spec.ImageRegistry
	}
	if dns.Spec.Type == "" {
		dns.Spec.Type = constants.DefaultDns.Spec.Type
	}
}

func (m *MatrixDedploy) setDefaultEtcdcluster(name string, mc *crdv1.MatrixSpec, etcdCluster *crdv1.EtcdCluster) {
	if mc.Etcd == nil {
		etcdCluster.Spec = crdv1.EtcdClusterSpec{}
	} else {
		etcdCluster.Spec = *mc.Etcd
	}
	if etcdCluster.Spec.ImageRegistry == "" {
		etcdCluster.Spec.ImageRegistry = constants.DefaultEtcd.Spec.ImageRegistry
	}
	if etcdCluster.Spec.Version == "" {
		etcdCluster.Spec.Version = constants.DefaultEtcd.Spec.Version
	}
	if etcdCluster.Spec.ImageRepo == "" {
		etcdCluster.Spec.ImageRepo = constants.DefaultEtcd.Spec.ImageRepo
	}
	if etcdCluster.Spec.Replicas == 0 {
		etcdCluster.Spec.Replicas = constants.DefaultEtcd.Spec.Replicas
	}
	if etcdCluster.Spec.StorageDir == "" {
		etcdCluster.Spec.StorageDir = constants.DefaultEtcd.Spec.StorageDir
	}
	if etcdCluster.Spec.StorageClass == "" {
		etcdCluster.Spec.StorageClass = constants.DefaultEtcd.Spec.StorageClass
	}
	if etcdCluster.Spec.StorageSize == "" {
		etcdCluster.Spec.StorageSize = constants.DefaultEtcd.Spec.StorageSize
	}
}

func (m *MatrixDedploy) setDefaultMaster(name string, mc *crdv1.MatrixSpec, master *crdv1.Master) error {
	if mc.Master == nil {
		master.Spec = crdv1.MasterSpec{}
	} else {
		master.Spec = *mc.Master
	}

	if master.Spec.Version == "" {
		master.Spec.Version = constants.DefaultMaster.Spec.Version
	}
	if master.Spec.Replicas == 0 {
		master.Spec.Replicas = constants.DefaultMaster.Spec.Replicas
	}
	if master.Spec.ImageRegistry == "" {
		master.Spec.ImageRegistry = constants.DefaultMaster.Spec.ImageRegistry
	}

	if master.Spec.ImageRepo == nil {
		master.Spec.ImageRepo = &crdv1.ImageRepo{
			Apiserver:constants.DefaultMaster.Spec.ImageRepo.Apiserver,
			ControllerManager:constants.DefaultMaster.Spec.ImageRepo.ControllerManager,
			Scheduler:constants.DefaultMaster.Spec.ImageRepo.Scheduler,
			Proxy:constants.DefaultMaster.Spec.ImageRepo.Proxy,
		}
	}
	if mc.Master != nil && mc.Master.ImageRepo != nil {
		if mc.Master.ImageRepo.Apiserver != "" {
			master.Spec.ImageRepo.Apiserver = mc.Master.ImageRepo.Apiserver
		}
		if mc.Master.ImageRepo.ControllerManager != "" {
			master.Spec.ImageRepo.ControllerManager = mc.Master.ImageRepo.ControllerManager
		}
		if mc.Master.ImageRepo.Scheduler != "" {
			master.Spec.ImageRepo.Scheduler = mc.Master.ImageRepo.Scheduler
		}
		if mc.Master.ImageRepo.Proxy != "" {
			master.Spec.ImageRepo.Proxy = mc.Master.ImageRepo.Proxy
		}
	}

	if master.Spec.EtcdCluster == "" {
		master.Spec.EtcdCluster = fmt.Sprintf("%s-ec", name)
	}

	nodeList := &corev1.NodeList{}
	err := m.Client.List(m.Context, nodeList)
	if err != nil {
		return fmt.Errorf("get node list failure, error: %v\n", err)
	}
	nodeIp := "127.0.0.1"
	if len(nodeList.Items) > 0 {
		nodeIp = nodeList.Items[0].Status.Addresses[0].Address
	}

	if master.Spec.Expose == nil {
		master.Spec.Expose = &crdv1.Expose{
			Method: "NodePort",
			Node: []string{nodeIp},
		}
	} else {
		if len(master.Spec.Expose.Node) == 0 {
			master.Spec.Expose.Node = append(master.Spec.Expose.Node, nodeIp)
		}
		if master.Spec.Expose.Port == "" {
			master.Spec.Expose.Port = mc.Master.Expose.Port
		}
	}

	return nil
}

func (m *MatrixDedploy) setDefaultNetworkPlugin(name string, mc *crdv1.MatrixSpec, np *crdv1.NetworkPlugin) {
	if np.Spec.Type == "" {
		np.Spec.Type = constants.DefaultNetworkPlugin.Spec.Type
		np.Spec.Calico = constants.DefaultNetworkPlugin.Spec.Calico
	}
}