package updater

import (
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
)

// DockerConfigKeychain implements authn.Keychain
type DockerConfigKeychain struct {
	Auths map[string]authn.AuthConfig
}

// Resolve is called automatically by the library when it's about to make a network call
func (k *DockerConfigKeychain) Resolve(target authn.Resource) (authn.Authenticator, error) {
	registry := target.RegistryStr()

	// Check if there are credentials for this specific registry
	if authConfig, exists := k.Auths[registry]; exists {
		return authn.FromConfig(authConfig), nil
	}

	// Edge-case handling for Docker Hub's weird legacy URL format in config.json
	if registry == name.DefaultRegistry {
		if authConfig, exists := k.Auths["https://index.docker.io/v1/"]; exists {
			return authn.FromConfig(authConfig), nil
		}
	}

	// No credentials found for this registry, proceed anonymously
	return authn.Anonymous, nil
}
