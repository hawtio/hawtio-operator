package util

import (
	"fmt"
	"io/ioutil"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	serializerjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/yaml"
	cgoscheme "k8s.io/client-go/kubernetes/scheme"

	apps "github.com/openshift/api/apps/v1"
	authorization "github.com/openshift/api/authorization/v1"
	build "github.com/openshift/api/build/v1"
	image "github.com/openshift/api/image/v1"
	oauth "github.com/openshift/api/oauth/v1"
	route "github.com/openshift/api/route/v1"
	template "github.com/openshift/api/template/v1"
)

var (
	scheme         = runtime.NewScheme()
	codecs         = serializer.NewCodecFactory(scheme)
	decoderFunc    = decoder
	jsonSerializer = serializerjson.NewSerializerWithOptions(serializerjson.DefaultMetaFactory, scheme, scheme, serializerjson.SerializerOptions{Yaml: false, Pretty: false, Strict: false})
)

func init() {
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Version: "v1"})
	cgoscheme.AddToScheme(scheme)

	//add OpenShift types
	apps.Install(scheme)
	authorization.Install(scheme)
	build.Install(scheme)
	image.Install(scheme)
	oauth.Install(scheme)
	route.Install(scheme)
	template.Install(scheme)

	//legacy OpenShift types
	apps.DeprecatedInstallWithoutGroup(scheme)
	authorization.DeprecatedInstallWithoutGroup(scheme)
	build.DeprecatedInstallWithoutGroup(scheme)
	image.DeprecatedInstallWithoutGroup(scheme)
	oauth.DeprecatedInstallWithoutGroup(scheme)
	route.DeprecatedInstallWithoutGroup(scheme)
	template.DeprecatedInstallWithoutGroup(scheme)
}

func decoder(gv schema.GroupVersion, codecs serializer.CodecFactory) runtime.Decoder {
	codec := codecs.UniversalDecoder(gv)
	return codec
}

func Encode(obj runtime.Object) ([]byte, error) {
	return runtime.Encode(jsonSerializer, obj)
}

func RuntimeObjectFromUnstructured(u *unstructured.Unstructured) (runtime.Object, error) {
	gvk := u.GroupVersionKind()
	decoder := decoderFunc(gvk.GroupVersion(), codecs)

	b, err := u.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("error running MarshalJSON on unstructured object: %v", err)
	}

	ro, _, err := decoder.Decode(b, &gvk, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decode json data with gvk(%v): %v", gvk.String(), err)
	}

	return ro, nil
}

func LoadKubernetesResource(jsonData []byte) (runtime.Object, error) {
	u := unstructured.Unstructured{}

	err := u.UnmarshalJSON(jsonData)
	if err != nil {
		return nil, err
	}

	return RuntimeObjectFromUnstructured(&u)
}

func jsonIfYaml(source []byte, filename string) ([]byte, error) {
	if strings.HasSuffix(filename, ".yaml") || strings.HasSuffix(filename, ".yml") {
		return yaml.ToJSON(source)
	}

	return source, nil
}
func LoadConfigFromFile(path string) ([]byte, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	data, err = jsonIfYaml(data, path)
	if err != nil {
		return nil, err
	}

	return data, err
}
