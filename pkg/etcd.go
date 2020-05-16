package pkg

import (
	"context"
	"crypto/x509"
	"fmt"
	"github.com/cxwen/matrix/common/constants"
	. "github.com/cxwen/matrix/common/utils"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"sync"
	"time"
)

type EtcdCluster interface {
	CreateService(string, string) error
	CreateEtcdCerts(string, string) error
	CreateEtcdStatefulSet(string, string, int, string, string) error
	CheckEtcdReady(string, string, int) error
	DeleteEtcd(string, string) error
}

type EctdDeploy struct {
	context.Context
	client.Client
	Log logr.Logger
}

func (e *EctdDeploy) CreateService(etcdClusterName string, namespace string) error {
	headLessService := corev1.Service{
		TypeMeta: constants.ServiceTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: etcdClusterName,
			Namespace: namespace,
			Labels: map[string]string{
				"k8s-app": etcdClusterName,
			},
		},
		Spec:corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{
					Name: "client",
					Port: 2379,
					Protocol: "TCP",
					TargetPort: intstr.FromInt(2379),
				},
				{
					Name: "server",
					Port: 2380,
					Protocol: "TCP",
					TargetPort: intstr.FromInt(2380),
				},
			},
			Selector: map[string]string{
				"k8s-app": etcdClusterName,
			},
		},
	}

	err := e.Client.Create(e.Context, &headLessService)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			e.Log.Info("etcd headless service is already exist", "etcdCluster", etcdClusterName)
		} else {
			e.Log.Error(err, "create etcd headless service failure", "etcdCluster", etcdClusterName)
			return err
		}
	}
	e.Log.Info("etcd headless service success", "name", headLessService.Name, "namespace", namespace)

	clientService := corev1.Service{
		TypeMeta: constants.ServiceTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-client", etcdClusterName),
			Namespace: namespace,
			Labels: map[string]string{
				"k8s-app": etcdClusterName,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			SessionAffinity: corev1.ServiceAffinityNone,
			Ports: []corev1.ServicePort{
				{
					Name: "client",
					Port: 2379,
					Protocol: "TCP",
					TargetPort: intstr.FromInt(2379),
				},
			},
			Selector: map[string]string{
				"k8s-app": etcdClusterName,
			},
		},
	}

	err = e.Client.Create(e.Context, &clientService)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			e.Log.Info("etcd client service is already exist", "etcdCluster", etcdClusterName)
		} else {
			e.Log.Error(err, "create etcd client service failure", "etcdCluster", etcdClusterName)
			return err
		}
	}
	e.Log.Info("etcd client service success", "name", headLessService.Name, "namespace", namespace)

	return nil
}

func (e *EctdDeploy) CreateEtcdCerts(etcdClusterName string, namespace string) error {
	clientSvc := &corev1.Service{}
	initTry := 10
	for i:=0;i<initTry;i++ {
		e.Log.Info("try get etcd client svc", "name", etcdClusterName, "num", i)
		getClientSvcOk := client.ObjectKey{Name: fmt.Sprintf("%s-client", etcdClusterName), Namespace: namespace}
		err := e.Client.Get(e.Context, getClientSvcOk, clientSvc)
		if err != nil {
			if IgnoreNotFound(err) == nil {
				return err
			} else {
				time.Sleep(time.Second * 5)
				continue
			}
		} else {
			break
		}
	}

	caCfg := Config{
		CommonName: "kubernetes",
	}
	caCert, caKey, err := GenerateCaCert(caCfg)
	if err != nil {
		return err
	}

	serverDomain := fmt.Sprintf("%s.%s", etcdClusterName, namespace)
	clientDomain := fmt.Sprintf("%s-client.%s", etcdClusterName, namespace)
	clientSvcClusterIp := clientSvc.Spec.ClusterIP

	// create server cert
	serverDnsNames := []string{
		"*."+serverDomain,
		clientDomain,
		clientSvcClusterIp,
		"localhost",
	}
	serverAlternateIPs := []net.IP{
		net.ParseIP(clientSvcClusterIp),
		net.ParseIP("127.0.0.1"),
		net.ParseIP("0.0.0.0"),

	}
	serverCfg := Config{
		CommonName: "etcd-server",
		AltNames:   AltNames{
			DNSNames: serverDnsNames,
			IPs: serverAlternateIPs,
		},
		Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	serverCert, serverKey, err := GenerateCert(serverCfg, caCert, caKey)
	if err != nil {
		return err
	}

	// create peer cert
	peerDnsNames := []string{
		fmt.Sprintf("*.%s", serverDomain),
		fmt.Sprintf("*.%s.svc.cluster.local", serverDomain),
	}
	peerCfg := Config{
		CommonName: "etcd-peer",
		AltNames:   AltNames{
			DNSNames: peerDnsNames,
		},
		Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	peerCert, peerKey, err := GenerateCert(peerCfg, caCert, caKey)
	if err != nil {
		return err
	}

	// create client cert
	clientCfg := Config{
		CommonName: "etcd-client",
		Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientCert, clientKey, err := GenerateCert(clientCfg, caCert, caKey)
	if err != nil {
		return err
	}

	//ca.crt  ca.key  etcd-client.crt  etcd-client.key  peer.crt  peer.key  server.crt  server.key
	etcdCerts := map[string]string{
		"ca.crt":          string(EncodeCertPEM(caCert)),
		"ca.key":          string(EncodePrivateKeyPEM(caKey)),
		"server.crt":      string(EncodeCertPEM(serverCert)),
		"server.key":      string(EncodePrivateKeyPEM(serverKey)),
		"peer.crt":        string(EncodeCertPEM(peerCert)),
		"peer.key":        string(EncodePrivateKeyPEM(peerKey)),
		"etcd-client.crt": string(EncodeCertPEM(clientCert)),
		"etcd-client.key": string(EncodePrivateKeyPEM(clientKey)),
	}

	etcdCertsConfigmap := corev1.ConfigMap{
		TypeMeta: constants.ConfigmapTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-cert", etcdClusterName),
			Namespace: namespace,
		},
		Data: etcdCerts,
	}

	err = e.Client.Create(e.Context, &etcdCertsConfigmap)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			e.Log.Info("etcd certs configmap is already exist", "etcdCluster", etcdClusterName)
		} else {
			e.Log.Error(err, "create etcd certs configmap failure", "etcdCluster", etcdClusterName)
			return err
		}
	}
	e.Log.Info("etcd certs configmap create success", "name", etcdCertsConfigmap.Name, "namespace", namespace)

	return nil
}

func (e *EctdDeploy) CreateEtcdStatefulSet(etcdClusterName string, namespace string, replicas int, image string, datadir string) error {
	replicasInt32 := int32(replicas)
	hostPathType := corev1.HostPathDirectoryOrCreate
	selector := map[string]string{"k8s-app": etcdClusterName}
	statefulSet := appsv1.StatefulSet{
		TypeMeta: constants.StatefulsetTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: etcdClusterName,
			Namespace: namespace,
			Labels: selector,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicasInt32,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			ServiceName: etcdClusterName,
		},
	}

	statefulSet.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: etcdClusterName,
			Labels: selector,
		},
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
						{
							PodAffinityTerm: corev1.PodAffinityTerm{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{
										{
											Key: "k8s-app",
											Operator: metav1.LabelSelectorOpIn,
											Values: []string{etcdClusterName},
										},
									},
								},
								TopologyKey: "kubernetes.io/hostname",
							},
							Weight: 100,
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name: "etcd",
					Image: image,
					ImagePullPolicy: "IfNotPresent",
					Command: []string{
						"/bin/sh", "-ec",
						constants.EtcdStartCommand,
					},
					Lifecycle: &corev1.Lifecycle{
						PreStop: &corev1.Handler{
							Exec: &corev1.ExecAction{
								Command: []string{
									"/bin/sh", "-ec",
									constants.EtcdPreStopCommand,
								},
							},
						},
					},
					Env: []corev1.EnvVar{
						{Name:"CLUSTER_NAME",ValueFrom:&corev1.EnvVarSource{FieldRef:&corev1.ObjectFieldSelector{FieldPath:"metadata.name"}},},
						{Name:"CLUSTER_NAMESPACE",ValueFrom:&corev1.EnvVarSource{FieldRef:&corev1.ObjectFieldSelector{FieldPath:"metadata.namespace"}},},
						{Name:"CLUSTER_SIZE",Value: strconv.Itoa(replicas),},
					},
					LivenessProbe: &corev1.Probe{
						Handler: corev1.Handler{
							Exec: &corev1.ExecAction{
								Command: []string{
									"/bin/sh", "-ec",
									`ETCDCTL_API=3 etcdctl --endpoints="https://127.0.0.1:2379" --cacert=/etc/cert/ca.crt --cert=/etc/cert/peer.crt --key=/etc/cert/peer.key endpoint status`,
								},
							},
						},
						FailureThreshold: 3,
						InitialDelaySeconds: 300,
						PeriodSeconds: 60,
						SuccessThreshold: 1,
						TimeoutSeconds: 60,
					},
					Ports: []corev1.ContainerPort{
						{Name: "server", ContainerPort: 2380, Protocol: "TCP"},
						{Name: "client", ContainerPort: 2379, Protocol: "TCP"},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "datadir", MountPath: "/var/lib/etcd"},
						{Name: "cert", MountPath: "/etc/cert"},
						{Name: "hosttime", MountPath: "/etc/localtime"},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "datadir",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: datadir,
							Type: &hostPathType,
						},
					},
				},
				{
					Name: "hosttime",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/etc/localtime",
						},
					},
				},
				{
					Name: "cert",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: fmt.Sprintf("%s-cert", etcdClusterName),
							},
						},
					},
				},
			},
		},
	}

	partition := int32(0)
	statefulSet.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{Partition:&partition},
	}

	err := e.Client.Create(e.Context, &statefulSet)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			e.Log.Info("etcd statefulset is already exist", "etcdCluster", etcdClusterName)
		} else {
			e.Log.Error(err, "create etcd statefulset failure", "etcdCluster", etcdClusterName)
			return err
		}
	}
	e.Log.Info("etcd statefulset create success", "name", statefulSet.Name, "namespace", namespace)

	return nil
}

func (e *EctdDeploy) CheckEtcdReady(etcdClusterName string, namespace string, replicas int) error {
	var wg sync.WaitGroup
	var err error
	wg.Add(1)
	go func(wg *sync.WaitGroup, err *error, etcdClusterName string, namespace string, replicas int) {
		timeout := time.After(time.Minute * 3)
		defer wg.Done()
		for {
			select {
			case <-timeout:
				errNew := fmt.Errorf("check etcd ready timeout")
				err = &errNew
				return
			default:
				ready := true
				for i:=0;i<replicas;i++ {
					domain := fmt.Sprintf("%s-%d.%s.%s", etcdClusterName, i, etcdClusterName, namespace)
					telnetResult := Telnet(domain, "2379")
					e.Log.Info("check etcdcluster 2379 port", "domain", domain, "telnet result", telnetResult)
					if ! telnetResult {
						ready = false
					}
				}

				if ready {
					return
				}

				time.Sleep(time.Second * 5)
			}
		}
	}(&wg, &err, etcdClusterName, namespace, replicas)

	wg.Wait()

	if err != nil {
		return err
	}

	return nil
}

func (e *EctdDeploy) DeleteEtcd(etcdClusterName string, namespace string) error {
	// delete statefulset
	statefulSet := appsv1.StatefulSet{
		TypeMeta: constants.StatefulsetTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcdClusterName,
			Namespace: namespace,
		},
	}

	err := e.Client.Delete(e.Context, &statefulSet)
	if IgnoreNotFound(err) != nil {
		return err
	}

	// delete cert configmap
	configmap := corev1.ConfigMap{
		TypeMeta: constants.ConfigmapTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-cert", etcdClusterName),
			Namespace: namespace,
		},
	}
	err = e.Client.Delete(e.Context, &configmap)
	if IgnoreNotFound(err) != nil {
		return err
	}

	// delete svc
	headLessService := corev1.Service{
		TypeMeta: constants.ServiceTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: etcdClusterName,
			Namespace: namespace,
		},
	}

	err = e.Client.Delete(e.Context, &headLessService)
	if IgnoreNotFound(err) != nil {
		e.Log.Error(err, "delete etcd headless service failure", "etcdCluster", etcdClusterName)
		return err
	}

	clientService := corev1.Service{
		TypeMeta: constants.ServiceTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-client", etcdClusterName),
			Namespace: namespace,
		},
	}

	err = e.Client.Delete(e.Context, &clientService)
	if IgnoreNotFound(err) != nil {
		e.Log.Error(err, "delete etcd client service failure", "etcdCluster", etcdClusterName)
		return err
	}

	return nil
}


