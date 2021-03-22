# Hawtio Operator Upgrading Guide

This document describes important changes between hawtio-operator versions and instructions on how to upgrade your operator installation.

- [hawtio-operator 0.4.0](#hawtio-operator-0.4.0)

## hawtio-operator 0.4.0

### `spec.rbac.enabled` field is removed from CRD

From 0.4.0 RBAC support is always enabled, so the field to toggle the feature is removed from the CRD ([#48](https://github.com/hawtio/hawtio-operator/pull/48)). If your `Hawtio` custom resource contains the following field (whether it is `true` or `false`), remove it from the yaml definition.

```yaml
spec:
    rbac:
        enabled: true
```

**NOTE:** From 0.4.0 and onwards, if you don't provide `spec.rbac.configMap` RBAC is still activated with the default [ACL](https://github.com/hawtio/hawtio-online/blob/master/docker/ACL.yaml).
