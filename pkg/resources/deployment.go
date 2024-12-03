package resources

import (
	"fmt"
	"os"
	"path"

	"github.com/Masterminds/semver"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labelUtils "k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/pointer"

	hawtiov1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	serviceSigningSecretVolumeName            = "hawtio-online-tls-serving"
	serviceSigningSecretVolumeMountPath       = "/etc/tls/private/serving"
	serviceSigningSecretVolumeMountPathLegacy = "/etc/tls/private"
	clientCertificateSecretVolumeName         = "hawtio-online-tls-proxying"
	clientCertificateSecretVolumeMountPath    = "/etc/tls/private/proxying"
	onlineConfigMapVolumeName                 = "hawtio-online"
	rbacConfigMapVolumeName                   = "hawtio-rbac"
	rbacConfigMapVolumeMountPath              = "/etc/hawtio/rbac"
	RBACConfigMapKey                          = "ACL.yaml"
	configVersionAnnotation                   = "hawtio.hawt.io/configversion"
	clientCertSecretVersionAnnotation         = "hawtio.hawt.io/certversion"
	serverRootDirectory                       = "/usr/share/nginx/html"
)

func NewDeployment(hawtio *hawtiov1.Hawtio, apiSpec *capabilities.ApiServerSpec, openShiftConsoleURL string, configMapVersion string, clientCertSecretVersion string, buildVariables util.BuildVariables) (*appsv1.Deployment, error) {
	podTemplateSpec, err := newPodTemplateSpec(hawtio, apiSpec, openShiftConsoleURL, configMapVersion, clientCertSecretVersion, buildVariables)
	if err != nil {
		return nil, err
	}
	return newDeployment(hawtio, hawtio.Spec.Replicas, podTemplateSpec), nil
}

func newDeployment(hawtio *hawtiov1.Hawtio, replicas *int32, pts corev1.PodTemplateSpec) *appsv1.Deployment {
	annotations := map[string]string{}
	PropagateAnnotations(hawtio, annotations)

	labels := LabelsForHawtio(hawtio.Name)
	PropagateLabels(hawtio, labels)

	// Deployment replicas field is defaulted to '1', so we have to equal that defaults, otherwise
	// the comparison algorithm assumes the requested resource is different, which leads to an infinite
	// reconciliation loop
	r := pointer.Int32PtrDerefOr(replicas, 1)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        hawtio.Name,
			Namespace:   hawtio.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &r,
			Selector: &metav1.LabelSelector{
				MatchLabels: LabelsForHawtio(hawtio.Name),
			},
			Template: pts,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
			},
		},
	}
}

/**
 *
 * Creates a new pod template comprising 2 constainers:
 * - The hawtio container is the main Hawtio-Online application image
 * - The gteway container is the auxiliary image that provides useful javascript functions to
 *   the Hawtio-Online web server, inc. jolokia connection API and cluster URI checking
 *
 */
func newPodTemplateSpec(hawtio *hawtiov1.Hawtio, apiSpec *capabilities.ApiServerSpec, openShiftConsoleURL string, configMapVersion string, clientCertSecretVersion string, buildVariables util.BuildVariables) (corev1.PodTemplateSpec, error) {
	hawtioVersion := getOnlineVersion(buildVariables)
	hawtioContainer := newHawtioContainer(hawtio, newHawtioEnvVars(hawtio, apiSpec, openShiftConsoleURL), hawtioVersion, buildVariables.ImageRepository)
	gatewayVersion := getGatewayVersion(buildVariables)
	gatewayContainer := newGatewayContainer(hawtio, newGatewayEnvVars(hawtio, apiSpec), gatewayVersion, buildVariables.GatewayImageRepository)

	annotations := map[string]string{
		configVersionAnnotation: configMapVersion,
	}
	if clientCertSecretVersion != "" {
		annotations[clientCertSecretVersionAnnotation] = clientCertSecretVersion
	}
	PropagateAnnotations(hawtio, annotations)

	volumeMounts, err := newVolumeMounts(apiSpec.IsOpenShift4, hawtioVersion, hawtio.Spec.RBAC.ConfigMap, buildVariables)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	if len(volumeMounts) > 0 {
		/* Distribute the volume mounts between the containers */
		hawtioContainer.VolumeMounts = append(hawtioContainer.VolumeMounts, volumeMounts[onlineConfigMapVolumeName])
		hawtioContainer.VolumeMounts = append(hawtioContainer.VolumeMounts, volumeMounts[serviceSigningSecretVolumeName])

		if apiSpec.IsOpenShift4 {
			hawtioContainer.VolumeMounts = append(hawtioContainer.VolumeMounts, volumeMounts[clientCertificateSecretVolumeName])
		}

		if hawtio.Spec.RBAC.ConfigMap != "" {
			gatewayContainer.VolumeMounts = append(gatewayContainer.VolumeMounts, volumeMounts[rbacConfigMapVolumeName])
		}

		gatewayContainer.VolumeMounts = append(gatewayContainer.VolumeMounts, volumeMounts[serviceSigningSecretVolumeName])
	}
	volumes := newVolumes(hawtio, apiSpec.IsOpenShift4)

	labels := LabelsForHawtio(hawtio.Name)
	additionalLabels, err := labelUtils.ConvertSelectorToLabelsMap(buildVariables.AdditionalLabels)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	for name, value := range additionalLabels {
		labels[name] = value
	}
	PropagateLabels(hawtio, labels)

	pod := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
			// Used to trigger a rollout deployment if config changed,
			// similarly to `kubectl rollout restart`
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				hawtioContainer,
				gatewayContainer,
			},
			Volumes: volumes,
		},
	}

	return pod, err
}

func newVolumes(hawtio *hawtiov1.Hawtio, isOpenShift4 bool) []corev1.Volume {
	var volumes []corev1.Volume

	volume := newSecretVolume(hawtio.Name+"-tls-serving", serviceSigningSecretVolumeName)
	volumes = append(volumes, volume)

	if isOpenShift4 {
		volume = newSecretVolume(hawtio.Name+"-tls-proxying", clientCertificateSecretVolumeName)
		volumes = append(volumes, volume)
	}

	volume = newConfigMapVolume(hawtio.Name, onlineConfigMapVolumeName)
	volumes = append(volumes, volume)

	if rbacConfigMapName := hawtio.Spec.RBAC.ConfigMap; rbacConfigMapName != "" {
		volume = newConfigMapVolume(rbacConfigMapName, rbacConfigMapVolumeName)
		volumes = append(volumes, volume)
	}

	return volumes
}

func newHawtioEnvVars(hawtio *hawtiov1.Hawtio, apiSpec *capabilities.ApiServerSpec, openShiftConsoleURL string) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	envVarsForHawtio := envVarsForHawtio(hawtio.Spec.Type, hawtio.Name, apiSpec.IsOpenShift4)
	envVars = append(envVars, envVarsForHawtio...)

	if apiSpec.IsOpenShift4 {
		envVarsForOpenShift4 := envVarsForHawtioOCP4(apiSpec.Version, openShiftConsoleURL)
		envVars = append(envVars, envVarsForOpenShift4...)
	}

	envVarsForNginx := envVarsForNginx(hawtio.Spec.Nginx)
	envVars = append(envVars, envVarsForNginx...)

	return envVars
}

func newGatewayEnvVars(hawtio *hawtiov1.Hawtio, apiSpec *capabilities.ApiServerSpec) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	envVarsForGateway := envVarsForGateway(apiSpec.IsOpenShift4)
	envVars = append(envVars, envVarsForGateway...)

	envVarsForRBAC := envVarsForRBAC(hawtio.Spec.RBAC)
	envVars = append(envVars, envVarsForRBAC...)

	return envVars
}

func newVolumeMounts(isOpenShift4 bool, hawtioVersion string, rbacConfigMapName string, buildVariables util.BuildVariables) (map[string]corev1.VolumeMount, error) {
	var volumeMounts map[string]corev1.VolumeMount
	var volumeMountPath string

	volumeMounts = make(map[string]corev1.VolumeMount)

	/*
	 * The hawtio-online config-map volume
	 */
	if buildVariables.ServerRootDirectory != "" {
		volumeMountPath = path.Join(buildVariables.ServerRootDirectory, "online", hawtioConfigKey)
	} else {
		volumeMountPath = path.Join(serverRootDirectory, "online", hawtioConfigKey)
	}
	volumeMount := newVolumeMount(onlineConfigMapVolumeName, volumeMountPath, hawtioConfigKey)
	volumeMounts[onlineConfigMapVolumeName] = volumeMount

	/*
	 * The serving-certificate volume
	 */
	volumeMountPath, err := getServingCertificateMountPath(hawtioVersion, buildVariables.LegacyServingCertificateMountVersion)
	if err != nil {
		return nil, err
	}
	volumeMount = newVolumeMount(serviceSigningSecretVolumeName, volumeMountPath, "")
	volumeMounts[serviceSigningSecretVolumeName] = volumeMount

	if isOpenShift4 {
		/*
		 * The proxying volume
		 */
		volumeMount = newVolumeMount(clientCertificateSecretVolumeName, clientCertificateSecretVolumeMountPath, "")
		volumeMounts[clientCertificateSecretVolumeName] = volumeMount
	}

	/*
	 * The rbac volume
	 */
	if rbacConfigMapName != "" {
		volumeMount = newVolumeMount(rbacConfigMapVolumeName, rbacConfigMapVolumeMountPath, "")
		volumeMounts[rbacConfigMapVolumeName] = volumeMount
	}

	return volumeMounts, nil
}

func getServingCertificateMountPath(version string, legacyServingCertificateMountVersion string) (string, error) {
	if len(version) == 0 {
		version = "latest"
	}
	if version != "latest" {
		semVer, err := semver.NewVersion(version)
		if err != nil {
			// not a standard version format (possibly an arbitrary tag)
			// which is fine and just skip version check
			return serviceSigningSecretVolumeMountPath, nil
		}
		var constraints *semver.Constraints
		if legacyServingCertificateMountVersion == "" {
			constraints, err = semver.NewConstraint("< 1.7.0")
			if err != nil {
				return "", err
			}
		} else {
			constraints, err = semver.NewConstraint(legacyServingCertificateMountVersion)
			if err != nil {
				return "", err
			}
		}
		if constraints.Check(semVer) {
			return serviceSigningSecretVolumeMountPathLegacy, nil
		}
	}
	return serviceSigningSecretVolumeMountPath, nil
}

func getOnlineVersion(buildVariables util.BuildVariables) string {
	fmt.Println("Getting version from IMAGE_VERSION environment variable ...")
	version := os.Getenv("IMAGE_VERSION")
	if version == "" {
		fmt.Println("Getting version from build variable ImageVersion")
		version = buildVariables.ImageVersion
		if len(version) == 0 {
			fmt.Println("Defaulting to version being latest")
			version = "latest"
		}
	}
	return version
}

func getGatewayVersion(buildVariables util.BuildVariables) string {
	fmt.Println("Getting version from GATEWAY_IMAGE_VERSION environment variable ...")
	version := os.Getenv("GATEWAY_IMAGE_VERSION")
	if version == "" {
		fmt.Println("Getting version from build variable GatewayImageVersion")
		version = buildVariables.GatewayImageVersion
		if len(version) == 0 {
			fmt.Println("Defaulting to online version")
			version = getOnlineVersion(buildVariables)
		}
	}
	return version
}
