module github.com/hawtio/hawtio-operator

go 1.13

require (
	github.com/Masterminds/semver v1.5.0
	github.com/RHsyseng/operator-utils v0.0.0-20200506183821-e3b4a2ba9c30
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/gobuffalo/packr/v2 v2.7.1
	github.com/openshift/api v0.0.0-20200205145930-e9d93e317dd1
	// OpenShift 4.4 (via replace statement)
	github.com/openshift/client-go v3.9.0+incompatible
	github.com/operator-framework/operator-lib v0.1.0
	github.com/prometheus/client_golang v1.5.1 // indirect
	github.com/sirupsen/logrus v1.5.0 // indirect
	github.com/stretchr/testify v1.5.1
	golang.org/x/net v0.0.0-20200625001655-4c5254603344 // indirect
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae // indirect
	golang.org/x/tools v0.0.0-20200403190813-44a64ad78b9b // indirect
	google.golang.org/protobuf v1.25.0 // indirect
	k8s.io/api v0.18.6
	k8s.io/apiextensions-apiserver v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.6.2
)

replace k8s.io/client-go => k8s.io/client-go v0.18.6

replace github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20200116152001-92a2713fa240
