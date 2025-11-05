package resources

import (
	"fmt"
	"os"
	"path"

	"github.com/Masterminds/semver"
	"github.com/go-logr/logr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labelUtils "k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/pointer"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
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

func NewDefaultDeployment(hawtio *hawtiov2.Hawtio) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hawtio.Name,
			Namespace: hawtio.Namespace,
		},
	}
}

func NewDeployment(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec, openShiftConsoleURL string, configMapVersion string, clientCertSecretVersion string, buildVariables util.BuildVariables, log logr.Logger) (*appsv1.Deployment, error) {
	log.V(util.DebugLogLevel).Info("Reconciling deployment")

	podTemplateSpec, err := newPodTemplateSpec(hawtio, apiSpec, openShiftConsoleURL, configMapVersion, clientCertSecretVersion, buildVariables, log)
	if err != nil {
		return nil, err
	}
	return newDeployment(hawtio, hawtio.Spec.Replicas, podTemplateSpec, log), nil
}

func newDeployment(hawtio *hawtiov2.Hawtio, replicas *int32, pts corev1.PodTemplateSpec, log logr.Logger) *appsv1.Deployment {
	log.V(util.DebugLogLevel).Info("New deployment")

	// Deployment replicas field is defaulted to '1', so we have to equal that defaults, otherwise
	// the comparison algorithm assumes the requested resource is different, which leads to an infinite
	// reconciliation loop
	r := pointer.Int32PtrDerefOr(replicas, 1)

	deployment := NewDefaultDeployment(hawtio)

	annotations := map[string]string{}
	PropagateAnnotations(hawtio, annotations, log)

	labels := LabelsForHawtio(hawtio.Name)
	PropagateLabels(hawtio, labels, log)

	deployment.SetAnnotations(annotations)
	deployment.SetLabels(labels)
	deployment.Spec = appsv1.DeploymentSpec{
		Replicas: &r,
		Selector: &metav1.LabelSelector{
			MatchLabels: deployment.Labels,
		},
		Template: pts,
		Strategy: appsv1.DeploymentStrategy{
			Type: appsv1.RollingUpdateDeploymentStrategyType,
		},
	}
	return deployment
}

/**
 *
 * Creates a new pod template comprising 2 constainers:
 * - The hawtio container is the main Hawtio-Online application image
 * - The gteway container is the auxiliary image that provides useful javascript functions to
 *   the Hawtio-Online web server, inc. jolokia connection API and cluster URI checking
 *
 */
func newPodTemplateSpec(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec, openShiftConsoleURL string, configMapVersion string, clientCertSecretVersion string, buildVariables util.BuildVariables, log logr.Logger) (corev1.PodTemplateSpec, error) {
	log.V(util.DebugLogLevel).Info("New Pod Template Spec")

	hawtioVersion := getOnlineVersion(buildVariables)
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Using Hawtio Image Version: %s", hawtioVersion))

	hawtioContainer := newHawtioContainer(hawtio, apiSpec, openShiftConsoleURL, hawtioVersion, buildVariables.ImageRepository, log)

	gatewayVersion := getGatewayVersion(buildVariables)
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Using Hawtio Gateway Image Version: %s", gatewayVersion))

	gatewayContainer := newGatewayContainer(hawtio, apiSpec, gatewayVersion, buildVariables.GatewayImageRepository, log)

	annotations := map[string]string{
		configVersionAnnotation: configMapVersion,
	}
	if clientCertSecretVersion != "" {
		annotations[clientCertSecretVersionAnnotation] = clientCertSecretVersion
	}
	PropagateAnnotations(hawtio, annotations, log)

	volumeMounts, err := newVolumeMounts(hawtio, apiSpec, hawtioVersion, hawtio.Spec.RBAC.ConfigMap, buildVariables, log)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	if len(volumeMounts) > 0 {
		/* Distribute the volume mounts between the containers */
		volume, ok := volumeMounts[onlineConfigMapVolumeName]
		if ok {
			hawtioContainer.VolumeMounts = append(hawtioContainer.VolumeMounts, volume)
		}

		volume, ok = volumeMounts[serviceSigningSecretVolumeName]
		if ok {
			hawtioContainer.VolumeMounts = append(hawtioContainer.VolumeMounts, volume)
		}

		if apiSpec.IsOpenShift4 {
			volume, ok := volumeMounts[clientCertificateSecretVolumeName]
			if ok {
				hawtioContainer.VolumeMounts = append(hawtioContainer.VolumeMounts, volume)
			}
		}

		if hawtio.Spec.RBAC.ConfigMap != "" {
			volume, ok := volumeMounts[rbacConfigMapVolumeName]
			if ok {
				gatewayContainer.VolumeMounts = append(gatewayContainer.VolumeMounts, volume)
			}
		}

		volume, ok = volumeMounts[serviceSigningSecretVolumeName]
		if ok {
			gatewayContainer.VolumeMounts = append(gatewayContainer.VolumeMounts, volume)
		}
	}
	volumes := newVolumes(hawtio, apiSpec, log)

	labels := LabelsForHawtio(hawtio.Name)
	additionalLabels, err := labelUtils.ConvertSelectorToLabelsMap(buildVariables.AdditionalLabels)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	for name, value := range additionalLabels {
		labels[name] = value
	}
	PropagateLabels(hawtio, labels, log)

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

	log.V(util.DebugLogLevel).Info(fmt.Sprintf("PodTemplateSpec: %s", util.JSONToString(pod)))

	return pod, err
}

func newVolumes(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec, log logr.Logger) []corev1.Volume {
	log.V(util.DebugLogLevel).Info("Creating new volumes")

	var volumes []corev1.Volume

	if util.IsSSL(hawtio, apiSpec) {
		log.V(util.DebugLogLevel).Info("Adding secret volume for serving certificate %s-tls-serving at %s", hawtio.Name, serviceSigningSecretVolumeName)
		volume := newSecretVolume(hawtio.Name+"-tls-serving", serviceSigningSecretVolumeName)
		volumes = append(volumes, volume)
	}

	if apiSpec.IsOpenShift4 {
		log.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding secret volume for proxying certificate %s-tls-proxying at %s", hawtio.Name, clientCertificateSecretVolumeName))
		volume := newSecretVolume(hawtio.Name+"-tls-proxying", clientCertificateSecretVolumeName)
		volumes = append(volumes, volume)
	}

	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding config map volume %s at %s", hawtio.Name, onlineConfigMapVolumeName))
	volume := newConfigMapVolume(hawtio.Name, onlineConfigMapVolumeName)
	volumes = append(volumes, volume)

	if rbacConfigMapName := hawtio.Spec.RBAC.ConfigMap; rbacConfigMapName != "" {
		log.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding config map volume %s at %s", rbacConfigMapName, rbacConfigMapVolumeName))
		volume = newConfigMapVolume(rbacConfigMapName, rbacConfigMapVolumeName)
		volumes = append(volumes, volume)
	}

	return volumes
}

func newVolumeMounts(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec, hawtioVersion string, rbacConfigMapName string, buildVariables util.BuildVariables, log logr.Logger) (map[string]corev1.VolumeMount, error) {
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
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding volume mount %s at %s", onlineConfigMapVolumeName, volumeMountPath))

	volumeMount := newVolumeMount(onlineConfigMapVolumeName, volumeMountPath, hawtioConfigKey)
	volumeMounts[onlineConfigMapVolumeName] = volumeMount

	/*
	 * The serving-certificate volume
	 */
	if util.IsSSL(hawtio, apiSpec) {
		volumeMountPath, err := getServingCertificateMountPath(hawtioVersion, buildVariables.LegacyServingCertificateMountVersion)
		if err != nil {
			return nil, err
		}
		log.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding volume mount %s at %s", serviceSigningSecretVolumeName, volumeMountPath))
		volumeMount = newVolumeMount(serviceSigningSecretVolumeName, volumeMountPath, "")
		volumeMounts[serviceSigningSecretVolumeName] = volumeMount
	}

	if apiSpec.IsOpenShift4 {
		/*
		 * The proxying volume
		 */
		log.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding volume mount %s at %s", clientCertificateSecretVolumeName, clientCertificateSecretVolumeMountPath))
		volumeMount = newVolumeMount(clientCertificateSecretVolumeName, clientCertificateSecretVolumeMountPath, "")
		volumeMounts[clientCertificateSecretVolumeName] = volumeMount
	}

	/*
	 * The rbac volume
	 */
	if rbacConfigMapName != "" {
		log.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding volume mount %s at %s", rbacConfigMapVolumeName, rbacConfigMapVolumeMountPath))
		volumeMount = newVolumeMount(rbacConfigMapVolumeName, rbacConfigMapVolumeMountPath, "")
		volumeMounts[rbacConfigMapVolumeName] = volumeMount
	}

	log.V(util.DebugLogLevel).Info(fmt.Sprintf("New VolumeMounts %s", util.JSONToString(volumeMounts)))
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
