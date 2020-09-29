# Hawtio Operator

A [Kubernetes](https://kubernetes.io) operator, based on the [Operator SDK](https://sdk.operatorframework.io), that operates [Hawtio Online](https://github.com/hawtio/hawtio-online).

## Resources

The `Hawtio` CRD defines the resource the operator uses to configure a Hawtio Online operand, e.g.:

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
  routeHostName: hawtio-online.hawt.io
  # The version (default 'latest')
  version: latest
  # The compute resources required by the deployment
  resources:
    limits:
      cpu: "1"
      memory: 100Mi
    requests:
      cpu: 200m
      memory: 32Mi
```

## Features

The operator covers the following cases:

* Creation
  * Create Deployment, ConfigMap, Service and Route resources
  * Create a service account as OAuth client in `namespace` deployment
  * Create an OAuth client in `cluster` deployment
  * Create a Secret containing a client certificate used to authenticate to Jolokia endpoints
* Update
  * Reconcile the Deployment container image from the `version` field
  * Reconcile the Route host from the `routeHostName` field
  * Support emptying the `routeHostName` field (recreate the Route to re-generate the host)
  * Reconcile the `replicas` field into the Deployment
  * Reconcile the `resources` field into the Deployment
  * Support changing deployment type from / to `namespace` or `cluster`
  * Remove previous Route host from the OAuth client in `cluster` deployment
  * Trigger a rollout deployment on ConfigMap changes
* Deletion
  * Remove the Deployment, ConfigMap, Service and Route resources
  * Remove the service account as OAuth client in `namespace` deployment
  * Remove the route URL from the OAuth client authorized redirect URIs in `cluster` deployment
  * Remove the generated client certificate Secret

## Deploy

To create the required resources by the operator (e.g. CRD, service account, roles, role binding, deployment, ...), run the following command:

```console
$ make deploy
```

The above command must be executed on behalf of a privileged user, as the creation of the custom resource definition and the cluster role requires _cluster-admin_ permission.

## Test

To create and operate a Hawtio resource, you can run the following commands:

```console
# Create Hawtio
$ kubectl apply -f deploy/crs/hawtio_v1alpha1_hawtio_cr.yaml
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
