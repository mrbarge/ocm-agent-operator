package ocmagenthandler

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ocmagentv1alpha1 "github.com/openshift/ocm-agent-operator/pkg/apis/ocmagent/v1alpha1"
	"github.com/openshift/ocm-agent-operator/pkg/resourcemanager"
)

//go:generate mockgen -source $GOFILE -destination ../../pkg/util/test/generated/mocks/$GOPACKAGE/interfaces.go -package $GOPACKAGE

type OCMAgentHandler interface {
	// EnsureOCMAgentResourcesExist ensures that an OCM Agent is deployed on the cluster.
	EnsureOCMAgentResourcesExist(ocmagentv1alpha1.OcmAgent) error
	// EnsureOCMAgentResourcesAbsent ensures that all OCM Agent resources are removed on the cluster.
	EnsureOCMAgentResourcesAbsent(ocmagentv1alpha1.OcmAgent) error
}

type ocmAgentHandler struct {
	Client client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
	Ctx    context.Context
}

func New(client client.Client, scheme *runtime.Scheme, log logr.Logger, ctx context.Context) OCMAgentHandler {
	return &ocmAgentHandler{
		Client: client,
		Scheme: scheme,
		Log:    log,
		Ctx:    ctx,
	}
}


func (o *ocmAgentHandler) EnsureOCMAgentResourcesExist(ocmAgent ocmagentv1alpha1.OcmAgent) error {
	// Ensure we have a ConfigMap
	cm := resourcemanager.NewOCMConfigResourceMgr()
	err := o.ensureResource(ocmAgent, cm)
	if err != nil {
		return err
	}

	// Ensure we have a service
	svc := resourcemanager.NewOCMServiceResourceMgr()
	err = o.ensureResource(ocmAgent, svc)
	if err != nil {
		return err
	}

	// Ensure we have a secret
	secret := resourcemanager.NewOCMSecretResourceMgr(o.Client, o.Ctx, o.Scheme)
	err = o.ensureResource(ocmAgent, secret)
	if err != nil {
		return err
	}

	// Ensure we have a deployment
	dep := resourcemanager.NewOCMDeploymentResourceMgr()
	err = o.ensureResource(ocmAgent, dep)
	if err != nil {
		return err
	}

	return nil
}

func (o *ocmAgentHandler) EnsureOCMAgentResourcesAbsent(ocmAgent ocmagentv1alpha1.OcmAgent) error {

	// Ensure the deployment is removed
	dep := resourcemanager.NewOCMDeploymentResourceMgr()
	err := o.ensureResourceRemoved(ocmAgent, dep)
	if err != nil {
		return err
	}

	// Ensure the service is removed
	svc := resourcemanager.NewOCMServiceResourceMgr()
	err = o.ensureResourceRemoved(ocmAgent, svc)
	if err != nil {
		return err
	}

	// Ensure the configmap is removed
	cm := resourcemanager.NewOCMConfigResourceMgr()
	err = o.ensureResourceRemoved(ocmAgent, cm)
	if err != nil {
		return err
	}

	// Ensure the access token secret is removed
	secret := resourcemanager.NewOCMSecretResourceMgr(o.Client, o.Ctx, o.Scheme)
	err = o.ensureResourceRemoved(ocmAgent, secret)
	if err != nil {
		return err
	}

	return nil
}

func (o *ocmAgentHandler) ensureResource(ocmAgent ocmagentv1alpha1.OcmAgent, r resourcemanager.OCMResourceManager) error {
	obj, err := r.Build(ocmAgent)
	if err != nil {
		return err
	}
	name := obj.GetName()
	if name == "" {
		return fmt.Errorf("Object %s has no name", obj.GroupVersionKind().String())
	}
	gvk := obj.GroupVersionKind()

	// Get existing
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		err := o.Client.Get(o.Ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing)
		if err != nil && k8serrors.IsNotFound(err) {
			// Set the controller reference
			if err := controllerutil.SetControllerReference(&ocmAgent, obj, o.Scheme); err != nil {
				o.Log.Error(err, "can't set controller reference")
				return err
			}
			o.Log.Info("creating object which does not exist",
				"object", obj.GroupVersionKind().String(),
				"name", obj.GetName(), "namespace", obj.GetNamespace())
			err := o.Client.Create(o.Ctx, obj)
			if err != nil {
				return err
			}
			return nil
		}
		if err != nil {
			return err
		}

		// Merge the desired object with what actually exists
		changed, updated, err := r.Changed(existing, obj)
		if err != nil {
			return err
		}
		if changed {
			o.Log.Info("updating object due to differences detected",
				"object", obj.GroupVersionKind().String(),
				"name", obj.GetName(), "namespace", obj.GetNamespace())
			if err := o.Client.Update(o.Ctx, updated); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func (o *ocmAgentHandler) ensureResourceRemoved(ocmAgent ocmagentv1alpha1.OcmAgent, r resourcemanager.OCMResourceManager) error {
	obj, err := r.Build(ocmAgent)
	if err != nil {
		return err
	}
	name := obj.GetName()
	if name == "" {
		return fmt.Errorf("Object %s has no name", obj.GroupVersionKind().String())
	}
	gvk := obj.GroupVersionKind()

	// Does the resource already exist?
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	if err := o.Client.Get(o.Ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing); err != nil {
		if !k8serrors.IsNotFound(err) {
			// Return unexpected error
			return err
		} else {
			// Resource deleted
			return nil
		}
	}
	err = o.Client.Delete(o.Ctx, existing)
	if err != nil {
		return err
	}

	return nil
}
