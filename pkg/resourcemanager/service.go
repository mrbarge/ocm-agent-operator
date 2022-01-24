package resourcemanager

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"

	ocmagentv1alpha1 "github.com/openshift/ocm-agent-operator/pkg/apis/ocmagent/v1alpha1"
	oahconst "github.com/openshift/ocm-agent-operator/pkg/consts/ocmagenthandler"
)

type OCMServiceResourceMgr interface {
	OCMResourceManager
}

type ocmServiceResourceMgr struct{}

func NewOCMServiceResourceMgr() OCMResourceManager {
	return &ocmServiceResourceMgr{}
}

func (r *ocmServiceResourceMgr) Build(ocmAgent ocmagentv1alpha1.OcmAgent) (*unstructured.Unstructured, error) {
	namespacedName := oahconst.BuildNamespacedName()
	labels := map[string]string{
		"app": oahconst.OCMAgentName,
	}
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      oahconst.OCMAgentServiceName,
			Namespace: namespacedName.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.FromInt(oahconst.OCMAgentServicePort),
				Name:       oahconst.OCMAgentPortName,
				Port:       oahconst.OCMAgentServicePort,
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(svc.DeepCopy())
	if err != nil {
		return nil, err
	}
	unstructuredObj := unstructured.Unstructured{obj}
	unstructuredObj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"})
	return &unstructuredObj, nil
}

func (r *ocmServiceResourceMgr) Changed(current *unstructured.Unstructured, expected *unstructured.Unstructured) (bool, *unstructured.Unstructured, error) {
	gvk := expected.GroupVersionKind()
	if gvk.Group == "" && gvk.Kind == "Service" {

		changed := false
		currentService := &corev1.Service{}
		expectedService := &corev1.Service{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(current.UnstructuredContent(), currentService); err != nil {
			return false, nil, err
		}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(expected.UnstructuredContent(), expectedService); err != nil {
			return false, nil, err
		}

		if !reflect.DeepEqual(currentService.Spec.Selector, expectedService.Spec.Selector) {
			changed = true
		}
		if !reflect.DeepEqual(currentService.Spec.Ports, expectedService.Spec.Ports) {
			changed = true
		}

		if !changed {
			return false, nil, nil
		}

		updated := currentService.DeepCopy()
		updated.Spec = expectedService.Spec
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(updated.DeepCopy())
		if err != nil {
			return false, nil, err
		}
		unstructuredObj := unstructured.Unstructured{obj}
		unstructuredObj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"})
		return true, &unstructuredObj, nil
	}
	return false, nil, fmt.Errorf("expected object of kind Service but received %s", gvk.String())
}
