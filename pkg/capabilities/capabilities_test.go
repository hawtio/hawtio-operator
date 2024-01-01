/*
 * Copyright (C) 2019 Red Hat, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package capabilities

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"

	configv1 "github.com/openshift/api/config/v1"
	fakeconfig "github.com/openshift/client-go/config/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakekube "k8s.io/client-go/kubernetes/fake"
)

func Test_ApiCapabilities(t *testing.T) {

	res1 := metav1.APIResourceList{
		GroupVersion: "image.openshift.io/v1",
		APIResources: []metav1.APIResource{
			{Name: "imagestreams"},
		},
	}
	res2 := metav1.APIResourceList{
		GroupVersion: "route.openshift.io/v1",
		APIResources: []metav1.APIResource{
			{Name: "routes"},
		},
	}
	res3 := metav1.APIResourceList{
		GroupVersion: "oauth.openshift.io/v1",
		APIResources: []metav1.APIResource{
			{Name: "oauthclientauthorizations"},
		},
	}
	res3a := metav1.APIResourceList{
		GroupVersion: "console.openshift.io/v1",
		APIResources: []metav1.APIResource{
			{Name: "consolelinks"},
		},
	}

	res4 := metav1.APIResourceList{
		GroupVersion: "something.openshift.io/v1",
	}
	res5 := metav1.APIResourceList{
		GroupVersion: "not.anything.io/v1",
	}
	res6 := metav1.APIResourceList{
		GroupVersion: "something.else.io/v1",
	}

	startTime, _ := time.Parse(time.RFC3339, "2023-09-18T15:35:39Z")

	clusterVersion := &configv1.ClusterVersion{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterVersion",
			APIVersion: "config.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: configv1.ClusterID("86c5af53-0a82-4d59-957a-4c5a74caf26d"),
		},
		Status: configv1.ClusterVersionStatus{
			Desired: configv1.Release{
				Image:   "quay.io/crcont/ocp-release@sha256:4769b0a12fbbd7c2d6846a6bcf69761a1339d39bd96be8d44c122eac032caad1",
				Version: "4.13.12",
			},
			History: []configv1.UpdateHistory{
				{
					State:          configv1.CompletedUpdate,
					StartedTime:    metav1.NewTime(startTime),
					CompletionTime: nil,
					Image:          "quay.io/crcont/ocp-release@sha256:4769b0a12fbbd7c2d6846a6bcf69761a1339d39bd96be8d44c122eac032caad1",
					Verified:       false,
					Version:        "4.13.12",
				},
			},
		},
	}

	testCases := []struct {
		name      string
		resList   []*metav1.APIResourceList
		expected  ApiServerSpec
		osversion *configv1.ClusterVersion
	}{
		{
			"Relevant APIs available for fully true api spec",
			[]*metav1.APIResourceList{&res1, &res2, &res3, &res3a},
			ApiServerSpec{
				Version:           "4.13.12",
				KubeVersion:       "1.26",
				IsOpenShift4:      true,
				IsOpenShift43Plus: true,
				ImageStreams:      true,
				Routes:            true,
				ConsoleLink:       true,
			},
			clusterVersion,
		},
		{
			"No relevant resources so expect false",
			[]*metav1.APIResourceList{&res4, &res5, &res6},
			ApiServerSpec{
				Version:           "1.26",
				KubeVersion:       "1.26",
				IsOpenShift4:      false,
				IsOpenShift43Plus: false,
				ImageStreams:      false,
				Routes:            false,
			},
			nil,
		},
		{
			"No resources so everything false",
			[]*metav1.APIResourceList{},
			ApiServerSpec{
				Version:           "1.26",
				KubeVersion:       "1.26",
				IsOpenShift4:      false,
				IsOpenShift43Plus: false,
				ImageStreams:      false,
				Routes:            false,
			},
			nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			api := fakekube.NewSimpleClientset()
			fd := api.Discovery().(*fakediscovery.FakeDiscovery)
			fd.Resources = tc.resList
			fd.FakedServerVersion = &version.Info{
				Major: "1",
				Minor: "26",
			}

			var configObjects []runtime.Object
			if tc.osversion != nil {
				configObjects = append(configObjects, tc.osversion)
			}
			configClient := fakeconfig.NewSimpleClientset(configObjects...)

			apiSpec, err := APICapabilities(context.TODO(), api, configClient)
			if err != nil {
				t.Error(err)
			}

			if apiSpec == nil {
				t.Error("Failed to return an api specification")
			}

			if apiSpec.Version != tc.expected.Version {
				t.Error("Expected api specification version not expected", "actual", apiSpec.Version, "expected", tc.expected.Version)
			}

			if apiSpec.KubeVersion != tc.expected.KubeVersion {
				t.Error("Not Expected kube version", "actual", apiSpec.KubeVersion, "expected", tc.expected.KubeVersion)
			}

			if apiSpec.IsOpenShift4 != tc.expected.IsOpenShift4 {
				t.Error("Not Expected cluster to be openshift4", "actual", apiSpec.IsOpenShift4, "expected", tc.expected.IsOpenShift4)
			}

			if apiSpec.Routes != tc.expected.Routes {
				t.Error("Expected api specification routes not expected")
			}

			if apiSpec.ImageStreams != tc.expected.ImageStreams {
				t.Error("Expected api specification image streams not expected")
			}
		})
	}
}

func Test_ApiCapabilitiesOpenshift3(t *testing.T) {

	res1 := metav1.APIResourceList{
		GroupVersion: "image.openshift.io/v1",
		APIResources: []metav1.APIResource{
			{Name: "imagestreams"},
		},
	}

	testCases := []struct {
		name     string
		resList  []*metav1.APIResourceList
		expected ApiServerSpec
	}{
		{
			"Openshift 3 parse version 1.11",
			[]*metav1.APIResourceList{&res1},
			ApiServerSpec{
				Version:           "1.11",
				KubeVersion:       "1.11",
				IsOpenShift4:      false,
				IsOpenShift43Plus: false,
				ImageStreams:      true,
				Routes:            true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			api := fakekube.NewSimpleClientset()
			configClient := fakeconfig.NewSimpleClientset()
			fd := api.Discovery().(*fakediscovery.FakeDiscovery)
			fd.Resources = tc.resList
			fd.FakedServerVersion = &version.Info{
				Major: "1",
				Minor: "11",
			}

			apiSpec, err := APICapabilities(context.TODO(), api, configClient)
			if err != nil {
				t.Error(err)
			}

			if apiSpec == nil {
				t.Error("Failed to return an api specification")
			}

			if apiSpec.Version != tc.expected.Version {
				t.Error("Expected api specification version not expected", "Actual", apiSpec.Version, "Expected", tc.expected.Version)
			}

			if apiSpec.KubeVersion != tc.expected.KubeVersion {
				t.Error("Not Expected kube version", "actual", apiSpec.KubeVersion, "expected", tc.expected.KubeVersion)
			}

			if apiSpec.IsOpenShift4 {
				t.Error("Expected not to be openshift cluster")
			}

			if apiSpec.ImageStreams != tc.expected.ImageStreams {
				t.Error("Expected api specification image streams not expected")
			}

		})
	}
}

func Test_ApiCapabilitiesKubernetes(t *testing.T) {

	testCases := []struct {
		name     string
		resList  []*metav1.APIResourceList
		expected ApiServerSpec
	}{
		{
			"Vanilla Kubernetes parse version 1.26",
			[]*metav1.APIResourceList{},
			ApiServerSpec{
				Version:           "1.26",
				KubeVersion:       "1.26",
				IsOpenShift4:      false,
				IsOpenShift43Plus: false,
				ImageStreams:      false,
				Routes:            false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			api := fakekube.NewSimpleClientset()
			configClient := fakeconfig.NewSimpleClientset()
			fd := api.Discovery().(*fakediscovery.FakeDiscovery)
			fd.Resources = tc.resList
			fd.FakedServerVersion = &version.Info{
				Major: "1",
				Minor: "26",
			}

			apiSpec, err := APICapabilities(context.TODO(), api, configClient)
			if err != nil {
				t.Error(err)
			}

			if apiSpec == nil {
				t.Error("Failed to return an api specification")
			}

			if apiSpec.Version != tc.expected.Version {
				t.Error("Expected api specification version not expected", "Actual", apiSpec.Version, "Expected", tc.expected.Version)
			}

			if apiSpec.KubeVersion != tc.expected.KubeVersion {
				t.Error("Not Expected kube version", "actual", apiSpec.KubeVersion, "expected", tc.expected.KubeVersion)
			}

			if apiSpec.IsOpenShift4 {
				t.Error("Expected not to be openshift cluster")
			}

			if apiSpec.ImageStreams != tc.expected.ImageStreams {
				t.Error("Expected api specification image streams not expected")
			}

		})
	}
}
