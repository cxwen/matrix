package pkg

import (
	"context"
	"fmt"
	"github.com/cxwen/matrix/common/constants"
	. "github.com/cxwen/matrix/common/utils"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubernetes/pkg/apis/rbac"
	"k8s.io/kubernetes/pkg/registry/core/service/ipallocator"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Dns interface {
	Create(string, int, string, string, string) error
	Delete(string, string, string, string) error
}

type DnsDedploy struct {
	context.Context
	client.Client
	Log logr.Logger

	MatrixClient client.Client
}

func (d *DnsDedploy) Create(dnsType string, replicas int, version string, imageRegistry string, imageRepo string) error {
	corednsDeploymentBytes, coreDNSConfigMapBytes, coreDNSServiceBytes, err := getCorednsYamlBytes(imageRegistry, imageRepo, version)
	if err != nil {
		return errors.Wrap(err, "get coredns yaml template failer")
	}

	if err := d.createCoreDNSAddon(corednsDeploymentBytes, coreDNSServiceBytes, coreDNSConfigMapBytes); err != nil {
		return err
	}
	fmt.Println("[addons] Applied essential addon: CoreDNS")

	return nil
}

func (d *DnsDedploy) Delete(dnsType string, imageRegistry string, imageRepo string, version string) error {
	corednsDeploymentBytes, coreDNSConfigMapBytes, coreDNSServiceBytes, err := getCorednsYamlBytes(imageRegistry, imageRepo, version)
	if err != nil {
		return errors.Wrap(err, "get coredns yaml template failer")
	}

	coreDNSDeployment := &appsv1.Deployment{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), corednsDeploymentBytes, coreDNSDeployment); err != nil {
		return errors.Wrapf(err, "%s Deployment", constants.UnableToDecodeCoreDNS)
	}

	if err := d.MatrixClient.Delete(d.Context, coreDNSDeployment); err != nil {
		return err
	}

	coreDNSConfigMap := &corev1.ConfigMap{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), coreDNSConfigMapBytes, coreDNSConfigMap); err != nil {
		return errors.Wrapf(err, "%s ConfigMap", constants.UnableToDecodeCoreDNS)
	}

	if err := d.MatrixClient.Delete(d.Context, coreDNSConfigMap); IgnoreNotFound(err) != nil {
		return err
	}

	coreDNSClusterRoles := &rbac.ClusterRole{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), []byte(constants.CoreDNSClusterRole), coreDNSClusterRoles); err != nil {
		return errors.Wrapf(err, "%s ClusterRole", constants.UnableToDecodeCoreDNS)
	}

	if err := d.MatrixClient.Delete(d.Context, coreDNSClusterRoles); IgnoreNotFound(err) != nil {
		return err
	}

	coreDNSClusterRolesBinding := &rbac.ClusterRoleBinding{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), []byte(constants.CoreDNSClusterRoleBinding), coreDNSClusterRolesBinding); err != nil {
		return errors.Wrapf(err, "%s ClusterRoleBinding", constants.UnableToDecodeCoreDNS)
	}

	if err := d.MatrixClient.Delete(d.Context, coreDNSClusterRolesBinding); IgnoreNotFound(err) != nil {
		return err
	}

	coreDNSServiceAccount := &corev1.ServiceAccount{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), []byte(constants.CoreDNSServiceAccount), coreDNSServiceAccount); err != nil {
		return errors.Wrapf(err, "%s ServiceAccount", constants.UnableToDecodeCoreDNS)
	}

	if err := d.MatrixClient.Delete(d.Context, coreDNSServiceAccount); IgnoreNotFound(err) != nil {
		return err
	}

	coreDNSService := &corev1.Service{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), coreDNSServiceBytes, coreDNSService); err != nil {
		return errors.Wrap(err, "unable to decode the DNS service")
	}

	if err := d.MatrixClient.Delete(d.Context, coreDNSService); IgnoreNotFound(err) != nil {
		return err
	}

	return nil
}

func (d *DnsDedploy) createCoreDNSAddon(deploymentBytes, serviceBytes, configBytes []byte) error {
	coreDNSConfigMap := &corev1.ConfigMap{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), configBytes, coreDNSConfigMap); err != nil {
		return errors.Wrapf(err, "%s ConfigMap", constants.UnableToDecodeCoreDNS)
	}

	// Create the ConfigMap for CoreDNS or retain it in case it already exists
	if err := CreateOrRetainConfigMap(d.Context, d.MatrixClient, coreDNSConfigMap); err != nil {
		return err
	}

	coreDNSClusterRoles := &rbac.ClusterRole{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), []byte(constants.CoreDNSClusterRole), coreDNSClusterRoles); err != nil {
		return errors.Wrapf(err, "%s ClusterRole", constants.UnableToDecodeCoreDNS)
	}

	// Create the Clusterroles for CoreDNS or update it in case it already exists
	if err := CreateOrUpdateClusterRole(d.Context, d.MatrixClient, coreDNSClusterRoles); err != nil {
		return err
	}

	coreDNSClusterRolesBinding := &rbac.ClusterRoleBinding{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), []byte(constants.CoreDNSClusterRoleBinding), coreDNSClusterRolesBinding); err != nil {
		return errors.Wrapf(err, "%s ClusterRoleBinding", constants.UnableToDecodeCoreDNS)
	}

	// Create the Clusterrolebindings for CoreDNS or update it in case it already exists
	if err := CreateOrUpdateClusterRoleBinding(d.Context, d.MatrixClient, coreDNSClusterRolesBinding); err != nil {
		return err
	}

	coreDNSServiceAccount := &corev1.ServiceAccount{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), []byte(constants.CoreDNSServiceAccount), coreDNSServiceAccount); err != nil {
		return errors.Wrapf(err, "%s ServiceAccount", constants.UnableToDecodeCoreDNS)
	}

	// Create the ConfigMap for CoreDNS or update it in case it already exists
	if err := CreateOrUpdateServiceAccount(d.Context, d.MatrixClient, coreDNSServiceAccount); err != nil {
		return err
	}

	coreDNSDeployment := &appsv1.Deployment{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), deploymentBytes, coreDNSDeployment); err != nil {
		return errors.Wrapf(err, "%s Deployment", constants.UnableToDecodeCoreDNS)
	}

	// Create the Deployment for CoreDNS or update it in case it already exists
	if err := CreateOrUpdateDeployment(d.Context, d.MatrixClient, coreDNSDeployment); err != nil {
		return err
	}

	coreDNSService := &corev1.Service{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), serviceBytes, coreDNSService); err != nil {
		return errors.Wrap(err, "unable to decode the DNS service")
	}

	return createDNSService(d.Context, d.MatrixClient, coreDNSService)
}

func GetDNSIP(svcSubnet string) (net.IP, error) {
	// Get the service subnet CIDR
	_, svcSubnetCIDR, err := net.ParseCIDR(svcSubnet)
	if err != nil {
		return nil, errors.Wrapf(err, "couldn't parse service subnet CIDR %q", svcSubnet)
	}

	// Selects the 10th IP in service subnet CIDR range as dnsIP
	dnsIP, err := ipallocator.GetIndexedIP(svcSubnetCIDR, 10)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get tenth IP address from service subnet CIDR %s", svcSubnetCIDR.String())
	}

	return dnsIP, nil
}

func CreateOrRetainConfigMap(ctx context.Context, runtimeclient client.Client, object *corev1.ConfigMap) error {
	getOk := client.ObjectKey{Name: fmt.Sprintf("%s", object.Name), Namespace: object.Namespace}
	if err := runtimeclient.Get(ctx, getOk, object); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil
		}
		if err := runtimeclient.Create(ctx, object); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return errors.Wrap(err, "unable to create configmap")
			}
		}
	}
	return nil
}

func CreateOrUpdateClusterRole(ctx context.Context, runtimeclient client.Client, object *rbac.ClusterRole) error {
	getOk := client.ObjectKey{Name: fmt.Sprintf("%s", object.Name), Namespace: object.Namespace}
	if err := runtimeclient.Get(ctx, getOk, object); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil
		}
		if err := runtimeclient.Create(ctx, object); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return errors.Wrap(err, "unable to create clusterrole")
			}
		}
	}
	return nil
}

func CreateOrUpdateClusterRoleBinding(ctx context.Context, runtimeclient client.Client, object *rbac.ClusterRoleBinding) error {
	getOk := client.ObjectKey{Name: fmt.Sprintf("%s", object.Name), Namespace: object.Namespace}
	if err := runtimeclient.Get(ctx, getOk, object); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil
		}
		if err := runtimeclient.Create(ctx, object); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return errors.Wrap(err, "unable to create clusterrolebinding")
			}
		}
	}
	return nil
}

func CreateOrUpdateServiceAccount(ctx context.Context, runtimeclient client.Client, object *corev1.ServiceAccount) error {
	getOk := client.ObjectKey{Name: fmt.Sprintf("%s", object.Name), Namespace: object.Namespace}
	if err := runtimeclient.Get(ctx, getOk, object); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil
		}
		if err := runtimeclient.Create(ctx, object); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return errors.Wrap(err, "unable to create service account")
			}
		}
	}
	return nil
}

func CreateOrUpdateDeployment(ctx context.Context, runtimeclient client.Client, object *appsv1.Deployment) error {
	getOk := client.ObjectKey{Name: fmt.Sprintf("%s", object.Name), Namespace: object.Namespace}
	if err := runtimeclient.Get(ctx, getOk, object); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil
		}
		if err := runtimeclient.Create(ctx, object); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return errors.Wrap(err, "unable to create coredns deployment")
			}
		}
	}
	return nil
}

func createDNSService(ctx context.Context, runtimeclient client.Client, object *corev1.Service) error {
	getOk := client.ObjectKey{Name: fmt.Sprintf("%s", object.Name), Namespace: object.Namespace}
	if err := runtimeclient.Get(ctx, getOk, object); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil
		}
		if err := runtimeclient.Create(ctx, object); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return errors.Wrap(err, "unable to create coredns service")
			}
		}
	}
	return nil
}

func getCorednsYamlBytes(imageRegistry string, imageRepo string, version string) ([]byte, []byte, []byte, error) {
	// Get the YAML manifest
	corednsDeploymentBytes, err := ParseTemplate(constants.CoreDNSDeployment, struct{ DeploymentName, Image, ControlPlaneTaintKey string }{
		DeploymentName:       constants.CoreDNSDeploymentName,
		Image:                fmt.Sprintf("%s/%s:%s", imageRegistry, imageRepo, version),
		ControlPlaneTaintKey: constants.LabelNodeRoleMaster,
	})

	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error when parsing CoreDNS deployment template")
	}

	// Get the config file for CoreDNS
	coreDNSConfigMapBytes, err := ParseTemplate(constants.CoreDNSConfigMap, struct{ DNSDomain, UpstreamNameserver, Federation, StubDomain string }{
		DNSDomain:          "cluster.local",
		UpstreamNameserver: "/etc/resolv.conf",
		Federation:         "",
		StubDomain:         "",
	})
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error when parsing CoreDNS configMap template")
	}

	dnsip, err := GetDNSIP("10.96.0.0/12")
	if err != nil {
		return nil, nil, nil, err
	}

	coreDNSServiceBytes, err := ParseTemplate(constants.KubeDNSService, struct{ DNSIP string }{
		DNSIP: dnsip.String(),
	})
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error when parsing CoreDNS service template")
	}

	return corednsDeploymentBytes, coreDNSConfigMapBytes, coreDNSServiceBytes, nil
}