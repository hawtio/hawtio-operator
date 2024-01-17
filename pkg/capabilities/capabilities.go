package capabilities

import (
	"context"
	"errors"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "k8s.io/client-go/kubernetes"

	"github.com/Masterminds/semver"
	errs "github.com/pkg/errors"
)

type ApiServerSpec struct {
	Version           string // Set to the version of the cluster
	KubeVersion       string // Set to the kubernetes version of the cluster (different to version if using OpenShift, for example)
	IsOpenShift4      bool   // Set to true if running on openshift 4
	IsOpenShift43Plus bool   // Set to true if running openshift 4.3+
	ImageStreams      bool   // Set to true if the API Server supports imagestreams
	Routes            bool   // Set to true if the API Server supports routes
	ConsoleLink       bool   // Set to true if the API Server support the openshift console link API
}

type RequiredApiSpec struct {
	routes       string
	imagestreams string
	consolelinks string
}

var RequiredApi = RequiredApiSpec{
	routes:       "routes.route.openshift.io/v1",
	imagestreams: "imagestreams.image.openshift.io/v1",
	consolelinks: "consolelinks.console.openshift.io/v1",
}

func contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}

func createResourceIndex(apiClient kclient.Interface) ([]string, error) {
	_, apiResourceLists, err := apiClient.Discovery().ServerGroupsAndResources()
	if err != nil {
		return nil, err
	}

	resIndex := []string{}

	for _, apiResList := range apiResourceLists {
		for _, apiResource := range apiResList.APIResources {
			resIndex = append(resIndex, fmt.Sprintf("%s.%s", apiResource.Name, apiResList.GroupVersion))
		}
	}

	return resIndex, nil
}

// APICapabilities For testing the given platform's capabilities
func APICapabilities(ctx context.Context, apiClient kclient.Interface, configClient configclient.Interface) (*ApiServerSpec, error) {
	if apiClient == nil {
		return nil, errors.New("No api client. Cannot determine api capabilities")
	}

	if configClient == nil {
		return nil, errors.New("No config client. Cannot determine api capabilities")
	}

	apiSpec := ApiServerSpec{}

	info, err := apiClient.Discovery().ServerVersion()
	if err != nil {
		return nil, errs.Wrap(err, "Failed to discover server version")
	}

	apiSpec.KubeVersion = info.Major + "." + info.Minor

	resIndex, err := createResourceIndex(apiClient)
	if err != nil {
		return nil, errs.Wrap(err, "Failed to create API Resource index")
	}

	apiSpec.Routes = contains(resIndex, RequiredApi.routes)
	apiSpec.ImageStreams = contains(resIndex, RequiredApi.imagestreams)
	apiSpec.ConsoleLink = contains(resIndex, RequiredApi.consolelinks)

	apiSpec.IsOpenShift4 = false

	var clusterVersion *configv1.ClusterVersion
	if apiSpec.ConsoleLink {
		//
		// Update the kubernetes version to the Openshift Version
		//
		clusterVersion, err = configClient.ConfigV1().ClusterVersions().Get(ctx, "version", metav1.GetOptions{})
		if err == nil {
			apiSpec.IsOpenShift4 = true
		} else if !kerrors.IsNotFound(err) {
			// Some other error rather than not found
			// If error is not found then treat as not OpenShift
			return nil, errs.Wrap(err, "Error reading cluster version")
		}
	}

	if apiSpec.IsOpenShift4 && clusterVersion != nil {
		// Let's take the latest version from the history
		for _, update := range clusterVersion.Status.History {
			if update.State == configv1.CompletedUpdate {
				// Obtain the version from the last completed update
				// Update the api spec version
				var openShiftSemVer *semver.Version
				openShiftSemVer, err = semver.NewVersion(update.Version)
				if err != nil {
					return nil, errs.Wrap(err, fmt.Sprintf("Error parsing OpenShift cluster semantic version %s", update.Version))
				}

				apiSpec.Version = openShiftSemVer.String()

				// Update whether this is OpenShift 4.3+
				constraint43, err := semver.NewConstraint(">= 4.3")
				if err != nil {
					return nil, errs.Wrap(err, fmt.Sprintf("Error parsing OpenShift cluster semantic version %s", update.Version))
				}

				apiSpec.IsOpenShift43Plus = constraint43.Check(openShiftSemVer)
				break
			}
		}
	} else {
		// This is not OpenShift so plain kubernetes or something else
		apiSpec.IsOpenShift4 = false

		// Update version to kubernetes version
		apiSpec.Version = apiSpec.KubeVersion
		apiSpec.IsOpenShift43Plus = false
	}

	return &apiSpec, nil
}
