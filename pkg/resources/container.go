package resources

import (
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	hawtiov1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1"
)

const containerPortName = "https"
const containerGatewayPortName = "express"

func newHawtioContainer(hawtio *hawtiov1.Hawtio, envVars []corev1.EnvVar, imageVersion string, imageRepository string) corev1.Container {
	/*
	 * - name: hawtio-online-container
	 *   image: quay.io/hawtio/online
	 *   imagePullPolicy: Always
	 *   ports:
	 *   - name: https
	 *     containerPort: 8443
	 *   livenessProbe:
	 *     httpGet:
	 *       path: /online
	 *       port: https
	 *       scheme: HTTPS
	 *     periodSeconds: 10
	 *     timeoutSeconds: 1
	 *   readinessProbe:
	 *     httpGet:
	 *       path: /online
	 *       port: https
	 *       scheme: HTTPS
	 *     initialDelaySeconds: 5
	 *     periodSeconds: 5
	 *     timeoutSeconds: 1
	 *   resources:
	 *     requests:
	 *       cpu: "0.2"
	 *       memory: 32Mi
	 *     limits:
	 *       cpu: "1.0"
	 *       memory: 500Mi
	 *   volumeMounts:
	 *     - name: hawtio-online-tls-serving
	 *       mountPath: /etc/tls/private/serving
	 */
	container := corev1.Container{
		Name:            hawtio.Name + "-container",
		Image:           getHawtioImageFor(imageVersion, imageRepository),
		ImagePullPolicy: getImagePullPolicy(),
		Env:             envVars,
		ReadinessProbe: &corev1.Probe{
			InitialDelaySeconds: 5,
			TimeoutSeconds:      1,
			PeriodSeconds:       5,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString(containerPortName),
					Path:   "/online",
					Scheme: "HTTPS",
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			TimeoutSeconds: 1,
			PeriodSeconds:  10,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString(containerPortName),
					Path:   "/online",
					Scheme: "HTTPS",
				},
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          containerPortName,
				ContainerPort: 8443,
				Protocol:      "TCP",
			},
		},
		Resources: hawtio.Spec.Resources,
	}

	return container
}

func newGatewayContainer(hawtio *hawtiov1.Hawtio, envVars []corev1.EnvVar, imageVersion string, imageGatewayRepository string) corev1.Container {
	/*
	 * - name: hawtio-online-gateway-container
	 *   image: quay.io/hawtio/online-gateway
	 *   ports:
	 *     - name: express
	 *       containerPort: 3000
	 *   livenessProbe:
	 *      httpGet:
	 *        path: /status
	 *        port: express
	 *        scheme: HTTPS
	 *      periodSeconds: 120
	 *      timeoutSeconds: 1
	 *   readinessProbe:
	 *      httpGet:
	 *        path: /status
	 *        port: express
	 *        scheme: HTTPS
	 *      initialDelaySeconds: 5
	 *      periodSeconds: 30
	 *      timeoutSeconds: 1
	 */
	container := corev1.Container{
		Name:            hawtio.Name + "-gateway-container",
		Image:           getGatewayImageFor(imageVersion, imageGatewayRepository),
		ImagePullPolicy: getImagePullPolicy(),
		Env:             envVars,
		Ports: []corev1.ContainerPort{
			{
				Name:          containerGatewayPortName,
				ContainerPort: 3000,
				Protocol:      "TCP",
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString(containerGatewayPortName),
					Path:   "/status",
					Scheme: "HTTPS",
				},
			},
			PeriodSeconds:  10,
			TimeoutSeconds: 1,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString(containerGatewayPortName),
					Path:   "/status",
					Scheme: "HTTPS",
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
			TimeoutSeconds:      1,
		},
		Resources: hawtio.Spec.Resources,
	}

	return container
}

func getHawtioImageFor(tag string, imageRepository string) string {
	return getImageFor(tag, imageRepository, "IMAGE_REPOSITORY", "quay.io/hawtio/online")
}

func getGatewayImageFor(tag string, gatewayImgRepository string) string {
	return getImageFor(tag, gatewayImgRepository, "GATEWAY_IMAGE_REPOSITORY", "quay.io/hawtio/online-gateway")
}

func getImageFor(tag string, imgRepo string, envVar string, defaultVal string) string {
	repository := os.Getenv(envVar)
	if repository == "" {
		if imgRepo != "" {
			repository = imgRepo
		} else {
			repository = defaultVal
		}
	}

	if strings.HasPrefix(tag, "sha256:") {
		// tag is a sha checksum tag
		return repository + "@" + tag
	}

	return repository + ":" + tag
}

func getImagePullPolicy() corev1.PullPolicy {
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
