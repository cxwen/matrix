package pkg

import (
	"context"
	"fmt"
	crdv1 "github.com/cxwen/matrix/api/v1"
	"github.com/cxwen/matrix/common/constants"
	. "github.com/cxwen/matrix/common/utils"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubernetes/pkg/apis/rbac"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NetworkPlugin interface {
	CreateCalico(calicoConfig *crdv1.CalicoConfig) error
	DeleteCalico(calicoConfig *crdv1.CalicoConfig) error
}

type NetworkPluginDedploy struct {
	context.Context
	client.Client
	Log logr.Logger
	MatrixClient client.Client
}

func (n *NetworkPluginDedploy) CreateCalico(calicoConfig *crdv1.CalicoConfig) error {
	n.setDefaultCalicoCalicoConfig(calicoConfig)

	// create or update calico configmap
	err := n.createOrUpdateCalicoConfigMap()
	if err != nil {
		return err
	}

	// create or update crd
	err = n.createOrUpdateCalicoCrds()
	if err != nil {
		return err
	}

	// create of update calico serviceaccount
	err = n.createOrUpdateServiceAcounts()
	if err != nil {
		return err
	}

	// create of update calico clusterrole
	err = n.createOrUpdateClusterRoles()
	if err != nil {
		return err
	}

	// create of update calico ClusterRoleBinding
	err = n.createOrUpdateClusterRoleBindings()
	if err != nil {
		return err
	}

	// create of update calico node daemonset
	err = n.createOrUpdateCalicoNodeDaemonset(calicoConfig)
	if err != nil {
		return err
	}

	// create of update calico kube-controller deployment
	err = n.createOrUpdateCalicoKubeControllerDeployment(calicoConfig)
	if err != nil {
		return err
	}

	return nil
}

func (n *NetworkPluginDedploy) DeleteCalico(calicoConfig *crdv1.CalicoConfig) error {
	n.setDefaultCalicoCalicoConfig(calicoConfig)

	// delete calico node daemonset
	err := n.deleteCalicoNodeDaemonset(calicoConfig)
	if err != nil {
		return err
	}

	// delete calico kube-controller deployment
	err = n.deleteCalicoKubeControllerDeployment(calicoConfig)
	if err != nil {
		return err
	}

	// delete calico configmap
	err = n.deleteCalicoConfigMap()
	if err != nil {
		return err
	}

	// delete crd
	err = n.deleteCalicoCrds()
	if err != nil {
		return err
	}

	// delete calico serviceaccount
	err = n.deleteServiceAcounts()
	if err != nil {
		return err
	}

	// delete calico clusterrole
	err = n.deleteClusterRoles()
	if err != nil {
		return err
	}

	// delete calico ClusterRoleBinding
	err = n.deleteClusterRoleBindings()
	if err != nil {
		return err
	}

	return nil
}

func (n *NetworkPluginDedploy) setDefaultCalicoCalicoConfig(calicoConfig *crdv1.CalicoConfig) {
	if calicoConfig.Version == "" {
		calicoConfig.Version = constants.DefaultCalicoVersion
	}
	if calicoConfig.ImageRegistry == "" {
		calicoConfig.ImageRegistry = constants.DefaultImageRegistry
	}
	if calicoConfig.NodeImageRepo == "" {
		calicoConfig.NodeImageRepo = constants.DefaultNodeImageRepo
	}
	if calicoConfig.InstallCniImageRepo == "" {
		calicoConfig.InstallCniImageRepo = constants.DefaultInstallCniImageRepo
	}
	if calicoConfig.KubeControllerImageRepo == "" {
		calicoConfig.KubeControllerImageRepo = constants.DefaultKubeControllerImageRepo
	}
	if calicoConfig.FlexvolDriverImageRepo == "" {
		calicoConfig.FlexvolDriverImageRepo = constants.DefaultFlexvolDriverImageRepo
	}

	if calicoConfig.Ipv4poolCidr == "" {
		calicoConfig.Ipv4poolCidr = constants.DefaultIpv4poolCidr
	}
	if calicoConfig.Ipv4poolIpip == "" {
		calicoConfig.Ipv4poolIpip = constants.DefaultIpv4poolIpip
	}

	if calicoConfig.IpAutodetectionMethod == "" {
		endpoint := &corev1.Endpoints{}
		getOk := client.ObjectKey{Name: "kubernetes", Namespace: "default"}
		err := n.MatrixClient.Get(n.Context, getOk, endpoint)
		if err != nil {
			calicoConfig.IpAutodetectionMethod = constants.DefaultIpAutodetectionMethod
			n.Log.Error(err, "get kubernetes endpoint failure","set calico default IpAutodetectionMethod", constants.DefaultIpAutodetectionMethod)
		} else {
			calicoConfig.IpAutodetectionMethod = fmt.Sprintf("can-reach=%s",endpoint.Subsets[0].Addresses[0].IP)
		}
	}
}

func (n *NetworkPluginDedploy) createOrUpdateCalicoConfigMap() error {
	calicoConfigmapBytes, err := ParseTemplate(constants.CalicoConfigMap)
	if err != nil {
		return err
	}
	calicoConfigmap := &corev1.ConfigMap{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), calicoConfigmapBytes, calicoConfigmap); err != nil {
		return errors.Wrapf(err, "%s ConfigMap", constants.UnableToDecodeCalico)
	}

	if err = n.MatrixClient.Create(n.Context, calicoConfigmap); err != nil {
		if apierrors.IsAlreadyExists(err) {
			err = n.MatrixClient.Update(n.Context, calicoConfigmap)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) createOrUpdateCalicoCrds() error {
	for _, crdYaml := range constants.CalicoCrds {
		crdBytes, err := ParseTemplate(crdYaml)
		if err != nil {
			return err
		}
		crd := &apiextensions.CustomResourceDefinition{}
		if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), crdBytes, crd); err != nil {
			return errors.Wrapf(err, "%s CustomResourceDefinition", constants.UnableToDecodeCalico)
		}

		if err = n.MatrixClient.Create(n.Context, crd); err != nil {
			if apierrors.IsAlreadyExists(err) {
				err = n.MatrixClient.Update(n.Context, crd)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) createOrUpdateServiceAcounts() error {
	for _, saYaml := range constants.CalicoServiceAcounts {
		saBytes, err := ParseTemplate(saYaml)
		if err != nil {
			return err
		}
		sa := &corev1.ServiceAccount{}
		if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), saBytes, sa); err != nil {
			return errors.Wrapf(err, "%s ServiceAccount", constants.UnableToDecodeCalico)
		}

		if err = n.MatrixClient.Create(n.Context, sa); err != nil {
			if apierrors.IsAlreadyExists(err) {
				err = n.MatrixClient.Update(n.Context, sa)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) createOrUpdateClusterRoles() error {
	for _, crYaml := range constants.CalicoClusterRoles {
		crBytes, err := ParseTemplate(crYaml)
		if err != nil {
			return err
		}
		cr := &rbac.ClusterRole{}
		if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), crBytes, cr); err != nil {
			return errors.Wrapf(err, "%s ClusterRole", constants.UnableToDecodeCalico)
		}

		if err = n.MatrixClient.Create(n.Context, cr); err != nil {
			if apierrors.IsAlreadyExists(err) {
				err = n.MatrixClient.Update(n.Context, cr)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) createOrUpdateClusterRoleBindings() error {
	for _, crbYaml := range constants.CalicoClusterRoleBindings {
		crbBytes, err := ParseTemplate(crbYaml)
		if err != nil {
			return err
		}
		crb := &rbac.ClusterRoleBinding{}
		if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), crbBytes, crb); err != nil {
			return errors.Wrapf(err, "%s ClusterRoleBinding", constants.UnableToDecodeCalico)
		}

		if err = n.MatrixClient.Create(n.Context, crb); err != nil {
			if apierrors.IsAlreadyExists(err) {
				err = n.MatrixClient.Update(n.Context, crb)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) createOrUpdateCalicoNodeDaemonset(calicoConfig *crdv1.CalicoConfig) error {
	calicoNodeDaemonsetBytes, err := ParseTemplate(constants.CalicoNodeDaemonset, struct{
		CalicoInstallCniImage,
		CalicoFlexvolDriverImage,
		CalicoNodeImage,
		CalicoIpAutodetectionMethod,
		CalicoIpv4poolIpip,
		CalicoIpv4poolCidr string
	}{
		CalicoInstallCniImage:       fmt.Sprintf("%s/%s:%s", calicoConfig.ImageRegistry, calicoConfig.InstallCniImageRepo, calicoConfig.Version),
		CalicoFlexvolDriverImage:    fmt.Sprintf("%s/%s:%s", calicoConfig.ImageRegistry, calicoConfig.FlexvolDriverImageRepo, calicoConfig.Version),
		CalicoNodeImage:             fmt.Sprintf("%s/%s:%s", calicoConfig.ImageRegistry, calicoConfig.NodeImageRepo, calicoConfig.Version),
		CalicoIpAutodetectionMethod: calicoConfig.IpAutodetectionMethod,
		CalicoIpv4poolIpip:          calicoConfig.Ipv4poolIpip,
		CalicoIpv4poolCidr:          calicoConfig.Ipv4poolCidr,
	})

	if err != nil {
		return err
	}

	calicoNodeDaemonset := &appsv1.DaemonSet{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), calicoNodeDaemonsetBytes, calicoNodeDaemonset); err != nil {
		return errors.Wrapf(err, "%s ConfigMap", constants.UnableToDecodeCalico)
	}

	if err = n.MatrixClient.Create(n.Context, calicoNodeDaemonset); err != nil {
		if apierrors.IsAlreadyExists(err) {
			err = n.MatrixClient.Update(n.Context, calicoNodeDaemonset)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) createOrUpdateCalicoKubeControllerDeployment(calicoConfig *crdv1.CalicoConfig) error {
	calicoKubeControllersDeploymentBytes, err := ParseTemplate(constants.CalicoKubeControllersDeployment, struct{CalicoKubeControllerImage string}{
		CalicoKubeControllerImage: fmt.Sprintf("%s/%s:%s", calicoConfig.ImageRegistry, calicoConfig.KubeControllerImageRepo, calicoConfig.Version),
	})

	if err != nil {
		return err
	}
	calicoKubeControllersDeployment := &appsv1.Deployment{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), calicoKubeControllersDeploymentBytes, calicoKubeControllersDeployment); err != nil {
		return errors.Wrapf(err, "%s Deployment", constants.UnableToDecodeCalico)
	}

	if err = n.MatrixClient.Create(n.Context, calicoKubeControllersDeployment); err != nil {
		if apierrors.IsAlreadyExists(err) {
			err = n.MatrixClient.Update(n.Context, calicoKubeControllersDeployment)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) deleteCalicoConfigMap() error {
	calicoConfigmapBytes, err := ParseTemplate(constants.CalicoConfigMap)
	if err != nil {
		return err
	}
	calicoConfigmap := &corev1.ConfigMap{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), calicoConfigmapBytes, calicoConfigmap); err != nil {
		return errors.Wrapf(err, "%s ConfigMap", constants.UnableToDecodeCalico)
	}

	if err = n.MatrixClient.Delete(n.Context, calicoConfigmap); err != nil {
		if ! apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) deleteCalicoCrds() error {
	for _, crdYaml := range constants.CalicoCrds {
		crdBytes, err := ParseTemplate(crdYaml)
		if err != nil {
			return err
		}
		crd := &apiextensions.CustomResourceDefinition{}
		if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), crdBytes, crd); err != nil {
			return errors.Wrapf(err, "%s CustomResourceDefinition", constants.UnableToDecodeCalico)
		}

		if err = n.MatrixClient.Delete(n.Context, crd); err != nil {
			if ! apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) deleteServiceAcounts() error {
	for _, saYaml := range constants.CalicoServiceAcounts {
		saBytes, err := ParseTemplate(saYaml)
		if err != nil {
			return err
		}
		sa := &corev1.ServiceAccount{}
		if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), saBytes, sa); err != nil {
			return errors.Wrapf(err, "%s ServiceAccount", constants.UnableToDecodeCalico)
		}

		if err = n.MatrixClient.Delete(n.Context, sa); err != nil {
			if ! apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) deleteClusterRoles() error {
	for _, crYaml := range constants.CalicoClusterRoles {
		crBytes, err := ParseTemplate(crYaml)
		if err != nil {
			return err
		}
		cr := &rbac.ClusterRole{}
		if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), crBytes, cr); err != nil {
			return errors.Wrapf(err, "%s ClusterRole", constants.UnableToDecodeCalico)
		}

		if err = n.MatrixClient.Delete(n.Context, cr); err != nil {
			if ! apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) deleteClusterRoleBindings() error {
	for _, crbYaml := range constants.CalicoClusterRoleBindings {
		crbBytes, err := ParseTemplate(crbYaml)
		if err != nil {
			return err
		}
		crb := &rbac.ClusterRoleBinding{}
		if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), crbBytes, crb); err != nil {
			return errors.Wrapf(err, "%s ClusterRoleBinding", constants.UnableToDecodeCalico)
		}

		if err = n.MatrixClient.Delete(n.Context, crb); err != nil {
			if ! apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) deleteCalicoNodeDaemonset(calicoConfig *crdv1.CalicoConfig) error {
	calicoNodeDaemonsetBytes, err := ParseTemplate(constants.CalicoNodeDaemonset, struct{
		CalicoInstallCniImage,
		CalicoFlexvolDriverImage,
		CalicoNodeImage,
		CalicoIpAutodetectionMethod,
		CalicoIpv4poolIpip,
		CalicoIpv4poolCidr string
	}{
		CalicoInstallCniImage:       fmt.Sprintf("%s/%s:%s", calicoConfig.ImageRegistry, calicoConfig.InstallCniImageRepo, calicoConfig.Version),
		CalicoFlexvolDriverImage:    fmt.Sprintf("%s/%s:%s", calicoConfig.ImageRegistry, calicoConfig.FlexvolDriverImageRepo, calicoConfig.Version),
		CalicoNodeImage:             fmt.Sprintf("%s/%s:%s", calicoConfig.ImageRegistry, calicoConfig.NodeImageRepo, calicoConfig.Version),
		CalicoIpAutodetectionMethod: calicoConfig.IpAutodetectionMethod,
		CalicoIpv4poolIpip:          calicoConfig.Ipv4poolIpip,
		CalicoIpv4poolCidr:          calicoConfig.Ipv4poolCidr,
	})

	if err != nil {
		return err
	}

	calicoNodeDaemonset := &appsv1.DaemonSet{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), calicoNodeDaemonsetBytes, calicoNodeDaemonset); err != nil {
		return errors.Wrapf(err, "%s ConfigMap", constants.UnableToDecodeCalico)
	}

	if err = n.MatrixClient.Delete(n.Context, calicoNodeDaemonset); err != nil {
		if ! apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (n *NetworkPluginDedploy) deleteCalicoKubeControllerDeployment(calicoConfig *crdv1.CalicoConfig) error {
	calicoKubeControllersDeploymentBytes, err := ParseTemplate(constants.CalicoKubeControllersDeployment, struct{CalicoKubeControllerImage string}{
		CalicoKubeControllerImage: fmt.Sprintf("%s/%s:%s", calicoConfig.ImageRegistry, calicoConfig.KubeControllerImageRepo, calicoConfig.Version),
	})

	if err != nil {
		return err
	}
	calicoKubeControllersDeployment := &appsv1.Deployment{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), calicoKubeControllersDeploymentBytes, calicoKubeControllersDeployment); err != nil {
		return errors.Wrapf(err, "%s Deployment", constants.UnableToDecodeCalico)
	}

	if err = n.MatrixClient.Delete(n.Context, calicoKubeControllersDeployment); err != nil {
		if ! apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

