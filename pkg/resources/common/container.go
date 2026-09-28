package common

import (
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
)

// GetImagePullPolicy determines the pull policy subject to
// the env var IMAGE_PULL_POLICY.
func GetImagePullPolicy() corev1.PullPolicy {
	pullPolicy := os.Getenv("IMAGE_PULL_POLICY")

	if pullPolicy == "" {
		fmt.Println("Defaulting to pull policy of being 'Always'")
		pullPolicy = "Always"
	}

	if pullPolicy != "Always" && pullPolicy != "Never" && pullPolicy != "IfNotPresent" {
		fmt.Printf("Invalid value %s for IMAGE_PULL_POLICY\n", pullPolicy)
		fmt.Println("Defaulting to pull policy of being 'Always'")
		pullPolicy = "Always"
	}

	return corev1.PullPolicy(pullPolicy)
}
