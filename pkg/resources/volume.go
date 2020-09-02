package resources

import (
	corev1 "k8s.io/api/core/v1"
)

func newVolumeMount(volumeMountName string, volumeMountPath string, volumeMountSubPath string) corev1.VolumeMount {
	volumeMount := corev1.VolumeMount{
		Name:      volumeMountName,
		MountPath: volumeMountPath,
		SubPath:   volumeMountSubPath,
	}

	return volumeMount
}

func newSecretVolume(secretName string, volumeName string) corev1.Volume {
	volume := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretName,
			},
		},
	}
	return volume
}

func newConfigMapVolume(configMapName string, volumeName string) corev1.Volume {
	executeMode := int32(420)
	volume := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
				DefaultMode: &executeMode,
			},
		},
	}

	return volume
}
