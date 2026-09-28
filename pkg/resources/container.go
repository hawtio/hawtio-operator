package resources

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/go-logr/logr"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/resources/common"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

func newHawtioContainer(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec, openShiftConsoleURL string, bv util.BuildVariables, log logr.Logger) corev1.Container {
	/*
	 * - name: hawtio-online-container
	 *   image: quay.io/hawtio/online
	 *   imagePullPolicy: Always
	 *   ports:
	 *   - name: nginx
	 *     containerPort: 8443
	 *   livenessProbe:
	 *     httpGet:
	 *       path: /online
	 *       port: nginx
	 *       scheme: HTTPS
	 *     periodSeconds: 10
	 *     timeoutSeconds: 1
	 *   readinessProbe:
	 *     httpGet:
	 *       path: /online
	 *       port: nginx
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
	hawtioVersion := bv.GetOnlineVersion()
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Using Hawtio Image Version: %s", hawtioVersion))

	envVars := newHawtioEnvVars(hawtio, apiSpec, openShiftConsoleURL)
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Hawtio Container Env Vars %s", util.ToJSONString(envVars)))

	connect := common.PlainConnect
	if util.IsSSL(hawtio, apiSpec) {
		connect = common.SSLConnect
	}

	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Hawtio Container protocols %s", util.ToJSONString(connect)))

	var readinessPeriodValue int32 = common.OnlineReadinessPeriodValue
	if hawtio.Spec.HealthChecks.OnlineReadinessPeriod != nil {
		readinessPeriodValue = *hawtio.Spec.HealthChecks.OnlineReadinessPeriod
	}

	var livenessPeriodValue int32 = common.OnlineLivenessPeriodValue
	if hawtio.Spec.HealthChecks.OnlineLivenessPeriod != nil {
		livenessPeriodValue = *hawtio.Spec.HealthChecks.OnlineLivenessPeriod
	}

	container := corev1.Container{
		Name:            hawtio.Name + "-container",
		Image:           bv.ImageURI(bv.GetOnlineImageRepo(), hawtioVersion),
		ImagePullPolicy: common.GetImagePullPolicy(),
		Env:             envVars,
		ReadinessProbe: &corev1.Probe{
			InitialDelaySeconds: 5,
			TimeoutSeconds:      1,
			PeriodSeconds:       readinessPeriodValue,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString(common.ContainerPortName),
					Path:   "/online",
					Scheme: connect.Protocol,
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			TimeoutSeconds: 1,
			PeriodSeconds:  livenessPeriodValue,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString(common.ContainerPortName),
					Path:   "/online",
					Scheme: connect.Protocol,
				},
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          common.ContainerPortName,
				ContainerPort: connect.Port,
				Protocol:      "TCP",
			},
		},
		Resources: hawtio.Spec.Resources,
	}

	return container
}

func newGatewayContainer(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec, bv util.BuildVariables, log logr.Logger) corev1.Container {
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
	gatewayVersion := bv.GetOnlineGatewayVersion()
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Using Hawtio Gateway Image Version: %s", gatewayVersion))

	envVars := newGatewayEnvVars(hawtio, apiSpec)
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Gateway Container Env Vars %s", util.ToJSONString(envVars)))

	connect := common.PlainConnect
	if util.IsSSL(hawtio, apiSpec) {
		connect = common.SSLConnect
	}
	log.V(util.DebugLogLevel).Info(fmt.Sprintf("Gateway Container protocols %s", util.ToJSONString(connect)))

	var readinessPeriodValue int32 = common.GatewayReadinessPeriodValue
	if hawtio.Spec.HealthChecks.GatewayReadinessPeriod != nil {
		readinessPeriodValue = *hawtio.Spec.HealthChecks.GatewayReadinessPeriod
	}

	var livenessPeriodValue int32 = common.GatewayLivenessPeriodValue
	if hawtio.Spec.HealthChecks.GatewayLivenessPeriod != nil {
		livenessPeriodValue = *hawtio.Spec.HealthChecks.GatewayLivenessPeriod
	}

	container := corev1.Container{
		Name:            hawtio.Name + "-gateway-container",
		Image:           bv.ImageURI(bv.GetOnlineGatewayImageRepo(), gatewayVersion),
		ImagePullPolicy: common.GetImagePullPolicy(),
		Env:             envVars,
		Ports: []corev1.ContainerPort{
			{
				Name:          common.ContainerGatewayPortName,
				ContainerPort: 3000,
				Protocol:      "TCP",
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString(common.ContainerGatewayPortName),
					Path:   "/status",
					Scheme: connect.Protocol,
				},
			},
			PeriodSeconds:  livenessPeriodValue,
			TimeoutSeconds: 1,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.FromString(common.ContainerGatewayPortName),
					Path:   "/status",
					Scheme: connect.Protocol,
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       readinessPeriodValue,
			TimeoutSeconds:      1,
		},
		Resources: hawtio.Spec.Resources,
	}

	return container
}

func newHawtioEnvVars(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec, openShiftConsoleURL string) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	envVarsForHawtio := envVarsForHawtio(hawtio, apiSpec)
	envVars = append(envVars, envVarsForHawtio...)

	if apiSpec.IsOpenShift4 {
		envVarsForOpenShift4 := envVarsForHawtioOCP4(apiSpec.Version, openShiftConsoleURL)
		envVars = append(envVars, envVarsForOpenShift4...)
	}

	envVarsForNginx := common.EnvVarsForNginx(hawtio.GetNginxOptions())
	envVars = append(envVars, envVarsForNginx...)

	return envVars
}

func newGatewayEnvVars(hawtio *hawtiov2.Hawtio, apiSpec *capabilities.ApiServerSpec) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	envVarsForGateway := envVarsForGateway(hawtio, apiSpec)
	envVars = append(envVars, envVarsForGateway...)

	envVarsForRBAC := common.EnvVarsForRBAC(hawtio.GetRbacOptions())
	envVars = append(envVars, envVarsForRBAC...)

	return envVars
}
