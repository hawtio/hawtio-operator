package deployments

import (
	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/hawtio/hawtio-operator/pkg/resources/containers"
	"github.com/hawtio/hawtio-operator/pkg/resources/environments"
	"github.com/hawtio/hawtio-operator/pkg/resources/pods"
	"github.com/hawtio/hawtio-operator/pkg/resources/volumes"
	"github.com/hawtio/hawtio-operator/pkg/util/selectors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("package deployments")

const (
	serviceSigningSecretVolumeName         = "hawtio-online-tls-serving"
	clientCertificateSecretVolumeName      = "hawtio-online-tls-proxying"
	clientCertificateSecretVolumeMountPath = "/etc/tls/private/proxying"
	deploymentRolloutAnnotation            = "hawtio.hawt.io/restartedAt"
	hawtioVersionAnnotation                = "hawtio.hawt.io/hawtioversion"
	hawtioTypeAnnotation                   = "hawtio.hawt.io/hawtioType"
	configVersionAnnotation                = "hawtio.hawt.io/configversion"
)

// move this to util
// Set labels in a map
func labelsForInterconnect(name string) map[string]string {
	return map[string]string{
		selectors.LabelAppKey:      name,
		selectors.LabelResourceKey: name,
	}
}

// Create NewDeploymentForCR method to create deployment
func NewDeploymentForCR(cr *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openshiftVersion string, openshiftURL string, volumePath string, configResourceVersion string) *appsv1.Deployment {

	reqLogger := log.WithName(cr.Name)
	reqLogger.Info("Creating new Deployment for custom resource")

	namespacedName := types.NamespacedName{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}

	annotations := map[string]string{
		configVersionAnnotation: configResourceVersion,
		hawtioTypeAnnotation:    cr.Spec.Type,
		hawtioVersionAnnotation: cr.GetResourceVersion(),
	}

	dep, Spec := MakeDeployment(namespacedName, annotations, cr.Spec.Replicas, NewPodTemplateSpecForCR(cr, isOpenShift4, openshiftVersion, openshiftURL, volumePath))

	dep.Spec = Spec

	return dep

}

func MakeDeployment(namespacedName types.NamespacedName, annotations map[string]string, replicas int32, pts corev1.PodTemplateSpec) (deployment *appsv1.Deployment, spec appsv1.DeploymentSpec) {

	labels := selectors.LabelsForHawtio(namespacedName.Name)
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
	}
	Spec := appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: pts, //pods.NewPodTemplateSpecForCR(cr),
		Strategy: appsv1.DeploymentStrategy{
			Type: "RollingUpdate",
		},
	}

	return dep, Spec

}

func NewPodTemplateSpecForCR(cr *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openshiftVersion string, openshiftURL string, volumePath string) corev1.PodTemplateSpec {
	namespacedName := types.NamespacedName{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}
	reqLogger := log.WithName(namespacedName.Name)
	reqLogger.Info("Creating new pod template spec for custom resource")

	pts := pods.MakePodTemplateSpec(namespacedName, selectors.LabelsForHawtio(cr.Name))

	Spec := corev1.PodSpec{}
	Containers := []corev1.Container{}
	container := containers.MakeContainer(cr.Name, cr.Spec.Version, MakeEnvVarArrayForCR(cr, isOpenShift4, openshiftVersion, openshiftURL))

	volumeMounts := MakeVolumeMounts(cr, isOpenShift4, volumePath)
	if len(volumeMounts) > 0 {
		container.VolumeMounts = volumeMounts
	}
	volumes := MakeVolumes(cr, isOpenShift4)
	if len(volumes) > 0 {
		Spec.Volumes = volumes
	}
	Spec.Containers = append(Containers, container)
	pts.Spec = Spec

	return pts

}

func MakeVolumes(cr *hawtiov1alpha1.Hawtio, isOpenShift4 bool) []corev1.Volume {

	reqLogger := log.WithName(cr.Name)
	reqLogger.Info("Creating new Volume for custom resource")

	volumeDefinitions := []corev1.Volume{}

	secretName := cr.Name + "-tls-serving"
	volumeName := serviceSigningSecretVolumeName
	volume := volumes.MakeVolume(secretName, volumeName)
	volumeDefinitions = append(volumeDefinitions, volume)

	if isOpenShift4 {

		secretName = cr.Name + "-tls-proxying"
		volumeName = clientCertificateSecretVolumeName
		volume = volumes.MakeVolume(secretName, volumeName)
		volumeDefinitions = append(volumeDefinitions, volume)

	}
	configMapName := cr.Name
	volumeName = "hawtio-online"
	volume = volumes.MakeConfigMapVolume(configMapName, volumeName)
	volumeDefinitions = append(volumeDefinitions, volume)

	configMapName = cr.Name
	volumeName = "hawtio-integration"
	volume = volumes.MakeConfigMapVolume(configMapName, volumeName)
	volumeDefinitions = append(volumeDefinitions, volume)

	return volumeDefinitions
}

func MakeEnvVarArrayForCR(cr *hawtiov1alpha1.Hawtio, isOpenShift4 bool, openshiftVersion string, openshiftURL string) []corev1.EnvVar {

	reqLogger := log.WithName(cr.Name)
	reqLogger.Info("Adding Env variable ")
	envVar := []corev1.EnvVar{}
	envVarArrayForCluster := environments.AddEnvVarForContainer(cr.Spec.Type, cr.Name)
	envVar = append(envVar, envVarArrayForCluster...)

	if isOpenShift4 {

		envVarArrayFoOpenShift := environments.AddEnvVarForOpenshift(openshiftVersion, openshiftURL)
		envVar = append(envVar, envVarArrayFoOpenShift...)
	}

	return envVar
}

func MakeVolumeMounts(cr *hawtiov1alpha1.Hawtio, isOpenShift4 bool, volumePath string) []corev1.VolumeMount {

	reqLogger := log.WithName(cr.Name)
	reqLogger.Info("Creating new Volume Mounts for custom resource")

	volumeMounts := []corev1.VolumeMount{}

	volumeMountSubPath := "hawtconfig.json"
	volumeMountName := "hawtio-online"
	volumeMountNamepath := "/usr/share/nginx/html/online/hawtconfig.json"
	volumeMount := volumes.MakeVolumeMount(volumeMountName, volumeMountNamepath, volumeMountSubPath)
	volumeMounts = append(volumeMounts, volumeMount)

	volumeMountSubPath = "hawtconfig.json"
	volumeMountName = "hawtio-integration"
	volumeMountNamepath = "/usr/share/nginx/html/integration/hawtconfig.json"
	volumeMount = volumes.MakeVolumeMount(volumeMountName, volumeMountNamepath, volumeMountSubPath)
	volumeMounts = append(volumeMounts, volumeMount)

	volumeMountSubPath = ""
	volumeMountName = serviceSigningSecretVolumeName
	volumeMountNamepath = volumePath
	volumeMount = volumes.MakeVolumeMount(volumeMountName, volumeMountNamepath, volumeMountSubPath)
	volumeMounts = append(volumeMounts, volumeMount)

	if isOpenShift4 {

		volumeMountName = clientCertificateSecretVolumeName
		volumeMountNamepath = clientCertificateSecretVolumeMountPath
		volumeMount = volumes.MakeVolumeMount(volumeMountName, volumeMountNamepath, volumeMountSubPath)
		volumeMounts = append(volumeMounts, volumeMount)

	}

	return volumeMounts
}

func GetEnvVarByName(env []corev1.EnvVar, name string) (*corev1.EnvVar, int) {
	for i, v := range env {
		if v.Name == name {
			return &env[i], i
		}
	}
	return nil, -1
}

func GetVolumeMount(container corev1.Container, name string) (*corev1.VolumeMount, int) {
	for i, vm := range container.VolumeMounts {
		if vm.Name == name {
			return &container.VolumeMounts[i], i
		}
	}
	return nil, -1
}

func GetDeployment(objects []runtime.Object) *appsv1.Deployment {
	for _, object := range objects {
		if deployment, ok := object.(*appsv1.Deployment); ok {
			return deployment
		}
	}
	return nil
}
