# How to release Hawtio operator image

## 1. Create OLM manifest for the release version

Create a new version directory under `deploy/olm-catalog/`.

**TBD**

## 2. Tag version to the main branch

```console
make build # Make sure it builds
git tag x.x.x
git push origin main --tags
```

### 3. Build image locally and push to Docker Hub

Make sure you have logged in to docker.io:
```console
docker login
```

Build image and push it to Docker Hub:

```console
TAG=x.x.x make image
docker push hawtio/operator:x.x.x
```

> :information_source: For `podman` users, it is important to do `export BUILDAH_FORMAT=docker` before `make image` so that the built image is based on Docker manifest type.
