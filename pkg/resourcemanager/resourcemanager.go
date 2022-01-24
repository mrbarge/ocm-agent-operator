package resourcemanager

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	ocmagentv1alpha1 "github.com/openshift/ocm-agent-operator/pkg/apis/ocmagent/v1alpha1"
)

//go:generate mockgen -source $GOFILE -destination ../../pkg/util/test/generated/mocks/$GOPACKAGE/interfaces.go -package $GOPACKAGE
type OCMResourceManager interface {
	// Build builds a desired object to be created.
	Build(ocmagentv1alpha1.OcmAgent) (*unstructured.Unstructured, error)
	// MergeForUpdate prepares a desired object to be updated. Some objects, such
	// as Deployments and Services require some semantic-aware updates.
	Changed(current *unstructured.Unstructured, expected *unstructured.Unstructured) (bool, *unstructured.Unstructured, error)
}
