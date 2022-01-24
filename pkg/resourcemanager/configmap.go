package resourcemanager

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ocmagentv1alpha1 "github.com/openshift/ocm-agent-operator/pkg/apis/ocmagent/v1alpha1"
	oahconst "github.com/openshift/ocm-agent-operator/pkg/consts/ocmagenthandler"
)

type OCMConfigResourceMgr interface {
	OCMResourceManager
}

type ocmConfigResourceMgr struct{}

func NewOCMConfigResourceMgr() OCMResourceManager {
	return &ocmConfigResourceMgr{}
}

func (r *ocmConfigResourceMgr) Build(ocmAgent ocmagentv1alpha1.OcmAgent) (*unstructured.Unstructured, error) {
	namespacedName := oahconst.BuildNamespacedName()
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ocmAgent.Spec.OcmAgentConfig,
			Namespace: namespacedName.Namespace,
		},
		Data: map[string]string{
			oahconst.OCMAgentConfigServicesKey: strings.Join(ocmAgent.Spec.Services, ","),
			oahconst.OCMAgentConfigURLKey:      ocmAgent.Spec.OcmBaseUrl,
		},
	}
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cm.DeepCopy())
	if err != nil {
		return nil, err
	}
	unstructuredObj := unstructured.Unstructured{obj}
	unstructuredObj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
	return &unstructuredObj, nil
}

func (r *ocmConfigResourceMgr) Changed(current *unstructured.Unstructured, expected *unstructured.Unstructured) (bool, *unstructured.Unstructured, error) {
	gvk := expected.GroupVersionKind()
	if gvk.Group == "" && gvk.Kind == "ConfigMap" {
		// Nothing specific to ignore for ConfigMaps
		updated := current.DeepCopy()
		return false, updated, nil
	}
	return false, nil, fmt.Errorf("expected object of kind ConfigMap but received %s", gvk.String())
}
