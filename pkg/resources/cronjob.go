package resources

import (
	"strconv"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewDefaultCronJob(hawtio *hawtiov2.Hawtio) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hawtio.Name + "-certificate-expiry-check",
			Namespace: hawtio.Namespace,
		},
	}
}

func NewCronJob(hawtio *hawtiov2.Hawtio, pod *corev1.Pod, namespace string) (*batchv1.CronJob, error) {
	cronJob := NewDefaultCronJob(hawtio)

	//create cronJob to validate the Cert
	populateCertValidationCronJob(cronJob, hawtio, pod, namespace)

	if err := updateExpirationPeriod(cronJob, hawtio.Spec.Auth.ClientCertExpirationPeriod); err != nil {
		return nil, err
	}

	return cronJob, nil
}

func populateCertValidationCronJob(cronJob *batchv1.CronJob, hawtio *hawtiov2.Hawtio, pod *corev1.Pod, namespace string) {
	schedule := hawtio.Spec.Auth.ClientCertCheckSchedule
	serviceAccountName := pod.Spec.ServiceAccountName
	container := pod.Spec.Containers[0]
	period := hawtio.Spec.Auth.ClientCertExpirationPeriod

	if period == 0 {
		period = 24
	}

	cronJob.Spec = batchv1.CronJobSpec{
		Schedule:          schedule,
		ConcurrencyPolicy: batchv1.ForbidConcurrent,
		JobTemplate: batchv1.JobTemplateSpec{
			Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						ServiceAccountName: serviceAccountName,
						RestartPolicy:      "Never",
						Containers: []corev1.Container{
							{
								Name:  container.Name,
								Image: container.Image,
								Command: []string{
									"hawtio-operator",
								},
								Args: []string{
									"cert-expiry-check",
									"--cert-namespace",
									namespace,
									"--cert-expiration-period",
									strconv.Itoa(period),
								},
								ImagePullPolicy: "Always",
							},
						},
					},
				},
			},
		},
	}
}

func updateExpirationPeriod(cronJob *batchv1.CronJob, newPeriod int) error {
	arguments := cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Args
	for i, arg := range arguments {
		if arg == "--cert-expiration-period" {
			period, err := strconv.Atoi(arguments[i+1])
			if err != nil {
				return err
			}
			if period == newPeriod {
				return nil
			}
			cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Args[i+1] = strconv.Itoa(newPeriod)
			return nil
		}
	}
	return nil
}
