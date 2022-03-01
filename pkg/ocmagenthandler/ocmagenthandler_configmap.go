package ocmagenthandler

import (
	"context"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1 "github.com/openshift/api/config/v1"

	ocmagentv1alpha1 "github.com/openshift/ocm-agent-operator/pkg/apis/ocmagent/v1alpha1"
	oah "github.com/openshift/ocm-agent-operator/pkg/consts/ocmagenthandler"
)

func buildOCMAgentConfigMap(ocmAgent ocmagentv1alpha1.OcmAgent, clusterId string) corev1.ConfigMap {
	namespacedName := oah.BuildNamespacedName(ocmAgent.Spec.OcmAgentConfig)
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Data: map[string]string{
			oah.OCMAgentConfigServicesKey: strings.Join(ocmAgent.Spec.AgentConfig.Services, ","),
			oah.OCMAgentConfigURLKey:      ocmAgent.Spec.AgentConfig.OcmBaseUrl,
			oah.OCMAgentConfigClusterID: clusterId,
		},
	}
	return cm
}

// ensureConfigMap ensures that an OCMAgent ConfigMap exists on the cluster
// and that its configuration matches what is expected.
func (o *ocmAgentHandler) ensureConfigMap(ocmAgent ocmagentv1alpha1.OcmAgent) error {
	namespacedName := oah.BuildNamespacedName(ocmAgent.Spec.OcmAgentConfig)
	foundResource := &corev1.ConfigMap{}

	// Determine the cluster ID, used as a configmap value
	cv, err := o.fetchClusterVersion()
	if err != nil {
		o.Log.Error(err, "unable to fetch cluster ID for creating configmap")
		return err
	}
	clusterID := string(cv.Spec.ClusterID)

	populationFunc := func() corev1.ConfigMap {
		return buildOCMAgentConfigMap(ocmAgent, clusterID)
	}

	// Does the resource already exist?
	o.Log.Info("ensuring configmap exists", "resource", namespacedName.String())
	if err := o.Client.Get(o.Ctx, namespacedName, foundResource); err != nil {
		if k8serrors.IsNotFound(err) {
			// It does not exist, so must be created.
			o.Log.Info("An OCMAgent configmap does not exist; will be created.")
			// Populate the resource with the template
			resource := populationFunc()
			// Set the controller reference
			if err := controllerutil.SetControllerReference(&ocmAgent, &resource, o.Scheme); err != nil {
				return err
			}
			// and create it
			err = o.Client.Create(o.Ctx, &resource)
			if err != nil {
				return err
			}
		} else {
			// Return unexpectedly
			return err
		}
	} else {
		// It does exist, check if it is what we expected
		resource := populationFunc()
		if !reflect.DeepEqual(foundResource.Data, resource.Data) {
			// Specs aren't equal, update and fix.
			o.Log.Info("An OCMAgent configmap exists but contains unexpected configuration. Restoring.")
			foundResource = resource.DeepCopy()
			if err = o.Client.Update(context.TODO(), foundResource); err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *ocmAgentHandler) ensureConfigMapDeleted(ocmAgent ocmagentv1alpha1.OcmAgent) error {
	namespacedName := oah.BuildNamespacedName(ocmAgent.Spec.OcmAgentConfig)
	foundResource := &corev1.ConfigMap{}
	// Does the resource already exist?
	o.Log.Info("ensuring configmap removed", "resource", namespacedName.String())
	if err := o.Client.Get(o.Ctx, namespacedName, foundResource); err != nil {
		if !k8serrors.IsNotFound(err) {
			// Return unexpected error
			return err
		} else {
			// Resource deleted
			return nil
		}
	}
	err := o.Client.Delete(o.Ctx, foundResource)
	if err != nil {
		return err
	}
	return nil
}

func (o *ocmAgentHandler) fetchClusterVersion() (*configv1.ClusterVersion, error) {
	cv := &configv1.ClusterVersion{}
	err := o.Client.Get(o.Ctx, types.NamespacedName{Name: "version"}, cv)
	if err != nil {
		return nil, err
	}
	return cv, nil
}
