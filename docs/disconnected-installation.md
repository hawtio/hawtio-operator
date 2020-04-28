## Installation Instructions

**Step 1: Push manifests to quay.io:**

e.g. 
```bash
operator-courier push deploy/olm-catalog/hawtio-operator ${QUAY_NAMESPACE} hawtio-operator ${TAG} "${QUAY_TOKEN}
``` 

**Step 2: SSH to internal registry server and set environment variables:**



| variable | description  |
|---|---|
| APP_REGISTRY  | organization or namespace in public application registry   |
| APP_REGISTRY_ORG  |organization, namespace in local disconnected registry     |
| INTERNAL_REGISTRY  |  host:port/registry endpoint for local disconnected registry   |
| AUTH_TOKEN  | quay.io or registry token you have to Download/pull Catalogs if private repo   |

 
e.g.

```bash

export APP_REGISTRY=hawtio;export APP_REGISTRY_ORG=hawtio-org; export INTERNAL_REGISTRY="my.local.registry.domain.com:5000/images"

``` 


**Step 3: Build catalog, authentication token is not required unless there is private registry**

```bash

oc adm catalog build \
--appregistry-endpoint https://quay.io/cnr \
--appregistry-org ${APP_REGISTRY} \
--to="${INTERNAL_REGISTRY}/${APP_REGISTRY_ORG}:v1" \
--auth-token="basic XXXX"

```

**Step 4: Mirror catalog**

```bash
oc adm catalog mirror \
    ${INTERNAL_REGISTRY}/${APP_REGISTRY_ORG}:v1 \
    ${INTERNAL_REGISTRY}
```

**Step 5: Create a manifests**


```bash

oc apply -f ./${APP_REGISTRY_ORG}-manifests
oc image mirror -f ${APP_REGISTRY_ORG}-manifests/mapping.txt 

```

**Step 6: Disable All Default Sources, this step required only once**

oc patch OperatorHub cluster --type json \
    -p '[{"op": "add", "path": "/spec/disableAllDefaultSources", "value": true}]'


**Step 7: Create a Catalog Source**



```yaml

oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: my-operator-catalog
  namespace: openshift-marketplace
spec:
  sourceType: grpc
  image: ${INTERNAL_REGISTRY}/${APP_REGISTRY_ORG}:v1
  displayName: My Operator Catalog
  publisher: grpc
EOF

```

**Step 8: Verify Installation**

```bash


oc get catalogsource -n openshift-marketplace 

oc get pods -n openshift-marketplace

oc get packagemanifests -n openshift-marketplace
 ```
 
Reference Links :


https://docs.openshift.com/container-platform/4.3/operators/olm-restricted-networks.html

 