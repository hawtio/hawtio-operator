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

# Edit Hawtio config
$ kubectl edit configmap example-hawtio
configmap/example-hawtio edited

# Delete Hawtio
$ kubectl delete hawtio example-hawtio
hawtio.hawt.io "example-hawtio" deleted
```
