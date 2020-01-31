# Hawtio Operator

A Kubernetes operator based on the Operator SDK that installs and maintains [Hawtio Online](https://github.com/hawtio/hawtio-online) on a cluster.

## Custom Resource

```yaml
apiVersion: hawt.io/v1alpha1
kind: Hawtio
metadata:
  name: hawtio-online
spec:
  # The deployment type, either "cluster" or "namespace":
  # - cluster: Hawtio is capable of discovering and managing
  #   applications across all namespaces the authenticated user
  #   has access to.
  # - namespace: Hawtio is capable of discovering and managing
  #   applications within the deployment namespace.
  type: cluster
  # The number of desired replicas
  replicas: 1
  # The edge host name of the route that exposes the Hawtio service
  # externally. If not specified, it is automatically generated and
  # is of the form:
  #   <name>[-<namespace>].<suffix>
  # where <suffix> is the default routing sub-domain as configured for
  # the cluster.
  # Note that the operator will recreate the route if the field is emptied,
  # so that the host is re-generated.
  routeHostName: hawtio-online.192.168.64.38.nip.io
  # The version (default 'latest')
  version: latest
```

## Features

The operator covers the following cases:

* Creation
  * Create image stream, deployment config, config map, service and route resources
  * Create a service account as OAuth client in `namespace` deployment
  * Create an OAuth client in `cluster` deployment
* Update
  * Reconcile the image stream tag and deployment trigger from the `version` field
  * Reconcile the route host from the `routeHostName` field
  * Support emptying the `routeHostName` field (recreate the route to re-generate the host)
  * Reconcile the `replicas` count into the deployment config
  * Reconcile the `replicas` count from deployment config changes
  * Support changing deployment type from / to `namespace` or `cluster`
  * Remove previous route host from OAuth client in `cluster` deployment
  * Trigger a rollout deployment on config map change
* Deletion
  * Remove image stream, deployment config, config map, service and route resources
  * Remove the service account as OAuth client in `namespace` deployment
  * Remove the route URL from the OAuth client authorized redirect URIs in `cluster` deployment

## Install

To create the required resources by the operator (e.g. custom resource definition, service account, roles, role binding, ...), run the following command:

```console
$ make install
```

The above command must be executed on behalf of a privileged user, as the creation of the custom resource definition and the cluster role requires _cluster-admin_ permission.

Note that the cluster role creation is optional in case you plan to only deploy Hawtio custom resources with `namespace` deployment type.

## Deploy

To create the operator deployment, run the following command:

```console
$ make deploy

kubectl apply -f deploy/operator.yaml -n hawtio
deployment.apps/hawtio-operator created
```

## Test

To create and operate a Hawtio console resource, you can run the following commands:

```console
# Create Hawtio
$ kubectl apply -f deploy/crds/hawtio_v1alpha1_hawtio_cr.yaml
hawtio.hawt.io/hawtio-online created

# Get Hawtio info
$ kubectl get hawtio
NAME             AGE   URL                                           IMAGE
hawtio-online   16s   https://hawtio-online.192.168.64.38.nip.io   docker.io/hawtio/online:latest

# Scale Hawtio
$ kubectl scale hawtio hawtio-online --replicas=3
hawtio.hawt.io/hawtio-online scaled

# Edit Hawtio resource
$ kubectl patch hawtio hawtio-online --type='merge' -p '{"spec":{"routeHostName":"hawtio.192.168.64.38.nip.io"}}'
hawtio.hawt.io/hawtio-online patched
# Check the status has updated accordingly
$ kubectl get hawtio
NAME             AGE   URL                                   IMAGE
hawtio-online   1m    https://hawtio.192.168.64.38.nip.io   docker.io/hawtio/online:latest

# Edit Hawtio config
$ kubectl edit configmap hawtio-online
configmap/hawtio-online edited
# Watch rollout deployment triggered by config change
$ kubectl rollout status deployment.v1.apps/hawtio-online
Waiting for deployment "hawtio-online" rollout to finish: 1 out of 3 new replicas have been updated...
Waiting for deployment "hawtio-online" rollout to finish: 2 out of 3 new replicas have been updated...
Waiting for deployment "hawtio-online" rollout to finish: 1 old replicas are pending termination...
deployment "hawtio-online" successfully rolled out

# Change the Hawtio version
$ kubectl patch hawtio hawtio-online --type='merge' -p '{"spec":{"version":"1.7.1"}}'
hawtio.hawt.io/hawtio-online patched
# Check the status has updated accordingly
$ kubectl get hawtio
NAME             AGE   URL                                   IMAGE
hawtio-online   1m    https://hawtio.192.168.64.38.nip.io   docker.io/hawtio/online:1.7.1
# Watch rollout deployment triggered by version change
$ kubectl rollout status deployment.v1.apps/hawtio-online
...
deployment "hawtio-online" successfully rolled out

# Delete Hawtio
$ kubectl delete hawtio hawtio-online
hawtio.hawt.io "hawtio-online" deleted
```

## Development

To run the operator locally in order to speed up development cycle, you can run the following command:

```console
$ make run

INFO[0000] Running the operator locally.
```
