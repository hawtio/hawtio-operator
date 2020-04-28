package main

import (
	"bufio"
	"bytes"
	"fmt"
	api "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
	"github.com/heroku/docker-registry-client/registry"
	"net/http"
	"sort"

	"github.com/blang/semver"
	"github.com/hawtio/hawtio-operator/pkg/controller/hawtio/constants"
	"github.com/hawtio/hawtio-operator/tools/components"
	"github.com/hawtio/hawtio-operator/tools/util"
	"github.com/hawtio/hawtio-operator/version"
	oimagev1 "github.com/openshift/api/image/v1"
	olmversion "github.com/operator-framework/api/pkg/lib/version"
	csvv1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/tidwall/sjson"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/json"

	"os"

	"strconv"
	"strings"
	"time"
)

var (
	rh         = "Red Hat"
	maturity   = "alpha"
	maintainer = "The Hawtio team"
	csv        = csvSetting{

		Name:         "hawtio",
		DisplayName:  "Hawtio Operator",
		OperatorName: "hawtio-operator",
		CsvDir:       "hawtio-operator",
		Registry:     "registry.redhat.io",
		Context:      "fuse7-tech-preview",
		ImageName:    "fuse-console-operator",
		Tag:          constants.CurrentVersion,
	}
)

func main() {

	imageShaMap := map[string]string{}

	operatorName := csv.Name + "-operator"

	templateStruct := &csvv1.ClusterServiceVersion{}
	templateStruct.SetGroupVersionKind(csvv1.SchemeGroupVersion.WithKind("ClusterServiceVersion"))

	templateStrategySpec := csvv1.StrategyDetailsDeployment{}

	deployment := components.GetDeployment(csv.OperatorName, csv.Registry, csv.Context, csv.ImageName, csv.Tag, "Always")

	role := components.GetRole(csv.OperatorName)
	templateStrategySpec.Permissions = append(templateStrategySpec.Permissions, []csvv1.StrategyDeploymentPermissions{{ServiceAccountName: deployment.Spec.Template.Spec.ServiceAccountName, Rules: role.Rules}}...)

	clusterRole := components.GetClusterRole(csv.OperatorName)
	templateStrategySpec.ClusterPermissions = append(templateStrategySpec.ClusterPermissions, []csvv1.StrategyDeploymentPermissions{{ServiceAccountName: deployment.Spec.Template.Spec.ServiceAccountName, Rules: clusterRole.Rules}}...)

	templateStrategySpec.DeploymentSpecs = append(templateStrategySpec.DeploymentSpecs, []csvv1.StrategyDeploymentSpec{{Name: csv.OperatorName, Spec: deployment.Spec}}...)

	templateStruct.Spec.InstallStrategy.StrategySpec = templateStrategySpec
	templateStruct.Spec.InstallStrategy.StrategyName = "deployment"
	csvVersionedName := operatorName + ".v" + version.Version
	templateStruct.Name = csvVersionedName
	templateStruct.Namespace = "placeholder"
	annotdescrip := "Hawtio eases the discovery and management of Java applications deployed on OpenShift."

	var description = "Hawtio\n"
	description += "==============\n\n"
	description += "The Hawtio console eases the discovery and management of Java applications deployed on OpenShift.\n"
	description += "\n"
	description += "To secure the communication between the Hawtio console and the Jolokia agents, a client certificate must be generated and mounted into the Hawtio console pod with a secret, to be used for TLS client authentication. This client certificate must be signed using the [service signing certificate](https://docs.openshift.com/container-platform/4.2/authentication/certificates/service-serving-certificate.html) authority private key.\n\n"
	description += "Here are the steps to be performed prior to the deployment:\n\n"
	description += "1. First, retrieve the service signing certificate authority keys, by executing the following commmands as a _cluster-admin_ user:\n"
	description += "   ```sh\n"
	description += "    # The CA certificate\n"
	description += "    $ oc get secrets/signing-key -n openshift-service-ca -o " + strconv.Quote("jsonpath={.data['tls\\.crt']}") + "| base64 --decode > ca.crt\n"
	description += "    # The CA private key\n"
	description += "    $ oc get secrets/signing-key -n openshift-service-ca -o " + strconv.Quote("jsonpath={.data['tls\\.key']}") + "| base64 --decode > ca.key\n"
	description += "   ```\n\n"
	description += "2. Then, generate the client certificate, as documented in [Kubernetes certificates administration](https://kubernetes.io/docs/concepts/cluster-administration/certificates/), using either `easyrsa`, `openssl`, or `cfssl`, e.g., using `openssl`:\n"
	description += "   ```sh\n"
	description += "    # Generate the private key\n"
	description += "    $ openssl genrsa -out server.key 2048\n"
	description += "    # Write the CSR config file\n"
	description += "    $ cat <<EOT >> csr.conf\n"
	description += "    [ req ]\n"
	description += "    default_bits = 2048\n"
	description += "    prompt = no\n"
	description += "    default_md = sha256\n"
	description += "    distinguished_name = dn\n\n"
	description += "    [ dn ]\n"
	description += "    CN = hawtio-online.hawtio.svc\n\n"
	description += "    [ v3_ext ]\n"
	description += "    authorityKeyIdentifier=keyid,issuer:always\n"
	description += "    keyUsage=keyEncipherment,dataEncipherment,digitalSignature\n"
	description += "    extendedKeyUsage=serverAuth,clientAuth\n"
	description += "    EOT\n"
	description += "    # Generate the CSR\n"
	description += "    $ openssl req -new -key server.key -out server.csr -config csr.conf\n"
	description += "    # Issue the signed certificate\n"
	description += "    $ openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 10000 -extensions v3_ext -extfile csr.conf\n"
	description += "   ```\n\n"
	description += "3. Finally, you can create the secret to be mounted in the Hawtio console pod, from the generated certificate:\n"
	description += "   ```sh\n"
	description += "    $ oc create secret tls hawtio-online-tls-proxying --cert server.crt --key server.key\n"
	description += "   ```\n\n"
	description += "Note that `CN=hawtio-online.hawtio.svc` must be trusted by the Jolokia agents, for which client certification authentication is enabled. See the `clientPrincipal` parameter from the [Jolokia agent configuration options](https://jolokia.org/reference/html/agents.html#agent-jvm-config)."

	repository := "https://github.com/hawtio/hawtio-operator"
	examples := []string{"{\n        \"apiVersion\": \"hawt.io/v1alpha1\",\n        \"kind\": \"Hawtio\",\n        \"metadata\": {\n          \"name\": \"hawtio-online\"\n        },\n        \"spec\": {\n          \"replicas\": 1,\n          \"version\": \"1.8.0\"\n        }\n      }"}

	templateStruct.SetAnnotations(
		map[string]string{
			"capabilities":   "Seamless Upgrades",
			"categories":     "Monitoring",
			"certified":      "false",
			"createdAt":      time.Now().Format("2006-01-02 15:04:05"),
			"containerImage": deployment.Spec.Template.Spec.Containers[0].Image,
			"support":        "Red Hat",
			"description":    annotdescrip,
			"repository":     repository,
			"alm-examples":   "[" + strings.Join(examples, ",") + "]",
		},
	)
	/*templateStruct.SetLabels(
		map[string]string{
			"operator-" + csv.Name: "true",
		},
	)*/

	var opVersion olmversion.OperatorVersion
	templateStruct.Spec.DisplayName = csv.DisplayName
	templateStruct.Spec.Description = description
	templateStruct.Spec.Keywords = []string{"Hawtio", "Java", "Management", "Console"}
	opVersion.Version = semver.MustParse(version.Version)
	templateStruct.Spec.Version = opVersion
	templateStruct.Spec.Maturity = maturity
	templateStruct.Spec.Maintainers = []csvv1.Maintainer{{Name: maintainer, Email: "hawtio@googlegroups.com"}}
	templateStruct.Spec.Provider = csvv1.AppLink{Name: rh}
	templateStruct.Spec.Links = []csvv1.AppLink{
		{Name: "Hawtio Web site", URL: "https://hawt.io"},
	}

	templateStruct.Spec.Icon = []csvv1.Icon{
		{
			Data:      "PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0idXRmLTgiPz4KPCEtLSBHZW5lcmF0b3I6IEFkb2JlIElsbHVzdHJhdG9yIDE2LjAuNCwgU1ZHIEV4cG9ydCBQbHVnLUluIC4gU1ZHIFZlcnNpb246IDYuMDAgQnVpbGQgMCkgIC0tPgo8IURPQ1RZUEUgc3ZnIFBVQkxJQyAiLS8vVzNDLy9EVEQgU1ZHIDEuMS8vRU4iICJodHRwOi8vd3d3LnczLm9yZy9HcmFwaGljcy9TVkcvMS4xL0RURC9zdmcxMS5kdGQiPgo8c3ZnIHZlcnNpb249IjEuMSIgaWQ9IkxheWVyXzEiIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyIgeG1sbnM6eGxpbms9Imh0dHA6Ly93d3cudzMub3JnLzE5OTkveGxpbmsiIHg9IjBweCIgeT0iMHB4IgogICAgIHdpZHRoPSI2MDBweCIgaGVpZ2h0PSIxOTkuNDlweCIgdmlld0JveD0iMCAwIDYwMCAxOTkuNDkiIGVuYWJsZS1iYWNrZ3JvdW5kPSJuZXcgMCAwIDYwMCAxOTkuNDkiIHhtbDpzcGFjZT0icHJlc2VydmUiPgo8Zz4KCTxnPgoJCTxwYXRoIGZpbGw9IiNDMEMwQzAiIGQ9Ik0xODMuMzA0LDc1LjUwMWMtMTIuOTQ5LDAtMjAsNS4zOTgtMjcuMDE5LDEwLjQ2OVYzNi42NjloLTIyLjI5OXY5NS40N2wyMi4zODMsMjIuMzh2LTUzLjY4MwoJCQljNi41OTMtNS4wMzQsMTEuNTExLTcuMzYxLDIwLjUxNy03LjM2MWM1LjY3MywwLDkuMzc1LDMuMDQ1LDkuMzc1LDExLjQ4NXY1Ni44NjJoMjIuMzgydi01Ni41MQoJCQlDMjA4LjY0Myw4OS4xMzUsMjAzLjA2LDc1LjUwMSwxODMuMzA0LDc1LjUwMXoiLz4KICAgICAgICA8cGF0aCBmaWxsPSIjQzBDMEMwIiBkPSJNMjE1LjQyMiwxMzIuMTkyYzAtMTIuMzUsNi42NzYtMjMuMjc5LDI0Ljg3My0yMy4yNzljMCwwLDI0LjkyOSwwLjE1NiwyNC45MjksMGMwLDAsMC4xNjUtNS4yODIsMC01LjQ0MQoJCQljMC05LjA0OC00LjIyOS0xMC40NjktMTEuMzQ3LTEwLjQ2OWMtNy4zNzgsMC0yOC42NjgsMS4zMjQtMzUuNjQ4LDEuODI3VjgxLjAxN2MxMS4xNzktNC4zNDIsMjIuODUyLTUuNjA2LDM4LjY4LTUuNjc5CgkJCWMxOC4yOC0wLjA3NSwzMC43MDIsNi4yNDEsMzAuNzAyLDI3LjI1NXY1OS4yM2gtMTcuNjcybC00LjcxNC05LjM1Yy0wLjkzMSwxLjg0LTEyLjg0MywxMC41NzItMjUuODExLDEwLjMwOAoJCQljLTE2LjMzNC0wLjQzNy0yMy45OTEtMTEuNTYtMjMuOTkxLTIyLjU5OVYxMzIuMTkyeiBNMjQ1Ljc0MiwxNDQuNzk1YzkuMzYyLDAsMTkuNDgyLTYuNTY4LDE5LjQ4Mi02LjU2OFYxMjIuNjdsLTIwLjUzMiwxLjU4MgoJCQljLTUuOTEsMC41NTMtNi44ODQsNC45MjMtNi44ODQsOC42MzV2NC4xMjVDMjM3LjgwOCwxNDQuMDcsMjQxLjYyNSwxNDQuNzk1LDI0NS43NDIsMTQ0Ljc5NXoiLz4KICAgICAgICA8cGF0aCBmaWxsPSIjQzBDMEMwIiBkPSJNMzExLjk0Miw3Ni45MzdsMTcuMDA4LDU4LjY2N2wxNS4yMDctNTguNjY3aDIyLjI4bC01LjkwNywyMi44NjdsMTQuMDYyLDM1LjhsMTYuNzQ2LTU4LjY2N2gyNC4wMjkKCQkJbC0yNy40MzMsODQuODg2aC0yNS4wMjVsLTEyLjcwMy0zMC43NTJsLTkuNjAyLDMwLjc1MkgzMTUuMTVsLTI3LjE0OS04NC44ODZIMzExLjk0MnoiLz4KICAgICAgICA8cGF0aCBmaWxsPSIjQzBDMEMwIiBkPSJNNDIwLjA2Miw4MS40MTJsMTMuNzA5LTQuNjM0bDMuNzU4LTIzLjY2MmgxOC42MjV2MjMuNjYyaDE4Ljk3N3YxNy4xMDRoLTE4Ljk3N3YzNC4wNDkKCQkJYzAsMTIuNTAzLDIuODgyLDE1LjE3Myw3LjI2NywxNi44MDFjMCwwLDkuNTkyLDMuMzU3LDEwLjcwMywzLjM1N3YxMy43NDloLTE5LjUxM2MtMTIuNTY3LDAtMjAuODQtNy44NDktMjAuODQtMzAuMzg5VjkzLjg4MgoJCQloLTEzLjcwOVY4MS40MTJ6Ii8+CiAgICAgICAgPHBhdGggZmlsbD0iI0MwQzBDMCIgZD0iTTQ4NC42MjMsNTAuMTYyYzAtMi4zOTgsMS4yNzctNCwzLjgzOS00aDE2Ljc4NmMyLjM5NSwwLDMuNTg0LDEuNzY1LDMuNTg0LDR2MTQuMTcxCgkJCWMwLDIuMzk0LTEuMzQ5LDMuNjc4LTMuNTg0LDMuNjc4aC0xNi43ODZjLTIuMjM5LDAtMy44MzktMS40MzYtMy44MzktMy42NzhWNTAuMTYyeiBNNDg1LjQzMSw3Ni45MzdoMjIuMzc2djg0Ljg4NmgtMjIuMzc2CgkJCVY3Ni45Mzd6Ii8+CiAgICAgICAgPHBhdGggZmlsbD0iI0MwQzBDMCIgZD0iTTU1OS42MzcsNzYuMTQyYzI5LjIzMywwLDM4LjcyOSwxMC43MDgsMzguNzI5LDQ0LjQzNWMwLDMxLjQ5Ni05LjA1NSw0Mi4wNDQtMzguNzI5LDQyLjA0NAoJCQljLTI5LjMxMywwLTM4LjcyNi0xMS42Ny0zOC43MjYtNDIuMDQ0QzUyMC45MTEsODUuODkzLDUzMC45NDgsNzYuMTQyLDU1OS42MzcsNzYuMTQyeiBNNTU5LjYzNywxNDQuNjM5CgkJCWMxMi42MTEsMCwxNi4zNS0xLjcxMSwxNi4zNS0yNC4wNjJjMC0yMy43NzItMy4wMS0yNi40NTMtMTYuMzUtMjYuNDUzYy0xMy4yNDMsMC0xNi4zNDIsMi42OTMtMTYuMzQyLDI2LjQ1MwoJCQlDNTQzLjI5NSwxNDIuNTQ0LDU0Ny43NDQsMTQ0LjYzOSw1NTkuNjM3LDE0NC42Mzl6Ii8+Cgk8L2c+CiAgICA8cGF0aCBmaWxsPSIjQzBDMEMwIiBkPSJNMjAuNTc1LDEzMi41MTRjMTEuNzM2LDExLjc0MSwyNy4zNSwxOC4yMTQsNDMuOTUzLDE4LjIxNGMxMS40MDksMCwyMi42MjUtMy4xODcsMzIuNDQxLTkuMjExbDIuMDc3LTEuMjcKCQlsMjEuOTIsMjEuOTI5aDM0LjMzOGwtMzkuMDk5LTM5LjFsMS4yNzQtMi4wNjdjMTUuMDkxLTI0LjU5MywxMS4zOS01Ni4wMDctOC45OTctNzYuMzkzCgkJQzk2Ljc0MiwzMi44NzYsODEuMTI5LDI2LjQwOSw2NC41MjgsMjYuNDA5Yy0xNi42MDQsMC0zMi4yMDYsNi40NjgtNDMuOTQ3LDE4LjIwN0M4LjgzOCw1Ni4zNTgsMi4zNjUsNzEuOTcsMi4zNjUsODguNTY5CgkJQzIuMzY1LDEwNS4xNjcsOC44MzIsMTIwLjc3NSwyMC41NzUsMTMyLjUxNHogTTM3Ljc0NSw2MS43ODVjNy4xNDgtNy4xNTYsMTYuNjYzLTExLjA5NSwyNi43ODMtMTEuMDk1CgkJYzEwLjExNywwLDE5LjYyOSwzLjkzOSwyNi43OCwxMS4wOTVjNy4xNTUsNy4xNTgsMTEuMDk1LDE2LjY2OCwxMS4wOTUsMjYuNzg1YzAsMTAuMTIxLTMuOTQsMTkuNjMtMTEuMDk4LDI2Ljc4MwoJCWMtNy4xNDYsNy4xNTMtMTYuNjYsMTEuMDkyLTI2Ljc3NywxMS4wOTJjLTEwLjEyLDAtMTkuNjM0LTMuOTM4LTI2Ljc4Ni0xMS4wOTJjLTcuMTU0LTcuMTYxLTExLjA5NS0xNi42NjctMTEuMDk1LTI2Ljc4MwoJCUMyNi42NDcsNzguNDUzLDMwLjU4OCw2OC45NDIsMzcuNzQ1LDYxLjc4NXoiLz4KICAgIDxnPgoJCTxwYXRoIGZpbGw9IiNFQzc5MjkiIGQ9Ik04MC4xOTcsOTIuOTI2djAuMDkzYzAuNDY5LDAuODU0LDAuNzMzLDEuODI0LDAuNzQ2LDIuODYyYzAsMy4yNjktMi42NTUsNS45MjItNS45MjUsNS45MjIKCQkJYy0zLjI3LDAtNS45MTgtMi42NTMtNS45MTgtNS45MjJjMC0xLjQ0MiwwLjUxNS0yLjc2NywxLjM3OC0zLjc5NmMwLDAsMTQuNDY0LTIwLjEzNy01Ljc4NS0zMy40OThjMCwwLDUuODc1LDE1Ljg2LTguMTI5LDIyLjUyNwoJCQljMCwwLTMuODY2LDIuNzI5LDAuNC02LjMzNWMwLDAtMTEuODksNi42ODMtMTEuODksMjIuODE0YzAsNy4yOTEsMy43NzYsMTMuNjkxLDkuNDc0LDE3LjM3NgoJCQljLTAuMTc3LTAuNjU4LTAuMjg5LTEuNDExLTAuMjg5LTIuMjk5YzAuMDA2LTYuNDY5LDUuODA0LTE0LjAzMSw3LjU4Ni0xNC4wMTljMCwwLDAuNDUsNy42NjQsMi41MDYsMTEuMjc3CgkJCWMxLjYyLDMuMDU4LDAuNjM2LDYuNDQ0LTEuNTEzLDguMTM4YzAuOTYyLDAuMTM5LDEuOTQ0LDAuMjM0LDIuOTQ3LDAuMjM0YzUuNDY4LDAsMTAuNDQ2LTIuMTIyLDE0LjE0Ny01LjU5MQoJCQljMCwwLTAuMDA2LTAuMDE4LTAuMDA2LTAuMDIzQzg2LjgzMywxMDMuNjA1LDgyLjgyOCw5Ni4yMjQsODAuMTk3LDkyLjkyNnoiLz4KCTwvZz4KPC9nPgo8L3N2Zz4=",
			MediaType: "image/svg+xml",
		},
	}
	tLabels := map[string]string{
		//"alm-owner-" + csv.Name: operatorName,
		"name": operatorName,
	}
	templateStruct.Spec.Labels = tLabels
	templateStruct.Spec.Selector = &metav1.LabelSelector{MatchLabels: tLabels}
	templateStruct.Spec.InstallModes = []csvv1.InstallMode{
		{Type: csvv1.InstallModeTypeOwnNamespace, Supported: true},
		{Type: csvv1.InstallModeTypeSingleNamespace, Supported: true},
		{Type: csvv1.InstallModeTypeMultiNamespace, Supported: false},
		{Type: csvv1.InstallModeTypeAllNamespaces, Supported: false},
	}
	templateStruct.Spec.Replaces = operatorName + ".v" + version.PriorVersion
	templateStruct.Spec.CustomResourceDefinitions.Owned = []csvv1.CRDDescription{
		{
			Version:     api.SchemeGroupVersion.Version,
			Kind:        "Hawtio",
			DisplayName: "Hawtio",
			Description: "A Hawtio Console",
			Name:        "hawtios." + api.SchemeGroupVersion.Group,
			/*Resources: []csvv1.APIResourceReference{

				{
					Kind:    "StatefulSet",
					Version: appsv1.SchemeGroupVersion.String(),
				},
				{
					Kind:    "Secret",
					Version: corev1.SchemeGroupVersion.String(),
				},
				{
					Kind:    "Service",
					Version: corev1.SchemeGroupVersion.String(),
				},

				{
					Kind:    "ImageStream",
					Version: oimagev1.SchemeGroupVersion.String(),
				},
			},*/
			SpecDescriptors: []csvv1.SpecDescriptor{

				{
					Description:  "The number of Hawtio pods to deploy",
					DisplayName:  "Replicas",
					Path:         "replicas",
					XDescriptors: []string{"urn:alm:descriptor:com.tectonic.ui:fieldGroup:Deployment", "urn:alm:descriptor:com.tectonic.ui:podCount"},
				},
				{
					Description:  "The version used for the Hawtio",
					DisplayName:  "Version",
					Path:         "version",
					XDescriptors: []string{"urn:alm:descriptor:com.tectonic.ui:fieldGroup:Deployment", "urn:alm:descriptor:com.tectonic.ui:text"},
				},
			},
		},
	}

	opMajor, opMinor, opMicro := components.MajorMinorMicro(version.Version)
	csvFile := "deploy/olm-catalog" + "/hawtio-operator/" + opMajor + "." + opMinor + "." + opMicro + "/" + csvVersionedName + ".clusterserviceversion.yaml"

	imageName, _, _ := components.GetImage(deployment.Spec.Template.Spec.Containers[0].Image)
	relatedImages := []image{}

	templateStruct.Annotations["certified"] = "false"
	deployFile := "deploy/operator.yaml"
	createFile(deployFile, deployment)
	roleFile := "deploy/role.yaml"
	createFile(roleFile, role)
	clusterRoleFile := "deploy/cluster_role.yaml"
	createFile(clusterRoleFile, clusterRole)

	relatedImages = append(relatedImages, image{Name: imageName, Image: deployment.Spec.Template.Spec.Containers[0].Image})

	imageRef := constants.ImageRef{
		TypeMeta: metav1.TypeMeta{
			APIVersion: oimagev1.SchemeGroupVersion.String(),
			Kind:       "ImageStream",
		},
		Spec: constants.ImageRefSpec{
			Tags: []constants.ImageRefTag{
				{
					// Needs to match the component name for upstream and downstream.
					Name: "fuse7-tech-preview/fuse-console-operator",
					From: &corev1.ObjectReference{
						// Needs to match the image that is in your CSV that you want to replace.
						Name: deployment.Spec.Template.Spec.Containers[0].Image,
						Kind: "DockerImage",
					},
				},
			},
		},
	}

	sort.Sort(sort.Reverse(sort.StringSlice(constants.SupportedVersions)))
	for _, imageVersion := range constants.SupportedVersions {
		if versionConstants, found := constants.VersionConstants[imageVersion]; found {

			imageRef.Spec.Tags = append(imageRef.Spec.Tags, constants.ImageRefTag{
				Name: constants.HawtioImageTagComponent,
				From: &corev1.ObjectReference{
					Name: versionConstants.HawtioImageURL,
					Kind: "DockerImage",
				},
			})

			relatedImages = append(relatedImages, getRelatedImage(versionConstants.HawtioImageURL))
		}
	}

	if GetBoolEnv("DIGESTS") {

		for _, tagRef := range imageRef.Spec.Tags {

			if _, ok := imageShaMap[tagRef.From.Name]; !ok {
				imageShaMap[tagRef.From.Name] = ""
				imageName, imageTag, imageContext := components.GetImage(tagRef.From.Name)
				repo := imageContext + "/" + imageName

				digests, err := RetriveFromRedHatIO(repo, imageTag)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
				if len(digests) > 1 {
					imageShaMap[tagRef.From.Name] = strings.ReplaceAll(tagRef.From.Name, ":"+imageTag, "@"+digests[len(digests)-1])
				}
			}
		}
	}

	//not sure if we required mage-references file in the future So comment out for now.

	//imageFile := "deploy/olm-catalog/" + csv.CsvDir + "/" + opMajor + "." + opMinor + "." + opMicro + "/" + "image-references"
	//createFile(imageFile, imageRef)

	var templateInterface interface{}
	if len(relatedImages) > 0 {
		templateJSON, err := json.Marshal(templateStruct)
		if err != nil {
			fmt.Println(err)
		}
		result, err := sjson.SetBytes(templateJSON, "spec.relatedImages", relatedImages)
		if err != nil {
			fmt.Println(err)

		}
		if err = json.Unmarshal(result, &templateInterface); err != nil {
			fmt.Println(err)
		}
	} else {
		templateInterface = templateStruct
	}

	// find and replace images with SHAs where necessary
	templateByte, err := json.Marshal(templateInterface)
	if err != nil {
		fmt.Println(err)
	}
	for from, to := range imageShaMap {
		if to != "" {
			templateByte = bytes.ReplaceAll(templateByte, []byte(from), []byte(to))
		}
	}
	if err = json.Unmarshal(templateByte, &templateInterface); err != nil {
		fmt.Println(err)
	}
	createFile(csvFile, &templateInterface)
	packageFile := "deploy/olm-catalog/" + csv.CsvDir + "/" + operatorName + ".package.yaml"
	p, err := os.Create(packageFile)
	defer p.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
	pwr := bufio.NewWriter(p)
	pwr.WriteString("#! package-manifest: " + csvFile + "\n")
	packagedata := packageStruct{
		PackageName: operatorName,
		Channels: []channel{
			{
				Name:       maturity,
				CurrentCSV: operatorName + ".v" + version.PriorVersion,
			},
			{
				Name:       maturity + "-offline",
				CurrentCSV: csvVersionedName,
			},
		},
		DefaultChannel: maturity,
	}
	util.MarshallObject(packagedata, pwr)
	pwr.Flush()
}

func RetriveFromRedHatIO(image string, imageTag string) ([]string, error) {

	url := "https://" + constants.RedHatImageRegistry

	username := "" // anonymous
	password := "" // anonymous

	if userToken := strings.Split(os.Getenv("REDHATIO_TOKEN"), ":"); len(userToken) > 1 {
		username = userToken[0]
		password = userToken[1]
	}
	hub, err := registry.New(url, username, password)
	digests := []string{}
	if err != nil {
		fmt.Println(err)
	} else {
		tags, err := hub.Tags(image)
		if err != nil {
			fmt.Println(err)
		}
		// do not follow redirects - this is critical so we can get the registry digest from Location in redirect response
		hub.Client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		if _, exists := find(tags, imageTag); exists {
			req, err := http.NewRequest("GET", url+"/v2/"+image+"/manifests/"+imageTag, nil)
			if err != nil {
				fmt.Println(err)
			}
			req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
			resp, err := hub.Client.Do(req)
			if err != nil {
				fmt.Println(err)
			}
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp.StatusCode == 302 || resp.StatusCode == 301 {
				digestURL, err := resp.Location()
				if err != nil {
					fmt.Println(err)

				}

				if digestURL != nil {
					if url := strings.Split(digestURL.EscapedPath(), "/"); len(url) > 1 {
						digests = url

						return url, err
					}
				}
			}
		}

	}
	return digests, err
}

func find(slice []string, val string) (int, bool) {
	for i, item := range slice {
		if item == val {
			return i, true
		}
	}
	return -1, false
}

type csvSetting struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	OperatorName string `json:"operatorName"`
	CsvDir       string `json:"csvDir"`
	Registry     string `json:"repository"`
	Context      string `json:"context"`
	ImageName    string `json:"imageName"`
	Tag          string `json:"tag"`
}
type channel struct {
	Name       string `json:"name"`
	CurrentCSV string `json:"currentCSV"`
}
type packageStruct struct {
	PackageName    string    `json:"packageName"`
	Channels       []channel `json:"channels"`
	DefaultChannel string    `json:"defaultChannel"`
}
type image struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

func getRelatedImage(imageURL string) image {
	imageName, _, _ := components.GetImage(imageURL)
	return image{
		Name:  imageName,
		Image: imageURL,
	}
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func createFile(filepath string, obj interface{}) {
	f, err := os.Create(filepath)
	defer f.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
	writer := bufio.NewWriter(f)
	util.MarshallObject(obj, writer)
	writer.Flush()
}

func GetBoolEnv(key string) bool {
	val := GetEnv(key, "false")
	ret, err := strconv.ParseBool(val)
	if err != nil {
		return false
	}
	return ret
}

func GetEnv(key, fallback string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = fallback
	}
	return value
}
