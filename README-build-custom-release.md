# Custom kube-scheduler image build

Guide to building a **kube-scheduler** image with the plugins in this directory (including `FlavourClusterWide` and those registered in `cmd/scheduler/main.go`), aligned with a **specific Kubernetes release** (for example **1.32**).

## Compatibility matrix

The scheduler-plugins **minor** version must match the cluster `k8s.io/*` dependency **minor**:

| Kubernetes (cluster) | scheduler-plugins | Git branch         | Upstream reference tag |
|----------------------|-------------------|--------------------|------------------------|
| v1.33.x              | v0.33.x           | `release-1.33`     | `v0.33.5`              |
| v1.32.x              | v0.32.x           | `release-1.32`     | `v0.32.7`              |
| v1.31.x              | v0.31.x           | `release-1.31`     | `v0.31.8`              |

See the full table in [README.md — Compatibility Matrix](README.md#compatibility-matrix).

> **Important:** the scheduler binary must be compiled against the same Kubernetes **minor** as the cluster control plane. A mismatch (e.g. scheduler `v0.33` on a `1.32` cluster) can cause API or version validation errors.

## Prerequisites

- **Go:** version from `go.mod` on the chosen branch (e.g. `go 1.23.0` on `release-1.32`, `go 1.24.0` on `release-1.33`).
- **Container engine** with **buildx** support: Docker, Podman, or nerdctl.
- **Push** access to the target registry (Quay, internal GCR, etc.) if the image is not local-only.
- Source code of this repository (fork with custom plugins already integrated).

## 1. Choose the Kubernetes release

Define the cluster version, for example **1.32.7**:

- **K8s minor:** `1.32` → branch `release-1.32`, dependencies `k8s.io/* v0.32.7`.
- **Patch:** use the patch closest to the control plane (e.g. `1.32.7` when the cluster is `v1.32.7`).

## 2. Check out the correct branch

From the root of this directory (`scheduler-plugins`):

```bash
# Example: build for Kubernetes 1.32
git fetch origin
git checkout release-1.32
# If your fork uses its own remotes:
# git fetch forked && git checkout forked/release-1.32
```

If custom plugins (e.g. `pkg/flavourclusterwide/`) live on another branch, merge them into the release branch before building:

```bash
git merge <branch-with-custom-plugins>   # or cherry-pick the required commits
```

Verify that `go.mod` reflects the desired K8s version:

```bash
grep 'k8s.io/client-go' go.mod
# Should show v0.32.7 for Kubernetes 1.32
```

### (Optional) Bump K8s dependency patch

If you need a different patch than the branch default (e.g. `1.32.8`):

```bash
hack/upgrade-k8s.sh 1.32.8
make update-gomod
```

## 3. Pre-build verification

```bash
make verify    # formatting, go.mod, CRDs, etc.
make           # builds bin/kube-scheduler and bin/controller locally (optional)
```

## 4. Build the scheduler image

For a **secondary scheduler** on OpenShift/Kubernetes, the **kube-scheduler** image is usually enough (the controller is not required unless you use plugins that depend on it, e.g. coscheduling with PodGroup).

### Local build (single architecture, no push)

Load the image into a local registry (`localhost:5000`) or the local daemon with `--load`:

```bash
make local-image
```

Default output:

- `localhost:5000/scheduler-plugins/kube-scheduler:latest`
- `localhost:5000/scheduler-plugins/controller:latest` (both images; ignore the controller if unused)

For the **scheduler only** locally:

```bash
make clean build-scheduler-image \
  RELEASE_VERSION=v0.0.0 \
  REGISTRY=localhost:5000/scheduler-plugins \
  PLATFORMS=linux/$(uname -m) \
  EXTRA_ARGS="--load"
```

### Registry build (production / Quay)

The image tag should include the scheduler-plugins version as `v0.<minor>.<patch>` so the Makefile embeds the correct Kubernetes version in the binary (`v0.32.7` → `v1.32.7` in ldflags).

Recommended tag convention:

```text
v<YYYYMMDD>-v0.<k8s-minor>.<patch>
```

Example for **Kubernetes 1.32.7**:

```bash
export K8S_MINOR=32
export K8S_PATCH=7
export RELEASE_VERSION="v$(date +%Y%m%d)-v0.${K8S_MINOR}.${K8S_PATCH}"
export REGISTRY="quay.io/<user-or-org>/secondary-scheduler-plugins"
export PLATFORMS="linux/amd64"          # or linux/amd64,linux/arm64
export BUILDER="podman"                 # or docker

make clean build-scheduler-image \
  RELEASE_VERSION="${RELEASE_VERSION}" \
  REGISTRY="${REGISTRY}" \
  PLATFORMS="${PLATFORMS}" \
  BUILDER="${BUILDER}" \
  EXTRA_ARGS="--push"
```

Resulting image (Makefile default name):

```text
${REGISTRY}/kube-scheduler:${RELEASE_VERSION}
```

Example: `quay.io/mparrade/secondary-scheduler-plugins/kube-scheduler:v20250526-v0.32.7`

#### Flat tag (no `kube-scheduler/` path)

If deployment expects a flat name (e.g. `quay.io/mparrade/secondary-scheduler-plugins:v20250526-v0.32.7`):

```bash
make clean build-scheduler-image \
  RELEASE_VERSION="${RELEASE_VERSION}" \
  REGISTRY="quay.io/mparrade" \
  IMAGE="secondary-scheduler-plugins:${RELEASE_VERSION}" \
  PLATFORMS="${PLATFORMS}" \
  BUILDER="${BUILDER}" \
  EXTRA_ARGS="--push"
```

## 5. Examples by Kubernetes version

### Kubernetes 1.32

```bash
git checkout release-1.32
make verify

RELEASE_VERSION=v20250526-v0.32.7 \
REGISTRY=quay.io/mparrade \
IMAGE=secondary-scheduler-plugins:v20250526-v0.32.7 \
PLATFORMS=linux/amd64 \
BUILDER=podman \
EXTRA_ARGS="--push" \
make build-scheduler-image
```

Upstream reference: `registry.k8s.io/scheduler-plugins/kube-scheduler:v0.32.7` (built with k8s v1.32.7).

### Kubernetes 1.33

```bash
git checkout release-1.33
make verify

RELEASE_VERSION=v20250526-v0.33.5 \
REGISTRY=quay.io/mparrade \
IMAGE=secondary-scheduler-plugins:v20250526-v0.33.5 \
PLATFORMS=linux/amd64 \
BUILDER=podman \
EXTRA_ARGS="--push" \
make build-scheduler-image
```

Upstream reference: `registry.k8s.io/scheduler-plugins/kube-scheduler:v0.33.5` (built with k8s v1.33.5).

## 6. Relevant Makefile variables

| Variable | Description | Default / example |
|----------|-------------|-------------------|
| `RELEASE_VERSION` | Image tag and version embedded in the binary | `v20250526-v0.32.7` |
| `REGISTRY` | Target registry | `gcr.io/k8s-staging-scheduler-plugins` |
| `IMAGE` | Image name (without registry) | `kube-scheduler:$(RELEASE_VERSION)` |
| `PLATFORMS` | Architectures (buildx) | `linux/amd64,linux/arm64,...` |
| `BUILDER` | `podman`, `docker`, or `nerdctl` | `podman` |
| `EXTRA_ARGS` | buildx flags (`--load`, `--push`) | empty |
| `GO_BASE_IMAGE` | Build base image | `golang:$(go.mod version)` |
| `DISTROLESS_BASE_IMAGE` | Runtime image | `gcr.io/distroless/static:nonroot` |

The scheduler Dockerfile is at `build/scheduler/Dockerfile` and runs `make build-scheduler` inside the container.

## 7. Verify the image

```bash
# Version reported by the binary (should match the cluster minor)
podman run --rm quay.io/mparrade/secondary-scheduler-plugins:v20250526-v0.32.7 \
  /bin/kube-scheduler --version

# Binary only, no container
./bin/kube-scheduler --version
```

## 8. Use the image in the secondary scheduler

Set the image on the `SecondaryScheduler` CR or the operator deployment. See [README-flavour-cluster-wide.md](README-flavour-cluster-wide.md) for the `FlavourClusterWide` plugin and profile ConfigMap.

Example:

```yaml
spec:
  schedulerImage: 'quay.io/mparrade/secondary-scheduler-plugins:v20250526-v0.32.7'
```

Ensure `scheduler-config` enables the plugins you need in the secondary scheduler profile.

## 9. Official vs custom image

| Source | Image | When to use |
|--------|-------|-------------|
| Upstream | `registry.k8s.io/scheduler-plugins/kube-scheduler:v0.32.7` | No code changes; upstream plugins only |
| This repo | Own registry with custom tag | Custom plugins (`FlavourClusterWide`, etc.) or private patches |

## References

- [README.md](README.md) — available plugins and compatibility matrix
- [doc/develop.md](doc/develop.md) — development and `make local-image`
- [README-flavour-cluster-wide.md](README-flavour-cluster-wide.md) — cluster-wide plugin and OpenShift deployment
- [RELEASE.md](RELEASE.md) — upstream release process
