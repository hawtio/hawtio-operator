package resources

import (
	"path"

	"github.com/Masterminds/semver"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labelUtils "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	serviceSigningSecretVolumeName            = "hawtio-online-tls-serving"
	serviceSigningSecretVolumeMountPath       = "/etc/tls/private/serving"
	serviceSigningSecretVolumeMountPathLegacy = "/etc/tls/private"
	clientCertificateSecretVolumeName         = "hawtio-online-tls-proxying"
	clientCertificateSecretVolumeMountPath    = "/etc/tls/private/proxying"
	onlineConfigMapVolumeName                 = "hawtio-online"
	integrationConfigMapVolumeName            = "hawtio-integration"
	rbacConfigMapVolumeName                   = "hawtio-rbac"
	rbacConfigMapVolumeMountPath              = "/etc/hawtio/rbac"
	RBACConfigMapKey                          = "ACL.yaml"
	configVersionAnnotation                   = "hawtio.hawt.io/configversion"
	serverRootDirectory                       = "/usr/share/nginx/html"
)

func NewDeployment(hawtio *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openShiftVersion string, openShiftConsoleURL string, hawtioVersion string, configMapVersion string, buildVariables util.BuildVariables) (*appsv1.Deployment, error) {
	namespacedName := types.NamespacedName{
		Name:      hawtio.Name,
		Namespace: hawtio.Namespace,
	}

	podTemplateSpec, err := newPodTemplateSpec(hawtio, isOpenShift4, openShiftVersion, openShiftConsoleURL, hawtioVersion, configMapVersion, buildVariables)
	if err != nil {
		return nil, err
	}
	return newDeployment(namespacedName, hawtio.Spec.Replicas, podTemplateSpec), nil
}

func newDeployment(namespacedName types.NamespacedName, replicas *int32, pts corev1.PodTemplateSpec) *appsv1.Deployment {
	labels := labelsForHawtio(namespacedName.Name)
	var r int32
	if replicas != nil {
		r = *replicas
	} else {
		// Deployment replicas field is defaulted to '1', so we have to equal that defaults, otherwise
		// the comparison algorithm assumes the requested resource is different, which leads to an infinite
		// reconciliation loop
		r = 1
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &r,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: pts,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
			},
		},
	}
}

func newPodTemplateSpec(hawtio *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openShiftVersion string, openShiftConsoleURL string, hawtioVersion string, configMapVersion string, buildVariables util.BuildVariables) (corev1.PodTemplateSpec, error) {
	container := newContainer(hawtio, newEnvVars(hawtio, isOpenShift4, openShiftVersion, openShiftConsoleURL), buildVariables.ImageRepository)

	volumeMounts, err := newVolumeMounts(isOpenShift4, hawtioVersion, hawtio.Spec.RBAC.ConfigMap, buildVariables)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	if len(volumeMounts) > 0 {
		container.VolumeMounts = volumeMounts
	}
	volumes := newVolumes(hawtio, isOpenShift4)

	labels := labelsForHawtio(hawtio.Name)
	additionalLabels, err := labelUtils.ConvertSelectorToLabelsMap(buildVariables.AdditionalLabels)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	for name, value := range additionalLabels {
		labels[name] = value
	}

	pod := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
			// Used to trigger a rollout deployment if config changed,
			// similarly to `kubectl rollout restart`
			Annotations: map[string]string{
				configVersionAnnotation: configMapVersion,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				container,
			},
			Volumes: volumes,
		},
	}

	return pod, err
}

func newVolumes(hawtio *hawtiov1alpha1.Hawtio, isOpenShift4 bool) []corev1.Volume {
	var volumes []corev1.Volume

	volume := newSecretVolume(hawtio.Name+"-tls-serving", serviceSigningSecretVolumeName)
	volumes = append(volumes, volume)

	if isOpenShift4 {
		volume = newSecretVolume(hawtio.Name+"-tls-proxying", clientCertificateSecretVolumeName)
		volumes = append(volumes, volume)
	}

	volume = newConfigMapVolume(hawtio.Name, onlineConfigMapVolumeName)
	volumes = append(volumes, volume)

	volume = newConfigMapVolume(hawtio.Name, integrationConfigMapVolumeName)
	volumes = append(volumes, volume)

	if rbacConfigMapName := hawtio.Spec.RBAC.ConfigMap; rbacConfigMapName != "" {
		volume = newConfigMapVolume(rbacConfigMapName, rbacConfigMapVolumeName)
		volumes = append(volumes, volume)
	}

	return volumes
}

func newEnvVars(hawtio *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openShiftVersion string, openShiftConsoleURL string) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	envVarsForHawtio := envVarsForHawtio(hawtio.Spec.Type, hawtio.Name)
	envVars = append(envVars, envVarsForHawtio...)

	if isOpenShift4 {
		envVarsForOpenShift4 := envVarsForOpenshift4(openShiftVersion, openShiftConsoleURL)
		envVars = append(envVars, envVarsForOpenShift4...)
	}

	envVars = append(envVars, envVarForRBAC(hawtio.Spec.RBAC.ConfigMap))

	return envVars
}

func newVolumeMounts(isOpenShift4 bool, hawtioVersion string, rbacConfigMapName string, buildVariables util.BuildVariables) ([]corev1.VolumeMount, error) {
	var volumeMounts []corev1.VolumeMount
	var volumeMountPath string

	if buildVariables.ServerRootDirectory != "" {
		volumeMountPath = path.Join(buildVariables.ServerRootDirectory, "online", hawtioConfigKey)
	} else {
		volumeMountPath = path.Join(serverRootDirectory, "online", hawtioConfigKey)
	}
	volumeMount := newVolumeMount(onlineConfigMapVolumeName, volumeMountPath, hawtioConfigKey)
	volumeMounts = append(volumeMounts, volumeMount)

	if buildVariables.ServerRootDirectory != "" {
		volumeMountPath = path.Join(buildVariables.ServerRootDirectory, "integration", hawtioConfigKey)
	} else {
		volumeMountPath = path.Join(serverRootDirectory, "integration", hawtioConfigKey)
	}
	volumeMount = newVolumeMount(integrationConfigMapVolumeName, volumeMountPath, hawtioConfigKey)
	volumeMounts = append(volumeMounts, volumeMount)

	volumeMountPath, err := getServingCertificateMountPath(hawtioVersion, buildVariables.LegacyServingCertificateMountVersion)
	if err != nil {
		return nil, err
	}
	volumeMount = newVolumeMount(serviceSigningSecretVolumeName, volumeMountPath, "")
	volumeMounts = append(volumeMounts, volumeMount)

	if isOpenShift4 {
		volumeMount = newVolumeMount(clientCertificateSecretVolumeName, clientCertificateSecretVolumeMountPath, "")
		volumeMounts = append(volumeMounts, volumeMount)
	}

	if rbacConfigMapName != "" {
		volumeMount = newVolumeMount(rbacConfigMapVolumeName, rbacConfigMapVolumeMountPath, "")
		volumeMounts = append(volumeMounts, volumeMount)
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
