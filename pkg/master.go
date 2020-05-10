package pkg

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	crdv1 "github.com/cxwen/matrix/api/v1"
	"github.com/cxwen/matrix/common/constants"
	. "github.com/cxwen/matrix/common/utils"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	rbachelper "k8s.io/kubernetes/pkg/apis/rbac/v1"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"sync"
	"time"
)

type Master interface {
	CreateService(string, string) error
	CreateCerts(string, string) error
	CreateKubeconfig(string, string) error
	CreateDeployment(string, string, string, int, string, *crdv1.ImageRepo, string) error
	MasterInit(string, string, *crdv1.ImageRepo) error
	CheckMasterRunning(string, string) error
	DeleteMaster(string, string) error
}

type MasterDedploy struct {
	context.Context
	client.Client
	Log             logr.Logger
	CaCert          *x509.Certificate
	CaKey           *rsa.PrivateKey

	MasterCrd       *crdv1.Master
	AdminKubeconfig string
	MasterClient    runtimeclient.Client
}

func (m *MasterDedploy) CreateService(name string, namespace string) error {
	svc := corev1.Service{
		TypeMeta: constants.ServiceTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": name,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Port: 6443,
					TargetPort: intstr.FromInt(6443),
				},
			},
			Selector: map[string]string{
				"k8s-app": name,
			},
		},
	}

	err := m.Client.Create(m.Context, &svc)
	if err != nil {
		m.Log.Error(err, "create master service failure", "name", name)
		return err
	}

	return nil
}

func (m *MasterDedploy) CreateCerts(name string, namespace string) error {
	caCfg := Config{
		CommonName: "kubernetes",
	}
	caCert, caKey, err := GenerateCaCert(caCfg)
	if err != nil {
		return err
	}

	m.CaCert = caCert
	m.CaKey = caKey

	frontProxyCaCfg := Config{
		CommonName: "kubernetes",
	}
	frontProxyCaCert, frontProxyCaKey, err := GenerateCaCert(frontProxyCaCfg)
	if err != nil {
		return err
	}

	// create apiserver certs
	apiserverDnsNames := []string{
		"kubernetes","kubernetes.default","kubernetes.default.svc","kubernetes.default.svc.cluster.local",
	}
	apiserverAlternateIPs := []net.IP{
		net.ParseIP("127.0.0.1"),
		net.ParseIP("10.96.0.1"),
	}
	apiserverCfg := Config{
		CommonName: "kube-apiserver",
		AltNames:   AltNames{
			DNSNames: apiserverDnsNames,
			IPs: apiserverAlternateIPs,
		},
		Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	apiserverCert, apiserverKey, err := GenerateCert(apiserverCfg, caCert, caKey)
	if err != nil {
		return err
	}

	// create apiserver kubelet client
	apiserverKubeletClientCfg := Config{
		CommonName:"kube-apiserver-kubelet-client",
		Organization: []string{"system:masters"},
		Usages:[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	apiserverKubeletClientCert, apiserverKubeletClientKey, err := GenerateCert(apiserverKubeletClientCfg, caCert, caKey)
	if err != nil {
		return err
	}

	// create front proxy client
	frontProxyClientCfg := Config{
		CommonName:   "front-proxy-client",
		Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	frontProxyClientCert, frontProxyClientKey, err := GenerateCert(frontProxyClientCfg, frontProxyCaCert, frontProxyCaKey)
	if err != nil {
		return err
	}

	saKey, err := GenerateServiceAccountKey()
	if err != nil {
		return err
	}

	saPub, err := EncodePublicKeyPEM(&saKey.PublicKey)
	if err != nil {
		return err
	}

	//apiserver.crt apiserver-kubelet-client.crt  ca.crt  front-proxy-ca.key  front-proxy-client.key  sa.pub
	//apiserver.key apiserver-kubelet-client.key  ca.key  front-proxy-ca.crt  front-proxy-client.crt  sa.key
	k8sCerts := map[string]string{
		"ca.crt":                       string(EncodeCertPEM(caCert)),
		"ca.key":                       string(EncodePrivateKeyPEM(caKey)),
		"front-proxy-ca.crt":           string(EncodeCertPEM(frontProxyCaCert)),
		"front-proxy-ca.key":           string(EncodePrivateKeyPEM(frontProxyCaKey)),
		"apiserver.crt":                string(EncodeCertPEM(apiserverCert)),
		"apiserver.key":                string(EncodePrivateKeyPEM(apiserverKey)),
		"apiserver-kubelet-client.crt": string(EncodeCertPEM(apiserverKubeletClientCert)),
		"apiserver-kubelet-client.key": string(EncodePrivateKeyPEM(apiserverKubeletClientKey)),
		"front-proxy-client.crt":       string(EncodeCertPEM(frontProxyClientCert)),
		"front-proxy-client.key":       string(EncodePrivateKeyPEM(frontProxyClientKey)),
		"sa.pub":                       string(saPub),
		"sa.key":                       string(EncodePrivateKeyPEM(saKey)),
	}

	k8sCertsConfigmap := corev1.ConfigMap{
		TypeMeta: constants.ConfigmapTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-cert", name),
			Namespace: namespace,
		},
		Data: k8sCerts,
	}

	err = m.Client.Create(m.Context, &k8sCertsConfigmap)
	if err != nil {
		return err
	}

	return nil
}

func (m *MasterDedploy) CreateKubeconfig(name string, namespace string) error {
	adminKubeconfig, err := CreateAdminKubeconfig(m.CaCert, m.CaKey)
	if err != nil {
		return err
	}
	m.AdminKubeconfig = string(adminKubeconfig)

	controllerManagerKUbeconfig, err := CreateControllerManagerKUbeconfig(m.CaCert, m.CaKey)
	if err != nil {
		return err
	}

	schedulerKubeconfig, err := CreateSchedulerKubeconfig(m.CaCert, m.CaKey)
	if err != nil {
		return err
	}

	k8sKubeconfigConfigmap := corev1.ConfigMap{
		TypeMeta: constants.ConfigmapTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-kubeconfig", name),
			Namespace: namespace,
		},
		Data: map[string]string{
			AdminKubeConfigFileName: string(adminKubeconfig),
			ControllerManagerKubeConfigFileName: string(controllerManagerKUbeconfig),
			SchedulerKubeConfigFileName: string(schedulerKubeconfig),
		},
	}

	err = m.Client.Create(m.Context, &k8sKubeconfigConfigmap)
	if err != nil {
		return err
	}

	return nil
}

func (m *MasterDedploy) CreateDeployment(name string, namespace string, version string, replicas int, imageRegistry string, imageRepo *crdv1.ImageRepo, etcdClusterName string) error {
	replicasInt32 := int32(replicas)
	selector := map[string]string{"app": name}
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Namespace:namespace,
			Labels: selector,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicasInt32,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
		},
	}

	deployment.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
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
											Key: "app",
											Operator: metav1.LabelSelectorOpIn,
											Values: []string{name},
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
		},
	}

	kubeapiserContainer := getAapiserverContainer(fmt.Sprintf("%s/%s:%s", imageRegistry, imageRepo.Apiserver, version), etcdClusterName)
	controllerContainer := getControllerManagerContainer(fmt.Sprintf("%s/%s:%s", imageRegistry, imageRepo.ControllerManager, version))
	schedulerContainer  := getSchedulerContainer(fmt.Sprintf("%s/%s:%s", imageRegistry, imageRepo.Scheduler, version))
	deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, kubeapiserContainer)
	deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, controllerContainer)
	deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, schedulerContainer)

	deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "hosttime",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/localtime",
				},
			},
		},
		{
			Name: "k8s-cert",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-cert", name),
					},
				},
			},
		},
		{
			Name: "kubeconfig",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-kubeconfig", name),
					},
				},
			},
		},
		{
			Name: "etcd-cert",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-cert", etcdClusterName),
					},
				},
			},
		},
	}

	err := m.Client.Create(m.Context, &deployment)
	if err != nil {
		return err
	}

	return nil
}

func (m *MasterDedploy) MasterInit(version string, imageRegistry string, imageRepo *crdv1.ImageRepo) error {
	// init
	// create default namespace
	namespaceList := []string{"default","kube-system","kube-public"}
	for _, ns := range namespaceList {
		nsObject := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
			},
		}
		err := m.MasterClient.Create(m.Context, &nsObject)
		if err != nil {
			return err
		}
	}

	// configure cluster-info
	err := m.createAndUpdateClusterInfo(m.AdminKubeconfig)
	if err != nil {
		return err
	}

	// create kube-proxy daemonset
	err = m.createKubeproxyDaemonset(fmt.Sprintf("%s/%s:%s",imageRegistry, imageRepo, version))
	if err != nil {
		return err
	}

	return nil
}

func (m *MasterDedploy) CheckMasterRunning(name string, namespace string) error {
	masterDeployment := &appsv1.Deployment{}
	deploymentOk := client.ObjectKey{Name: fmt.Sprintf("%s", name), Namespace: namespace}

	var err error
	var wg sync.WaitGroup
	wg.Add(1)
	go func(wg *sync.WaitGroup, err *error, masterDeployment *appsv1.Deployment, deploymentOk client.ObjectKey) {
		timeout := time.After(time.Minute * 5)

		for {
			select {
			case <-timeout:
				errNew := fmt.Errorf("check master deployment running timeout")
				err = &errNew
				break
			default:
				getErr := m.Client.Get(m.Context, deploymentOk, masterDeployment)
				if getErr != nil {
					errNew := fmt.Errorf("get master deployment failure, error: %v\n", getErr)
					err = &errNew
					break
				}

				if *masterDeployment.Spec.Replicas == masterDeployment.Status.AvailableReplicas {
					break
				}

				time.Sleep(time.Second * 2)
			}
		}

		wg.Done()
	}(&wg, &err, masterDeployment, deploymentOk)

	wg.Wait()

	if err != nil {
		return err
	}

	return nil

}

func (m *MasterDedploy) DeleteMaster(name string, namespace string) error {
	svc := corev1.Service{
		TypeMeta: constants.ServiceTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := m.Client.Delete(m.Context, &svc)
	if err != nil {
		return err
	}

	k8sCertsConfigmap := corev1.ConfigMap{
		TypeMeta: constants.ConfigmapTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-cert", name),
			Namespace: namespace,
		},
	}
	err = m.Client.Delete(m.Context, &k8sCertsConfigmap)
	if err != nil {
		return err
	}

	k8sKubeconfigConfigmap := corev1.ConfigMap{
		TypeMeta: constants.ConfigmapTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-kubeconfig", name),
			Namespace: namespace,
		},
	}
	err = m.Client.Delete(m.Context, &k8sKubeconfigConfigmap)
	if err != nil {
		return err
	}

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Namespace:namespace,
		},
	}
	err = m.Client.Delete(m.Context, &deployment)
	if err != nil {
		return err
	}

	return nil
}

func (m *MasterDedploy) createAndUpdateClusterInfo(adminKubeconfig string) error {
	err := WriteFile("/etc/kubernetes/admin.conf", []byte(adminKubeconfig))
	if err != nil {
		return fmt.Errorf("create /etc/kubernetes/admin failure for cluster-info, error: %v\n", err)
	}

	cmd := "kubeadm init phase bootstrap-token"
	out, err := ExecCmd(cmd)
	if err != nil {
		return fmt.Errorf("exec cmd [%s] failure, error: %v\n", cmd, err)
	}

	m.Log.Info("configure cluster-info success", "out", out)

	clusterInfoCm := corev1.ConfigMap{}
	getOk := client.ObjectKey{Name: fmt.Sprintf("%s", m.MasterCrd.Name), Namespace: m.MasterCrd.Namespace}
	err = m.Client.Get(m.Context, getOk, &clusterInfoCm)
	if err != nil {
		return err
	}

	svc := corev1.Service{}
	getSvcOk := client.ObjectKey{Name: fmt.Sprintf("%s", m.MasterCrd.Name), Namespace: m.MasterCrd.Namespace}
	err = m.Client.Get(m.Context, getSvcOk, &svc)
	if err != nil {
		return err
	}

	kubeconfigConfKey := "kubeconfig.conf"
	kubeconfigConf := clusterInfoCm.Data[kubeconfigConfKey]
	newServer := fmt.Sprintf("server: https://%s:%s",m.MasterCrd.Spec.Expose.Node[0], svc.Spec.Ports[0].TargetPort.String())
	newKubeconfigConf := strings.Replace(kubeconfigConf, "server: https://127.0.0.1:6443", newServer, -1)
	clusterInfoCm.Data[kubeconfigConfKey] = newKubeconfigConf

	err = m.Client.Update(m.Context, &clusterInfoCm)
	if err != nil {
		return err
	}

	return nil
}

func (m *MasterDedploy) createKubeproxyDaemonset(image string) error {
	svc := corev1.Service{}
	getSvcOk := client.ObjectKey{Name: fmt.Sprintf("%s", m.MasterCrd.Name), Namespace: m.MasterCrd.Namespace}
	err := m.Client.Get(m.Context, getSvcOk, &svc)
	if err != nil {
		return err
	}
	proxyConfigMapBytes, err := ParseTemplate(constants.KubeProxyConfigMap19,
		struct {
			ControlPlaneEndpoint string
			ProxyConfigMap       string
		}{
			ControlPlaneEndpoint: fmt.Sprintf("https://%s:%s",m.MasterCrd.Spec.Expose.Node[0], svc.Spec.Ports[0].TargetPort.String()),
			ProxyConfigMap:       constants.KubeProxyConfigMap,
		})

	if err != nil {
		return errors.Wrap(err, "error when parsing kube-proxy configmap template")
	}

	proxyDaemonSetBytes, err := ParseTemplate(constants.KubeProxyDaemonSet19, struct{ Image, ProxyConfigMap, ProxyConfigMapKey string }{
		Image:             image,
		ProxyConfigMap:    constants.KubeProxyConfigMap,
		ProxyConfigMapKey: constants.KubeProxyConfigMapKey,
	})

	kubeproxyConfigMap := &corev1.ConfigMap{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), proxyConfigMapBytes, kubeproxyConfigMap); err != nil {
		return errors.Wrap(err, "unable to decode kube-proxy configmap")
	}

	// Create the ConfigMap for kube-proxy or update it in case it already exists
	if err := m.Client.Create(m.Context, kubeproxyConfigMap); apierrors.IsAlreadyExists(err) {
		err = m.Client.Update(m.Context, kubeproxyConfigMap)
		if err != nil {
			return err
		}
	}

	kubeproxyDaemonSet := &appsv1.DaemonSet{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), proxyDaemonSetBytes, kubeproxyDaemonSet); err != nil {
		return errors.Wrap(err, "unable to decode kube-proxy daemonset")
	}

	// Create the daemonset for kube-proxy or update it in case it already exists
	if err := m.Client.Create(m.Context, kubeproxyDaemonSet); apierrors.IsAlreadyExists(err) {
		err = m.Client.Update(m.Context, kubeproxyDaemonSet)
		if err != nil {
			return err
		}
	}

	err = m.createRbacRulesForKubeproxy()
	if err != nil {
		return err
	}

	return nil
}

func (m *MasterDedploy) createRbacRulesForKubeproxy() error {
	clusterRolebinding := &rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubeadm:node-proxier",
		},
		RoleRef: rbac.RoleRef{
			APIGroup: rbac.GroupName,
			Kind:     "ClusterRole",
			Name:     constants.KubeProxyClusterRoleName,
		},
		Subjects: []rbac.Subject{
			{
				Kind:      rbac.ServiceAccountKind,
				Name:      constants.KubeProxyServiceAccountName,
				Namespace: metav1.NamespaceSystem,
			},
		},
	}

	if err := m.Client.Create(m.Context, clusterRolebinding); apierrors.IsAlreadyExists(err) {
		err = m.Client.Update(m.Context, clusterRolebinding)
		if err != nil {
			return err
		}
	}

	role := &rbac.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.KubeProxyConfigMap,
			Namespace: metav1.NamespaceSystem,
		},
		Rules: []rbac.PolicyRule{
			rbachelper.NewRule("get").Groups("").Resources("configmaps").Names(constants.KubeProxyConfigMap).RuleOrDie(),
		},
	}

	if err := m.Client.Create(m.Context, role); apierrors.IsAlreadyExists(err) {
		err = m.Client.Update(m.Context, role)
		if err != nil {
			return err
		}
	}

	roleBinding := &rbac.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.KubeProxyConfigMap,
			Namespace: metav1.NamespaceSystem,
		},
		RoleRef: rbac.RoleRef{
			APIGroup: rbac.GroupName,
			Kind:     "Role",
			Name:     constants.KubeProxyConfigMap,
		},
		Subjects: []rbac.Subject{
			{
				Kind: rbac.GroupKind,
				Name: constants.NodeBootstrapTokenAuthGroup,
			},
		},
	}

	if err := m.Client.Create(m.Context, roleBinding); apierrors.IsAlreadyExists(err) {
		err = m.Client.Update(m.Context, roleBinding)
		if err != nil {
			return err
		}
	}

	return nil
}

func getAapiserverContainer(image string, etcdCluster string) corev1.Container {
	return corev1.Container{
		Name: "kube-apiserver",
		Command: []string{
			"kube-apiserver",
			"--allow-privileged=true",
			"--authorization-mode=Node,RBAC",
			"--client-ca-file=/etc/kubernetes/pki/ca.crt",
			"--enable-admission-plugins=NodeRestriction",
			"--enable-bootstrap-token-auth=true",
			"--endpoint-reconciler-type=none",
			"--etcd-cafile=/etc/kubernetes/pki/etcd/ca.crt",
			"--etcd-certfile=/etc/kubernetes/pki/etcd/etcd-client.crt",
			"--etcd-keyfile=/etc/kubernetes/pki/etcd/etcd-client.key",
			"--etcd-servers=https://"+etcdCluster+"-client:2379",
			"--insecure-port=0",
			"--kubelet-client-certificate=/etc/kubernetes/pki/apiserver-kubelet-client.crt",
			"--kubelet-client-key=/etc/kubernetes/pki/apiserver-kubelet-client.key",
			"--kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname",
			"--proxy-client-cert-file=/etc/kubernetes/pki/front-proxy-client.crt",
			"--proxy-client-key-file=/etc/kubernetes/pki/front-proxy-client.key",
			"--requestheader-allowed-names=front-proxy-client",
			"--requestheader-client-ca-file=/etc/kubernetes/pki/front-proxy-ca.crt",
			"--requestheader-extra-headers-prefix=X-Remote-Extra-",
			"--requestheader-group-headers=X-Remote-Group",
			"--requestheader-username-headers=X-Remote-User",
			"--runtime-config=storage.k8s.io/v1alpha1=true",
			"--secure-port=6443",
			"--service-account-key-file=/etc/kubernetes/pki/sa.pub",
			"--service-cluster-ip-range=10.96.0.0/12",
			"--tls-cert-file=/etc/kubernetes/pki/apiserver.crt",
			"--tls-private-key-file=/etc/kubernetes/pki/apiserver.key",
		},
		Image: image,
		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt(6443),
					Scheme: "HTTPS",
				},
			},
			FailureThreshold: 8,
			InitialDelaySeconds: 15,
			TimeoutSeconds: 15,
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("250m"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "k8s-cert", MountPath: "/etc/kubernetes/pki", ReadOnly:true},
			{Name: "kubeconfig", MountPath: "/etc/kubernetes", ReadOnly:true},
			{Name: "etcd-cert", MountPath: "/etc/kubernetes/pki/etcd", ReadOnly:true},
			{Name: "hosttime", MountPath: "/etc/localtime", ReadOnly:true},
		},
	}
}

func getControllerManagerContainer(image string) corev1.Container {
	return corev1.Container{
		Name: "kube-controller-manager",
		Command: []string{
			"kube-controller-manager",
			"--allocate-node-cidrs=true",
			"--authentication-kubeconfig=/etc/kubernetes/controller-manager.conf",
			"--authorization-kubeconfig=/etc/kubernetes/controller-manager.conf",
			"--bind-address=0.0.0.0",
			"--client-ca-file=/etc/kubernetes/pki/ca.crt",
			"--cluster-cidr=100.64.0.0/14",
			"--cluster-signing-cert-file=/etc/kubernetes/pki/ca.crt",
			"--cluster-signing-key-file=/etc/kubernetes/pki/ca.key",
			"--controllers=*,bootstrapsigner,tokencleaner",
			"--kubeconfig=/etc/kubernetes/controller-manager.conf",
			"--leader-elect=true",
			"--node-cidr-mask-size=24",
			"--requestheader-client-ca-file=/etc/kubernetes/pki/front-proxy-ca.crt",
			"--root-ca-file=/etc/kubernetes/pki/ca.crt",
			"--service-account-private-key-file=/etc/kubernetes/pki/sa.key",
			"--use-service-account-credentials=true",
		},
		Image: image,
		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt(10252),
					Scheme: "HTTP",
				},
			},
			FailureThreshold: 8,
			InitialDelaySeconds: 15,
			TimeoutSeconds: 15,
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("200m"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "k8s-cert", MountPath: "/etc/kubernetes/pki", ReadOnly:true},
			{Name: "kubeconfig", MountPath: "/etc/kubernetes", ReadOnly:true},
			{Name: "hosttime", MountPath: "/etc/localtime", ReadOnly:true},
		},
	}
}

func getSchedulerContainer(image string) corev1.Container {
	return corev1.Container{
		Name: "kube-scheduler",
		Command: []string{
			"kube-scheduler",
			"--address=0.0.0.0",
			"--kubeconfig=/etc/kubernetes/scheduler.conf",
			"--leader-elect=true",
		},
		Image: image,
		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt(10251),
					Scheme: "HTTP",
				},
			},
			FailureThreshold: 8,
			InitialDelaySeconds: 15,
			TimeoutSeconds: 15,
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("100m"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "kubeconfig", MountPath: "/etc/kubernetes", ReadOnly:true},
			{Name: "hosttime", MountPath: "/etc/localtime", ReadOnly:true},
		},
	}
}

