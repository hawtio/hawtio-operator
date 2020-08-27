package resources

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
)

const (
	serviceSigningSecretVolumeName         = "hawtio-online-tls-serving"
	clientCertificateSecretVolumeName      = "hawtio-online-tls-proxying"
	clientCertificateSecretVolumeMountPath = "/etc/tls/private/proxying"
	configVersionAnnotation                = "hawtio.hawt.io/configversion"
)

// Create NewDeploymentForCR method to create deployment
func NewDeploymentForCR(cr *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openshiftVersion string, openshiftURL string, volumePath string, configResourceVersion string) *appsv1.Deployment {
	namespacedName := types.NamespacedName{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}

	annotations := map[string]string{
		configVersionAnnotation: configResourceVersion,
	}

	dep := newDeployment(namespacedName, annotations, cr.Spec.Replicas, newPodTemplateSpecForCR(cr, isOpenShift4, openshiftVersion, openshiftURL, volumePath))

	return dep
}

func newDeployment(namespacedName types.NamespacedName, annotations map[string]string, replicas *int32, pts corev1.PodTemplateSpec) *appsv1.Deployment {
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
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        namespacedName.Name,
			Namespace:   namespacedName.Namespace,
			Labels:      labels,
			Annotations: annotations,
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

	return dep
}

func newPodTemplateSpecForCR(cr *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openshiftVersion string, openshiftURL string, volumePath string) corev1.PodTemplateSpec {
	namespacedName := types.NamespacedName{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}

	pts := newPodTemplateSpec(namespacedName, labelsForHawtio(cr.Name))

	spec := corev1.PodSpec{}
	var Containers []corev1.Container
	container := NewContainer(cr.Name, cr.Spec.Version, newEnvVarArrayForCR(cr, isOpenShift4, openshiftVersion, openshiftURL))

	volumeMounts := newVolumeMounts(cr, isOpenShift4, volumePath)
	if len(volumeMounts) > 0 {
		container.VolumeMounts = volumeMounts
	}
	v := newVolumes(cr, isOpenShift4)
	if len(v) > 0 {
		spec.Volumes = v
	}
	spec.Containers = append(Containers, container)
	pts.Spec = spec

	return pts
}

func newVolumes(cr *hawtiov1alpha1.Hawtio, isOpenShift4 bool) []corev1.Volume {
	var volumes []corev1.Volume

	secretName := cr.Name + "-tls-serving"
	volumeName := serviceSigningSecretVolumeName
	volume := newVolume(secretName, volumeName)
	volumes = append(volumes, volume)

	if isOpenShift4 {
		secretName = cr.Name + "-tls-proxying"
		volumeName = clientCertificateSecretVolumeName
		volume = newVolume(secretName, volumeName)
		volumes = append(volumes, volume)
	}

	configMapName := cr.Name
	volumeName = "hawtio-online"
	volume = newConfigMapVolume(configMapName, volumeName)
	volumes = append(volumes, volume)

	configMapName = cr.Name
	volumeName = "hawtio-integration"
	volume = newConfigMapVolume(configMapName, volumeName)
	volumes = append(volumes, volume)

	return volumes
}

func newEnvVarArrayForCR(cr *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openshiftVersion string, openshiftURL string) []corev1.EnvVar {
	var envVar []corev1.EnvVar

	envVarArrayForCluster := addEnvVarForContainer(cr.Spec.Type, cr.Name)
	envVar = append(envVar, envVarArrayForCluster...)

	if isOpenShift4 {
		envVarArrayFoOpenShift := addEnvVarForOpenshift(openshiftVersion, openshiftURL)
		envVar = append(envVar, envVarArrayFoOpenShift...)
	}

	return envVar
}

func newVolumeMounts(cr *hawtiov1alpha1.Hawtio, isOpenShift4 bool, volumePath string) []corev1.VolumeMount {
	var volumeMounts []corev1.VolumeMount

	volumeMountSubPath := hawtioConfigKey
	volumeMountName := "hawtio-online"
	volumeMountPath := "/usr/share/nginx/html/online/hawtconfig.json"
	volumeMount := newVolumeMount(volumeMountName, volumeMountPath, volumeMountSubPath)
	volumeMounts = append(volumeMounts, volumeMount)

	volumeMountSubPath = hawtioConfigKey
	volumeMountName = "hawtio-integration"
	volumeMountPath = "/usr/share/nginx/html/integration/hawtconfig.json"
	volumeMount = newVolumeMount(volumeMountName, volumeMountPath, volumeMountSubPath)
	volumeMounts = append(volumeMounts, volumeMount)

	volumeMountSubPath = ""
	volumeMountName = serviceSigningSecretVolumeName
	volumeMountPath = volumePath
	volumeMount = newVolumeMount(volumeMountName, volumeMountPath, volumeMountSubPath)
	volumeMounts = append(volumeMounts, volumeMount)

	if isOpenShift4 {
		volumeMountName = clientCertificateSecretVolumeName
		volumeMountPath = clientCertificateSecretVolumeMountPath
		volumeMount = newVolumeMount(volumeMountName, volumeMountPath, volumeMountSubPath)
		volumeMounts = append(volumeMounts, volumeMount)
	}

	return volumeMounts
}
