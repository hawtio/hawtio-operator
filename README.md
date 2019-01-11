# Hawtio Operator

A Kubernetes operator based on the Operator SDK that installs and maintains [Hawtio Online](https://github.com/hawtio/hawtio-online) on a cluster.

## Development

```console
$ make install run

INFO[0000] Running the operator locally.
```

## Test

```console
# Create Hawtio
$ kubectl apply -f deploy/crds/hawtio_v1alpha1_hawtio_cr.yaml
hawtio.hawt.io/example-hawtio created

# Get Hawtio info
$ kubectl get hawtio
NAME             AGE   URL                                           IMAGE
example-hawtio   16s   https://example-hawtio.192.168.64.38.nip.io   docker.io/hawtio/online:latest

# Scale Hawtio
$ kubectl scale hawtio example-hawtio --replicas=3
hawtio.hawt.io/example-hawtio scaled

# Edit Hawtio resource
$ kubectl patch hawtio example-hawtio --type='merge' -p '{"spec":{"routeHostName":"hawtio.192.168.64.38.nip.io"}}'
hawtio.hawt.io/example-hawtio patched
# Check the status has updated accordingly
$ kubectl get hawtio
NAME             AGE   URL                                   IMAGE
example-hawtio   1m    https://hawtio.192.168.64.38.nip.io   docker.io/hawtio/online:latest

# Edit Hawtio config
$ kubectl edit configmap example-hawtio
configmap/example-hawtio edited
# Watch rollout deployment triggered by config change
$ oc rollout status dc/example-hawtio
Waiting for rollout to finish: 0 out of 1 new replicas have been updated...
Waiting for rollout to finish: 0 out of 1 new replicas have been updated...
Waiting for rollout to finish: 1 old replicas are pending termination...
Waiting for rollout to finish: 1 old replicas are pending termination...
Waiting for latest deployment config spec to be observed by the controller loop...
replication controller "example-hawtio-2" successfully rolled out

# Delete Hawtio
$ kubectl delete hawtio example-hawtio
hawtio.hawt.io "example-hawtio" deleted
```
