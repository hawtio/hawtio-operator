package util

import (
	v1 "k8s.io/api/core/v1"
)

func GetEnvVarByName(env []v1.EnvVar, name string) (*v1.EnvVar, int) {
	for i, v := range env {
		if v.Name == name {
			return &env[i], i
		}
	}
	return nil, -1
}
