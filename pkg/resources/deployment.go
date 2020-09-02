package resources

import (
	"path"

	"github.com/Masterminds/semver"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	configVersionAnnotation                   = "hawtio.hawt.io/configversion"
	serverRootDirectory                       = "/usr/share/nginx/html"
)

// Create NewDeploymentForCR method to create deployment
func NewDeploymentForCR(hawtio *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openShiftVersion string, openShiftConsoleURL string, hawtioVersion string, configMapVersion string, buildVariables util.BuildVariables) (*appsv1.Deployment, error) {
	namespacedName := types.NamespacedName{
		Name:      hawtio.Name,
		Namespace: hawtio.Namespace,
	}

	podTemplateSpec, err := newPodTemplateSpecForCR(hawtio, isOpenShift4, openShiftVersion, openShiftConsoleURL, hawtioVersion, configMapVersion, buildVariables)
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
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
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
				Type: "RollingUpdate",
			},
		},
	}
}

func newPodTemplateSpecForCR(hawtio *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openShiftVersion string, openShiftConsoleURL string, hawtioVersion string, configMapVersion string, buildVariables util.BuildVariables) (corev1.PodTemplateSpec, error) {
	namespacedName := types.NamespacedName{
		Name:      hawtio.Name,
		Namespace: hawtio.Namespace,
	}

	pts := newPodTemplateSpec(namespacedName, labelsForHawtio(hawtio.Name), configMapVersion)

	spec := corev1.PodSpec{}
	var Containers []corev1.Container
	container := NewContainer(hawtio, newEnvVarArrayForCR(hawtio, isOpenShift4, openShiftVersion, openShiftConsoleURL), buildVariables.ImageRepository)

	volumeMounts, err := newVolumeMounts(isOpenShift4, hawtioVersion, buildVariables)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	if len(volumeMounts) > 0 {
		container.VolumeMounts = volumeMounts
	}
	v := newVolumes(hawtio, isOpenShift4)
	if len(v) > 0 {
		spec.Volumes = v
	}
	spec.Containers = append(Containers, container)
	pts.Spec = spec

	return pts, err
}

func newVolumes(hawtio *hawtiov1alpha1.Hawtio, isOpenShift4 bool) []corev1.Volume {
	var volumes []corev1.Volume

	secretName := hawtio.Name + "-tls-serving"
	volumeName := serviceSigningSecretVolumeName
	volume := newVolume(secretName, volumeName)
	volumes = append(volumes, volume)

	if isOpenShift4 {
		secretName = hawtio.Name + "-tls-proxying"
		volumeName = clientCertificateSecretVolumeName
		volume = newVolume(secretName, volumeName)
		volumes = append(volumes, volume)
	}

	configMapName := hawtio.Name
	volumeName = "hawtio-online"
	volume = newConfigMapVolume(configMapName, volumeName)
	volumes = append(volumes, volume)

	configMapName = hawtio.Name
	volumeName = "hawtio-integration"
	volume = newConfigMapVolume(configMapName, volumeName)
	volumes = append(volumes, volume)

	return volumes
}

func newEnvVarArrayForCR(hawtio *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openShiftVersion string, openShiftConsoleURL string) []corev1.EnvVar {
	var envVar []corev1.EnvVar

	envVarArrayForCluster := addEnvVarForContainer(hawtio.Spec.Type, hawtio.Name)
	envVar = append(envVar, envVarArrayForCluster...)

	if isOpenShift4 {
		envVarArrayFoOpenShift := addEnvVarForOpenshift(openShiftVersion, openShiftConsoleURL)
		envVar = append(envVar, envVarArrayFoOpenShift...)
	}

	return envVar
}

func newVolumeMounts(isOpenShift4 bool, hawtioVersion string, buildVariables util.BuildVariables) ([]corev1.VolumeMount, error) {
	var volumeMounts []corev1.VolumeMount
	var volumeMountPath string

	volumeMountSubPath := hawtioConfigKey
	volumeMountName := "hawtio-online"
	if buildVariables.ServerRootDirectory != "" {
		volumeMountPath = path.Join(buildVariables.ServerRootDirectory, "online", hawtioConfigKey)
	} else {
		volumeMountPath = path.Join(serverRootDirectory, "online", hawtioConfigKey)
	}
	volumeMount := newVolumeMount(volumeMountName, volumeMountPath, volumeMountSubPath)
	volumeMounts = append(volumeMounts, volumeMount)

	volumeMountSubPath = hawtioConfigKey
	volumeMountName = "hawtio-integration"
	if buildVariables.ServerRootDirectory != "" {
		volumeMountPath = path.Join(buildVariables.ServerRootDirectory, "integration", hawtioConfigKey)
	} else {
		volumeMountPath = path.Join(serverRootDirectory, "integration", hawtioConfigKey)
	}
	volumeMount = newVolumeMount(volumeMountName, volumeMountPath, volumeMountSubPath)
	volumeMounts = append(volumeMounts, volumeMount)

	volumeMountSubPath = ""
	volumeMountName = serviceSigningSecretVolumeName
	volumeMountPath, err := getServingCertificateMountPathFor(hawtioVersion, buildVariables.LegacyServingCertificateMountVersion)
	if err != nil {
		return nil, err
	}
	volumeMount = newVolumeMount(volumeMountName, volumeMountPath, volumeMountSubPath)
	volumeMounts = append(volumeMounts, volumeMount)

	if isOpenShift4 {
		volumeMountName = clientCertificateSecretVolumeName
		volumeMountPath = clientCertificateSecretVolumeMountPath
		volumeMount = newVolumeMount(volumeMountName, volumeMountPath, volumeMountSubPath)
		volumeMounts = append(volumeMounts, volumeMount)
	}

	return volumeMounts, nil
}

func getServingCertificateMountPathFor(version string, legacyServingCertificateMountVersion string) (string, error) {
	if len(version) == 0 {
		version = "latest"
	}
	if version != "latest" {
		semVer, err := semver.NewVersion(version)
		if err != nil {
			return "", err
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
