package resources

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConfigForHawtio(t *testing.T) {
	// Create a temporary "default" config file for the test
	// Ensures test doesn't fail if the real file isn't present
	tempHawtioConfigFile, err := os.CreateTemp("", "hawtconfig-default-*.json")
	require.NoError(t, err)
	defer os.Remove(tempHawtioConfigFile.Name()) // Clean up after the test

	// Write a simple default config to the temp file
	defaultConfigJSON := `{"about": {"title": "Hawtio Console"}, "branding": {"appName": "Hawtio"}}`
	_, err = tempHawtioConfigFile.Write([]byte(defaultConfigJSON))
	require.NoError(t, err)
	tempHawtioConfigFile.Close()

	// TEST CASES
	tests := []struct {
		name         string
		hawtio       *hawtiov2.Hawtio
		expectedJSON string
		expectErr    bool
	}{
		{
			name: "Empty Config - Should apply defaults only",
			hawtio: &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-hawtio"},
				Spec: hawtiov2.HawtioSpec{
					Config: hawtiov2.HawtioConfig{}, // Empty
				},
			},
			// Expects just the default values we wrote to the temp file
			expectedJSON: `{"about": {"title": "Hawtio Console"}, "branding": {"appName": "Hawtio"}, "online": {"consoleLink": {}}}`,
			expectErr:    false,
		},
		{
			name: "New Config - Should retain Description",
			hawtio: &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{Name: "modern-hawtio"},
				Spec: hawtiov2.HawtioSpec{
					Config: hawtiov2.HawtioConfig{
						About: hawtiov2.HawtioAbout{
							Description: "This is a modern description",
						},
					},
				},
			},
			// Expects the custom description merged with the default title
			expectedJSON: `{"about": {"title": "Hawtio Console", "description": "This is a modern description"}, "branding": {"appName": "Hawtio"}, "online": {"consoleLink": {}}}`,
			expectErr:    false,
		},
		{
			name: "Legacy Config - Should bridge AdditionalInfo to Description",
			hawtio: &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{Name: "legacy-hawtio"},
				Spec: hawtiov2.HawtioSpec{
					Config: hawtiov2.HawtioConfig{
						About: hawtiov2.HawtioAbout{
							AdditionalInfo: "This is legacy additional info",
						},
					},
				},
			},
			// Expects additionalInfo to be stripped, and its value moved to description
			expectedJSON: `{"about": {"title": "Hawtio Console", "description": "This is legacy additional info"}, "branding": {"appName": "Hawtio"}, "online": {"consoleLink": {}}}`,
			expectErr:    false,
		},
		{
			name: "Mixed Config - Description should take precedence over AdditionalInfo",
			hawtio: &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{Name: "mixed-hawtio"},
				Spec: hawtiov2.HawtioSpec{
					Config: hawtiov2.HawtioConfig{
						About: hawtiov2.HawtioAbout{
							Description:    "New description takes priority",
							AdditionalInfo: "Old info to be ignored",
						},
					},
				},
			},
			// Expects Description to win, AdditionalInfo to vanish
			expectedJSON: `{"about": {"title": "Hawtio Console", "description": "New description takes priority"}, "branding": {"appName": "Hawtio"}, "online": {"consoleLink": {}}}`,
			expectErr:    false,
		},
		{
			name: "About Config - Dark mode logo should fallback to standard logo",
			hawtio: &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{Name: "about-hawtio"},
				Spec: hawtiov2.HawtioSpec{
					Config: hawtiov2.HawtioConfig{
						About: hawtiov2.HawtioAbout{
							Title:  "My Corp Console",
							ImgSrc: "https://mycorp.com/logo.png",
							// ImgDarkModeSrc is intentionally left blank
						},
					},
				},
			},
			// Expects the dark mode logo to be automatically populated with the standard logo URL
			expectedJSON: `{"about": {"title": "Hawtio Console"}, "about": {"title": "My Corp Console", "imgSrc": "https://mycorp.com/logo.png", "imgDarkModeSrc": "https://mycorp.com/logo.png"}, "branding": {"appName":"Hawtio"}, "online": {"consoleLink": {}}}`,
			expectErr:    false,
		},
		{
			name: "Branding Config - Dark mode logo should fallback to standard logo",
			hawtio: &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{Name: "branding-hawtio"},
				Spec: hawtiov2.HawtioSpec{
					Config: hawtiov2.HawtioConfig{
						Branding: hawtiov2.HawtioBranding{
							AppName:    "My Corp Console",
							AppLogoURL: "https://mycorp.com/logo.png",
							// AppLogoDarkModeUrl is intentionally left blank
						},
					},
				},
			},
			// Expects the dark mode logo to be automatically populated with the standard logo URL
			expectedJSON: `{"about": {"title": "Hawtio Console"}, "branding": {"appName": "My Corp Console", "appLogoUrl": "https://mycorp.com/logo.png", "appLogoDarkModeUrl": "https://mycorp.com/logo.png"}, "online": {"consoleLink": {}}}`,
			expectErr:    false,
		},
	}

	// RUN TESTS
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute the function
			result, err := configForHawtio(tt.hawtio, tempHawtioConfigFile.Name())

			// Check error expectations
			if tt.expectErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Assert JSON equivalence
			// JSONEq is critical here because map merging doesn't guarantee key order
			assert.JSONEq(t, tt.expectedJSON, result, "The generated JSON config did not match expectations")
		})
	}
}
