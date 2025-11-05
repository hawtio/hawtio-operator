# How to release Hawtio operator image

## 1. Ensure the version in the Makefile is correct.

## 2. Create OLM manifest for the release version

Create a new version directory under `deploy/olm-catalog/`.

**TBD**

## 3. Tag version to the main branch

```console
make build # Make sure it builds
git tag x.x.x
git push origin main --tags
```

### 4. Build image locally and push to Docker Hub

Make sure you have access to quay.io:

Build image and push it to quay.io:

```console
make publish-image
```

> :information_source: For `podman` users, it is important to do `export BUILDAH_FORMAT=docker` before `make publish-image` so that the built image is based on Docker manifest type.
