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

type EctdCluster struct {
	context.Context
	client.Client
	Log logr.Logger
}

func (e *EctdCluster) CreateService(etcdClusterName string, namespace string) error {
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
		e.Log.Error(err, "create etcd headless service failure", "etcdCluster", etcdClusterName)
		return err
	}

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
		e.Log.Error(err, "create etcd client service failure", "etcdCluster", etcdClusterName)
		return err
	}

	return nil
}

func (e *EctdCluster) CreateEtcdCerts(etcdClusterName string, namespace string) error {
	clientSvc := &corev1.Service{}
	getClientSvcOk := client.ObjectKey{Name: fmt.Sprintf("%s-client", etcdClusterName), Namespace: ""}
	err := e.Client.Get(e.Context, getClientSvcOk, clientSvc)
	if err != nil {
		return err
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
		Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
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
		return err
	}

	return nil
}

func (e *EctdCluster) CreateEtcdStatefulSet(etcdClusterName string, namespace string, replicas int, image string, datadir string) error {
	replicasInt32 := int32(replicas)
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
						etcdStartCommand,
					},
					Lifecycle: &corev1.Lifecycle{
						PreStop: &corev1.Handler{
							Exec: &corev1.ExecAction{
								Command: []string{
									"/bin/sh", "-ec",
									etcdPreStopCommand,
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
		return err
	}

	return nil
}

func (e *EctdCluster) CheckEtcdReady(etcdClusterName string, namespace string, replicas int) error {
	var wg sync.WaitGroup
	var err error
	wg.Add(1)
	go func(wg *sync.WaitGroup, err *error, etcdClusterName string, namespace string, replicas int) {
		timeout := time.After(time.Minute * 5)

		for {
			select {
			case <-timeout:
				errNew := fmt.Errorf("check etcd ready timeout")
				err = &errNew
				break
			default:
				ready := true
				for i:=0;i<replicas;i++ {
					domain := fmt.Sprintf("%s-%d.%s.%s", etcdClusterName, i, etcdClusterName, namespace)
					if ! Telnet(domain, "2379") {
						ready = false
					}
				}

				if ready {
					break
				}

				time.Sleep(time.Second * 2)
			}
		}

		wg.Done()
	}(&wg, &err, etcdClusterName, namespace, replicas)

	wg.Wait()

	if err != nil {
		return err
	}

	return nil
}

func (e *EctdCluster) DeleteEtcd(etcdClusterName string, namespace string) error {
	// delete statefulset
	statefulSet := appsv1.StatefulSet{
		TypeMeta: constants.StatefulsetTypemeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcdClusterName,
			Namespace: namespace,
		},
	}

	err := e.Client.Create(e.Context, &statefulSet)
	if err != nil {
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
	err = e.Client.Create(e.Context, &configmap)
	if err != nil {
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
	if err != nil {
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
	if err != nil {
		e.Log.Error(err, "delete etcd client service failure", "etcdCluster", etcdClusterName)
		return err
	}

	return nil
}

const etcdPreStopCommand  = `|
certConfig="--ca-file=/etc/cert/ca.crt --cert-file=/etc/cert/peer.crt --key-file=/etc/cert/peer.key"
etcdctl --endpoints="https://127.0.0.1:2379" ${certConfig} member remove $(etcdctl --endpoints="https://127.0.0.1:2379" ${certConfig} member list | grep $(hostname) | cut -d':' -f1)`

const etcdStartCommand = `|
HOSTNAME=$(hostname)
index=${HOSTNAME##*-}

len=$((${#HOSTNAME}-2))
CLUSTER_NAME=${HOSTNAME:0:${len}}
nodeDomain=https://${HOSTNAME}.${CLUSTER_NAME}.${CLUSTER_NAMESPACE}
initialAdvertisePeerUrls=${nodeDomain}:2380
advertiseClientUrls=${nodeDomain}:2379
dataDir=/var/lib/etcd
initialCluster=""
initialClusterState="existing"

function EchoLog() {
	logTime=$(date "+%Y-%m-%d %H:%M:%S")
	echo "[ ${logTime} ] $1"
}

function RemoveMember() {
	memberId=""
	for i in $(seq 5); do
		memberId=$(etcdctl --endpoints="https://$2:2379" $1 member list 2>&1 | grep $(hostname) | awk '{print $1}' | sed 's/://g')
		if [ "${memberId}" != "" ]; then break;fi
	done
	if [ "${memberId}" != "" ]; then
		echo "This member ${HOSTNAME} and member id is ${memberId} already exists in the cluster and will be removed ..."
		for i in $(seq 5); do
			removeResult=$(etcdctl --endpoints="https://$2:2379" $1 member remove ${memberId} 2>&1 || true)
			if echo "${removeResult}" | grep "Removed member" &>/dev/null; then echo "${removeResult}"; break; fi
		done
	fi
}

echo "Init node ${HOSTNAME} ..."

echo "-----------------------------------------------------------------------------"
nodeName=${CLUSTER_NAME}-${index}
domain=${nodeName}.${CLUSTER_NAME}.${CLUSTER_NAMESPACE}
EchoLog "Waiting for ${domain} to come up"
while ! nslookup ${domain}; do sleep 3; done
sleep 8
echo "-----------------------------------------------------------------------------"

certConfig="--ca-file=/etc/cert/ca.crt --cert-file=/etc/cert/peer.crt --key-file=/etc/cert/peer.key"
CLUSTER_ENDPOINT=${CLUSTER_NAME}-client.${CLUSTER_NAMESPACE}

# Determine if the cluster already exists
EchoLog "Determine if the cluster already exists ..."
checkEndpoint=1
addEndpoints=""
for i in $(seq 5); do
	listResult=$(etcdctl --endpoints="https://${CLUSTER_ENDPOINT}:2379" ${certConfig} cluster-health 2>&1 || true)
	healthyCount=$(echo "${listResult}" | grep "is healthy:" | wc -l)
	if [ "${healthyCount}" != "0" ]; then
  		addEndpoints=$(echo -e "${listResult}" | head -n1 | awk '{print $9}')
  		checkEndpoint=0
  		break
	fi
done

if [ "${addEndpoints}" == "" ]; then
	addEndpoints="https://${CLUSTER_ENDPOINT}:2379"
else
	RemoveMember "${certConfig}" "${CLUSTER_ENDPOINT}"
fi

if [ "${index}" -eq "0" ]; then
	if [ "${checkEndpoint}" -ne "0" ]; then
		initialCluster="${HOSTNAME}=${nodeDomain}:2380"
  		initialClusterState=new
	else
		EchoLog "etcdctl --endpoints=\"${addEndpoints}\" ${certConfig} member add ${HOSTNAME} ${nodeDomain}:2380"
		addResult=$(etcdctl --endpoints="${addEndpoints}" ${certConfig} member add ${HOSTNAME} ${nodeDomain}:2380)
		echo "${addResult}"
		initialCluster=$(echo "${addResult}" | grep ETCD_INITIAL_CLUSTER= | sed 's/ETCD_INITIAL_CLUSTER=//g;s/"//g')
		echo "initialCluster: $initialCluster"
	fi
else
	echo ""; EchoLog "Waiting for the previous nodes become ready,sleeping..."
	for i in $(seq 0 $((${index} - 1))); do
		nodeName=${CLUSTER_NAME}-${i}
		domain=${nodeName}.${CLUSTER_NAME}.${CLUSTER_NAMESPACE}
		echo "etcdctl --endpoints=\"https://${CLUSTER_ENDPOINT}:2379\" ${certConfig} cluster-health 2>&1 | grep ${domain} | grep \"is health\""
		while true; do
			etcdctl --endpoints="https://${CLUSTER_ENDPOINT}:2379" ${certConfig} cluster-health 2>&1 | grep ${domain} | grep "is health" && break
		sleep 3
		done
	done
	addEndpoints="https://${CLUSTER_NAME}-0.${CLUSTER_NAME}.${CLUSTER_NAMESPACE}"
	echo ""; EchoLog "Check if the etcd cluster is healthy ..."
	while true; do
		healthCheck=$(etcdctl --endpoints="${addEndpoints}:2379" ${certConfig} cluster-health 2>&1 | tail -n1)
		if [ "${healthCheck}" == "cluster is healthy" ]; then
			sleep 3; break
  		fi
	done
	echo ""; EchoLog "The etcd cluster is healthy, next add member ..."
	echo "etcdctl --endpoints=\"${addEndpoints}:2379\" ${certConfig} member add ${HOSTNAME} ${nodeDomain}:2380"
	addResult=""
	while true; do
  		addResult=$(etcdctl --endpoints="${addEndpoints}:2379" ${certConfig} member add ${HOSTNAME} ${nodeDomain}:2380 || true)
  		if [ "$?" == "0" ]; then break; fi
	done
	echo "${addResult}"
	EchoLog "The member ${HOSTNAME} has been added to cluster"
	initialCluster=$(echo "${addResult}" | grep ETCD_INITIAL_CLUSTER= | sed 's/ETCD_INITIAL_CLUSTER=//g;s/"//g')
	echo "initialCluster: $initialCluster"
fi

rm -rf ${dataDir}/*
mkdir -p ${dataDir}

echo ""
echo "=============================================================================="

echo ""
echo "etcd --name=${HOSTNAME} \\"
echo "--initial-advertise-peer-urls=${initialAdvertisePeerUrls} \\"
echo "--listen-peer-urls=https://0.0.0.0:2380 \\"
echo "--listen-client-urls=https://0.0.0.0:2379 \\"
echo "--advertise-client-urls=${advertiseClientUrls} \\"
echo "--initial-cluster-token=${CLUSTER_NAME} \\"
echo "--data-dir=${dataDir} \\"
echo "--initial-cluster=${initialCluster} \\"
echo "--initial-cluster-state=${initialClusterState} \\"
echo "--client-cert-auth=true \\"
echo "--peer-client-cert-auth=true \\"
echo "--peer-cert-file=/etc/cert/peer.crt \\"
echo "--peer-key-file=/etc/cert/peer.key \\"
echo "--peer-trusted-ca-file=/etc/cert/ca.crt \\"
echo "--trusted-ca-file=/etc/cert/ca.crt \\"
echo "--cert-file=/etc/cert/server.crt \\"
echo "--key-file=/etc/cert/server.key"
echo ""
echo "=============================================================================="

echo ""
exec etcd --name=${HOSTNAME} \
  --initial-advertise-peer-urls=${initialAdvertisePeerUrls} \
  --listen-peer-urls=https://0.0.0.0:2380 \
  --listen-client-urls=https://0.0.0.0:2379 \
  --advertise-client-urls=${advertiseClientUrls} \
  --initial-cluster-token=${CLUSTER_NAME} \
  --data-dir=${dataDir} \
  --initial-cluster=${initialCluster} \
  --initial-cluster-state=${initialClusterState} \
  --client-cert-auth=true \
  --peer-client-cert-auth=true \
  --peer-cert-file=/etc/cert/peer.crt \
  --peer-key-file=/etc/cert/peer.key \
  --peer-trusted-ca-file=/etc/cert/ca.crt \
  --trusted-ca-file=/etc/cert/ca.crt \
  --cert-file=/etc/cert/server.crt \
  --key-file=/etc/cert/server.key`


