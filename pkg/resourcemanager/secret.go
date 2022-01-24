package resourcemanager

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ocmagentv1alpha1 "github.com/openshift/ocm-agent-operator/pkg/apis/ocmagent/v1alpha1"
	oahconst "github.com/openshift/ocm-agent-operator/pkg/consts/ocmagenthandler"
)

type OCMSecretResourceMgr interface {
	OCMResourceManager
}

type ocmSecretResourceMgr struct {
	Client client.Client
	Scheme *runtime.Scheme
	Ctx    context.Context
}

func NewOCMSecretResourceMgr(client client.Client, ctx context.Context, scheme *runtime.Scheme) OCMResourceManager {
	return &ocmSecretResourceMgr{
		Client: client,
		Ctx:    ctx,
		Scheme: scheme,
	}
}

func (r *ocmSecretResourceMgr) Build(ocmAgent ocmagentv1alpha1.OcmAgent) (*unstructured.Unstructured, error) {
	accessToken, err := r.fetchAccessTokenPullSecret()

	namespacedName := oahconst.BuildNamespacedName()
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ocmAgent.Spec.TokenSecret,
			Namespace: namespacedName.Namespace,
		},
		Data: map[string][]byte{
			oahconst.OCMAgentAccessTokenSecretKey: accessToken,
		},
	}
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(secret.DeepCopy())
	if err != nil {
		return nil, err
	}
	unstructuredObj := unstructured.Unstructured{obj}
	unstructuredObj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"})
	return &unstructuredObj, nil
}

func (r *ocmSecretResourceMgr) Changed(current *unstructured.Unstructured, expected *unstructured.Unstructured) (bool, *unstructured.Unstructured, error) {
	gvk := expected.GroupVersionKind()
	if gvk.Group == "" && gvk.Kind == "Secret" {

		changed := false
		currentSecret := &corev1.Secret{}
		expectedSecret := &corev1.Secret{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(current.UnstructuredContent(), currentSecret); err != nil {
			return false, nil, err
		}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(expected.UnstructuredContent(), expectedSecret); err != nil {
			return false, nil, err
		}

		if !reflect.DeepEqual(currentSecret.Data, expectedSecret.Data) {
			changed = true
		}

		if !changed {
			return false, nil, nil
		}

		updated := currentSecret.DeepCopy()
		updated.Data = expectedSecret.Data
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(updated.DeepCopy())
		if err != nil {
			return false, nil, err
		}
		unstructuredObj := unstructured.Unstructured{obj}
		unstructuredObj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"})
		return true, &unstructuredObj, nil
	}
	return false, nil, fmt.Errorf("expected object of kind Secret but received %s", gvk.String())
}

func (r *ocmSecretResourceMgr) fetchAccessTokenPullSecret() ([]byte, error) {
	foundResource := &corev1.Secret{}
	if err := r.Client.Get(r.Ctx, oahconst.BuildPullSecretNamespacedName(), foundResource); err != nil {
		// There should always be a pull secret, log this
		return nil, err
	}

	pullSecret, ok := foundResource.Data[oahconst.PullSecretKey]
	if !ok {
		return nil, fmt.Errorf("pull secret missing required key '%s'", oahconst.PullSecretKey)
	}

	var dockerConfig map[string]interface{}
	err := json.Unmarshal(pullSecret, &dockerConfig)
	if err != nil {
		return nil, err
	}

	authConfig, ok := dockerConfig["auths"]
	if !ok {
		return nil, fmt.Errorf("unable to find auths section in pull secret")
	}
	apiConfig, ok := authConfig.(map[string]interface{})[oahconst.PullSecretAuthTokenKey]
	if !ok {
		return nil, fmt.Errorf("unable to find pull secret auth key '%s' in pull secret", oahconst.PullSecretAuthTokenKey)
	}
	accessToken, ok := apiConfig.(map[string]interface{})["auth"]
	if !ok {
		return nil, fmt.Errorf("unable to find access auth token in pull secret")
	}
	strAccessToken := fmt.Sprintf("%v", accessToken)

	return []byte(strAccessToken), nil
}
