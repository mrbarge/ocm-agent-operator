package resourcemanager

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ocmagentv1alpha1 "github.com/openshift/ocm-agent-operator/pkg/apis/ocmagent/v1alpha1"
	oahconst "github.com/openshift/ocm-agent-operator/pkg/consts/ocmagenthandler"
)

const (
	deploymentRevisionAnnotation = "deployment.kubernetes.io/revision"
)

type OCMDeploymentResourceMgr interface {
	OCMResourceManager
}

type ocmDeploymentResourceMgr struct{}

func NewOCMDeploymentResourceMgr() OCMResourceManager {
	return &ocmDeploymentResourceMgr{}
}

func (r *ocmDeploymentResourceMgr) Build(ocmAgent ocmagentv1alpha1.OcmAgent) (*unstructured.Unstructured, error) {
	namespacedName := oahconst.BuildNamespacedName()
	labels := map[string]string{
		"app": oahconst.OCMAgentName,
	}
	labelSelectors := metav1.LabelSelector{
		MatchLabels: labels,
	}

	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount
	// Define a volume/volume mount for the access token secret
	var secretVolumeSourceDefaultMode int32 = 0600
	tokenSecretVolumeName := ocmAgent.Spec.TokenSecret
	volumes = append(volumes, corev1.Volume{
		Name: tokenSecretVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  ocmAgent.Spec.TokenSecret,
				DefaultMode: &secretVolumeSourceDefaultMode,
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      tokenSecretVolumeName,
		MountPath: filepath.Join(oahconst.OCMAgentSecretMountPath, tokenSecretVolumeName),
	})

	// Define a volume/volume mount for the config
	configVolumeName := ocmAgent.Spec.OcmAgentConfig
	var configVolumeSourceDefaultMode int32 = 0600
	volumes = append(volumes, corev1.Volume{
		Name: configVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: ocmAgent.Spec.OcmAgentConfig,
				},
				DefaultMode: &configVolumeSourceDefaultMode,
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      configVolumeName,
		MountPath: filepath.Join(oahconst.OCMAgentConfigMountPath, configVolumeName),
	})
	// Sort volume slices by name to keep the sequence stable.
	sort.Slice(volumes, func(i, j int) bool {
		return volumes[i].Name < volumes[j].Name
	})
	sort.Slice(volumeMounts, func(i, j int) bool {
		return volumeMounts[i].Name < volumeMounts[j].Name
	})

	// Construct the command arguments of the agent
	ocmAgentCommand := buildOCMAgentArgs(ocmAgent)

	replicas := int32(ocmAgent.Spec.Replicas)
	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &labelSelectors,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Volumes:            volumes,
					ServiceAccountName: oahconst.OCMAgentServiceAccount,
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "node-role.kubernetes.io/infra",
										Operator: corev1.NodeSelectorOpExists,
									}},
								},
								Weight: 1,
							}},
						},
					},
					Tolerations: []corev1.Toleration{{
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
						Key:      "node-role.kubernetes.io/infra",
					}},
					Containers: []corev1.Container{{
						VolumeMounts: volumeMounts,
						Image:        ocmAgent.Spec.OcmAgentImage,
						Command:      ocmAgentCommand,
						Name:         oahconst.OCMAgentName,
						Ports: []corev1.ContainerPort{{
							ContainerPort: oahconst.OCMAgentPort,
							Name:          oahconst.OCMAgentPortName,
						}},
					}},
				},
			},
		},
	}

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dep.DeepCopy())
	if err != nil {
		return nil, err
	}
	unstructuredObj := unstructured.Unstructured{obj}
	unstructuredObj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
	return &unstructuredObj, nil
}

func (r *ocmDeploymentResourceMgr) Changed(current *unstructured.Unstructured, expected *unstructured.Unstructured) (bool, *unstructured.Unstructured, error) {

	gvk := expected.GroupVersionKind()
	if gvk.Group == "apps" && gvk.Kind == "Deployment" {

		changed := false
		currentDeployment := &appsv1.Deployment{}
		expectedDeployment := &appsv1.Deployment{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(current.UnstructuredContent(), currentDeployment); err != nil {
			return false, nil, err
		}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(expected.UnstructuredContent(), expectedDeployment); err != nil {
			return false, nil, err
		}

		// There may be multiple containers eventually, so let's do a loop
		for _, name := range []string{oahconst.OCMAgentName} {
			var curImage, expImage string

			for i, c := range currentDeployment.Spec.Template.Spec.Containers {
				if name == c.Name {
					curImage = currentDeployment.Spec.Template.Spec.Containers[i].Image
					break
				}
			}
			for i, c := range expectedDeployment.Spec.Template.Spec.Containers {
				if name == c.Name {
					expImage = expectedDeployment.Spec.Template.Spec.Containers[i].Image
					break
				}
			}

			if len(curImage) == 0 {
				changed = true
				break
			} else if curImage != expImage {
				changed = true
			}
		}

		// Compare replicas
		if *(currentDeployment.Spec.Replicas) != *(expectedDeployment.Spec.Replicas) {
			changed = true
		}

		// Compare affinity
		if !reflect.DeepEqual(currentDeployment.Spec.Template.Spec.Affinity, expectedDeployment.Spec.Template.Spec.Affinity) {
			changed = true
		}

		// Compare tolerations
		if !reflect.DeepEqual(currentDeployment.Spec.Template.Spec.Tolerations, expectedDeployment.Spec.Template.Spec.Tolerations) {
			changed = true
		}
		if !changed {
			return false, nil, nil
		}

		updated := currentDeployment.DeepCopy()
		updated.Spec.Template.Spec.Tolerations = expectedDeployment.Spec.Template.Spec.Tolerations
		updated.Spec.Template.Spec.Affinity = expectedDeployment.Spec.Template.Spec.Affinity.DeepCopy()
		updated.Spec.Replicas = expectedDeployment.Spec.Replicas
		updated.Spec.Template.Spec.Containers = expectedDeployment.Spec.Template.Spec.Containers

		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(updated.DeepCopy())
		if err != nil {
			return false, nil, err
		}
		unstructuredObj := unstructured.Unstructured{obj}
		unstructuredObj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
		return true, &unstructuredObj, nil
	}
	return false, nil, fmt.Errorf("expected object of kind Deployment but received %s", gvk.String())

}

// buildOCMAgentArgs returns the full command argument list to run the OCM Agent
// in a deployment.
func buildOCMAgentArgs(ocmAgent ocmagentv1alpha1.OcmAgent) []string {
	accessTokenPath := filepath.Join(oahconst.OCMAgentSecretMountPath, ocmAgent.Spec.TokenSecret,
		oahconst.OCMAgentAccessTokenSecretKey)
	configServicesPath := filepath.Join(oahconst.OCMAgentConfigMountPath, ocmAgent.Spec.OcmAgentConfig,
		oahconst.OCMAgentConfigServicesKey)
	configURLPath := filepath.Join(oahconst.OCMAgentConfigMountPath, ocmAgent.Spec.OcmAgentConfig,
		oahconst.OCMAgentConfigURLKey)

	command := []string {
		oahconst.OCMAgentCommand,
		"serve",
		fmt.Sprintf("--access-token=@%s", accessTokenPath),
		fmt.Sprintf("--services=@%s", configServicesPath),
		fmt.Sprintf("--ocm-url=@%s", configURLPath),
	}
	return command
}
