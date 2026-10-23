package gateway

import (
	// "github.com/Masterminds/semver"
	"fmt"

	"github.com/go-logr/logr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labelUtils "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	hawtiopluginv1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtioplugin/v1"
	"github.com/hawtio/hawtio-operator/pkg/cfg"
	commonres "github.com/hawtio/hawtio-operator/pkg/resources/common"
	pluginres "github.com/hawtio/hawtio-operator/pkg/resources/hawtioplugin"

	"github.com/hawtio/hawtio-operator/pkg/util"
)

const (
	deploymentSSLPort = 8443

	// Serving Certificate
	serviceSigningSecretVolumeName = "hawtio-online-console-plugin-gateway-serving"

	// Proxy Certificate
	clientCertificateSecretVolumeName = "hawtio-online-console-plugin-gateway-proxying"
)

func NewDefaultDeployment(namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayName,
			Namespace: namespace,
		},
	}
}

func NewDeployment(hawtioPlugin *hawtiopluginv1.HawtioPlugin, namespace string, buildVariables util.BuildVariables, deploymentConfig cfg.DeploymentConfiguration, log logr.Logger) (*appsv1.Deployment, error) {
	log.V(util.DebugLogLevel).Info("Reconciling deployment")

	podTemplateSpec, err := newPodTemplateSpec(hawtioPlugin, buildVariables, deploymentConfig, log)
	if err != nil {
		return nil, err
	}
	return newDeployment(hawtioPlugin, namespace, buildVariables, podTemplateSpec, log)
}

func deploymentLabels(buildVariables util.BuildVariables) (map[string]string, error) {
	additionalLabels, err := labelUtils.ConvertSelectorToLabelsMap(buildVariables.AdditionalLabels)
	if err != nil {
		return nil, err
	}

	labels := commonres.LabelsForHawtio(gatewayName)
	labels["app.kubernetes.io/component"] = gatewayName
	labels["app.kubernetes.io/instance"] = gatewayName
	labels["app.kubernetes.io/part-of"] = commonres.LabelAppValue

	for name, value := range additionalLabels {
		labels[name] = value
	}

	return labels, nil
}

func newDeployment(hawtioPlugin *hawtiopluginv1.HawtioPlugin, namespace string, buildVariables util.BuildVariables, pts corev1.PodTemplateSpec, log logr.Logger) (*appsv1.Deployment, error) {
	log.V(util.DebugLogLevel).Info("New deployment")

	/*
	 * apiVersion: apps/v1
	 * kind: Deployment
	 * metadata:
	 *   labels:
	 *     app: hawtio-online-console-plugin
	 *     app.kubernetes.io/component: hawtio-online-console-plugin-gateway
	 *     app.kubernetes.io/instance: hawtio-online-console-plugin-gateway
	 *     app.kubernetes.io/part-of: hawtio-online-console-plugin
	 *   name: hawtio-online-console-plugin-gateway
	 *   namespace: <operator namespace>
	 * spec:
	 *   selector:
	 *     matchLabels:
	 *       app: hawtio-online-console-plugin-gateway
	 *	 template: ....
	 *   strategy:
	 *     rollingUpdate:
	 *       maxSurge: 25%
	 *       maxUnavailable: 25%
	 *     type: RollingUpdate
	 */

	deployment := NewDefaultDeployment(namespace)

	annotations := map[string]string{}
	commonres.PropagateAnnotations(hawtioPlugin, annotations, log)

	labels, err := deploymentLabels(buildVariables)
	if err != nil {
		return nil, err
	}
	commonres.PropagateLabels(hawtioPlugin, labels, log)

	deployment.SetAnnotations(annotations)
	deployment.SetLabels(labels)
	deployment.Spec = appsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: deployment.Labels,
		},
		Template: pts,
		Strategy: appsv1.DeploymentStrategy{
			Type: appsv1.RollingUpdateDeploymentStrategyType,
		},
	}
	return deployment, nil
}

/**
 * Creates a new pod template
 */
func newPodTemplateSpec(hawtioPlugin *hawtiopluginv1.HawtioPlugin, buildVariables util.BuildVariables, deploymentConfig cfg.DeploymentConfiguration, log logr.Logger) (corev1.PodTemplateSpec, error) {
	log.V(util.DebugLogLevel).Info("New Pod Template Spec")

	gatewayContainer := newContainer(hawtioPlugin, buildVariables, log)

	labels, err := deploymentLabels(buildVariables)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	commonres.PropagateLabels(hawtioPlugin, labels, log)

	annotations := map[string]string{}
	commonres.PropagateAnnotations(hawtioPlugin, annotations, log)

	/*
	 *   template:
	 *     metadata:
	 *       labels:
	 *         app: hawtio-online-console-plugin-gateway
	 *    spec:
	 *       containers: ....
	 *       volumes: ....
	 */

	volumes := newVolumes(hawtioPlugin, deploymentConfig, log)

	pod := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
			// Used to trigger a rollout deployment if config changed,
			// similarly to `kubectl rollout restart`
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				gatewayContainer,
			},
			ServiceAccountName: pluginres.ServiceAccountName,
			Volumes:            volumes,
		},
	}

	log.V(util.DebugLogLevel).Info(fmt.Sprintf("PodTemplateSpec: %s", util.ToJSONString(pod)))

	return pod, err
}

func newContainer(hawtioPlugin *hawtiopluginv1.HawtioPlugin, bv util.BuildVariables, log logr.Logger) corev1.Container {
	/*
	 *       containers:
	 *       - env: envVarsForGateway ....
	 */

	envVars := envVarsForGateway(hawtioPlugin)
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Gateway Container Env Vars %s", util.ToJSONString(envVars)))

	/*
	 * image: quay.io/hawtio/hawtio-online-console-plugin-gateway:0.9.0
	 * imagePullPolicy: IfNotPresent
	 * name: hawtio-online-console-plugin-gateway
	 * ports:
	 * - containerPort: 8443
	 *   name: express
	 *   protocol: TCP
	 * resources: {}
	 */
	gatewayVersion := bv.GetPluginGatewayVersion()
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Using Hawtio Plugin Gateway Image Version: %s", gatewayVersion))

	/*
	 * readinessProbe:
	 *   failureThreshold: 3
	 *   httpGet:
	 *     port: express
	 *     path: /status
	 *     scheme: HTTPS
	 *   initialDelaySeconds: 5
	 *   periodSeconds: 30
	 *   successThreshold: 1
	 *   timeoutSeconds: 1
	 */
	var readinessPeriodValue int32 = commonres.GatewayReadinessPeriodValue
	if hawtioPlugin.Spec.HealthChecks.GatewayReadinessPeriod != nil {
		readinessPeriodValue = *hawtioPlugin.Spec.HealthChecks.GatewayReadinessPeriod
	}

	/*
	 * livenessProbe:
	 *   failureThreshold: 3
	 *   httpGet:
	 *     port: express
	 *     path: /status
	 *     scheme: HTTPS
	 *   periodSeconds: 120
	 *   successThreshold: 1
	 *   timeoutSeconds: 1
	 */
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Gateway Container protocols %s", util.ToJSONString(commonres.SSLConnect)))

	var livenessPeriodValue int32 = commonres.GatewayLivenessPeriodValue
	if hawtioPlugin.Spec.HealthChecks.GatewayLivenessPeriod != nil {
		livenessPeriodValue = *hawtioPlugin.Spec.HealthChecks.GatewayLivenessPeriod
	}

	/*
	 * volumeMounts: ...
	 */
	volumeMounts := newVolumeMounts(hawtioPlugin, bv, log)

	container := corev1.Container{
		Name:            hawtioPlugin.Name + "-gateway-container",
		Image:           bv.ImageURI(bv.GetPluginGatewayImageRepo(), gatewayVersion),
		ImagePullPolicy: commonres.GetImagePullPolicy(),
		Env:             envVars,
		Ports: []corev1.ContainerPort{
			{
				Name:          commonres.ContainerGatewayPortName,
				ContainerPort: deploymentSSLPort,
				Protocol:      "TCP",
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString(commonres.ContainerGatewayPortName),
					Path:   "/status",
					Scheme: commonres.SSLConnect.Protocol,
				},
			},
			PeriodSeconds:  livenessPeriodValue,
			TimeoutSeconds: 1,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString(commonres.ContainerGatewayPortName),
					Path:   "/status",
					Scheme: commonres.SSLConnect.Protocol,
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       readinessPeriodValue,
			TimeoutSeconds:      1,
		},
		Resources:    hawtioPlugin.Spec.Resources,
		VolumeMounts: volumeMounts,
	}

	return container
}

func newVolumeMounts(hawtioPlugin *hawtiopluginv1.HawtioPlugin, buildVariables util.BuildVariables, log logr.Logger) []corev1.VolumeMount {
	/*
	 * - mountPath: /etc/hawtio/rbac
	 *   name: hawtio-rbac
	 * - mountPath: /etc/tls/private/serving
	 *   name: hawtio-serving
	 *   readOnly: true
	 * - mountPath: /etc/tls/private/proxying
	 *   name: hawtio-proxying
	 *   readOnly: true
	 */

	volumeMounts := []corev1.VolumeMount{}

	/*
	 * The rbac volume
	 */
	if hawtioPlugin.Spec.RBAC.ConfigMap != "" {
		log.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding volume mount %s at %s", commonres.RbacConfigMapVolumeName, commonres.RbacConfigMapVolumeMountPath))
		volumeMount := commonres.NewVolumeMount(commonres.RbacConfigMapVolumeName, commonres.RbacConfigMapVolumeMountPath, "")
		volumeMounts = append(volumeMounts, volumeMount)
	}

	/*
	 * The serving-certificate volume
	 */
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding volume mount %s at %s", serviceSigningSecretVolumeName, commonres.ServiceSigningSecretVolumeMountPath))
	volumeMount := commonres.NewVolumeMount(serviceSigningSecretVolumeName, commonres.ServiceSigningSecretVolumeMountPath, "")
	volumeMounts = append(volumeMounts, volumeMount)

	/*
	 * The proxying volume
	 */
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding volume mount %s at %s", clientCertificateSecretVolumeName, commonres.ClientCertificateSecretVolumeMountPath))
	volumeMount = commonres.NewVolumeMount(clientCertificateSecretVolumeName, commonres.ClientCertificateSecretVolumeMountPath, "")
	volumeMounts = append(volumeMounts, volumeMount)

	log.V(util.DebugLogLevel).Info(fmt.Sprintf("New VolumeMounts %s", util.ToJSONString(volumeMounts)))
	return volumeMounts
}

func newVolumes(hawtioPlugin *hawtiopluginv1.HawtioPlugin, config cfg.DeploymentConfiguration, log logr.Logger) []corev1.Volume {
	/*
	 * volumes:
	 *   - name: hawtio-serving
	 *     secret:
	 *       defaultMode: 420
	 *       secretName: hawtio-online-console-plugin-gateway-serving
	 *   - name: hawtio-proxying
	 *     secret:
	 *       defaultMode: 420
	 *       secretName: hawtio-online-console-plugin-gateway-proxying
	 *   - name: hawtio-rbac
	 *     configMap:
	 *       defaultMode: 420
	 *       name: hawtio-online-console-plugin-rbac
	 */

	log.V(util.DebugLogLevel).Info("Creating new volumes")

	var volumes []corev1.Volume

	if config.ServingCertSecretName != nil {
		log.V(util.DebugLogLevel).Info("Adding secret volume for serving certificate %s-tls-serving at %s", hawtioPlugin.Name, serviceSigningSecretVolumeName)
		volume := commonres.NewSecretVolume(*config.ServingCertSecretName, serviceSigningSecretVolumeName)
		volumes = append(volumes, volume)
	}

	if config.ClientCertSecretName != nil {
		log.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding secret volume for proxying certificate %s-tls-proxying at %s", hawtioPlugin.Name, clientCertificateSecretVolumeName))
		volume := commonres.NewSecretVolume(*config.ClientCertSecretName, clientCertificateSecretVolumeName)
		volumes = append(volumes, volume)
	}

	if rbacConfigMapName := hawtioPlugin.Spec.RBAC.ConfigMap; rbacConfigMapName != "" {
		log.V(util.DebugLogLevel).Info(fmt.Sprintf("Adding config map volume %s at %s", rbacConfigMapName, commonres.RbacConfigMapVolumeName))
		volume := commonres.NewConfigMapVolume(rbacConfigMapName, commonres.RbacConfigMapVolumeName)
		volumes = append(volumes, volume)
	}

	return volumes
}
