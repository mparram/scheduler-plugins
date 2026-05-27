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

## 4. Build and publish container images

Run `make build-images` to build **kube-scheduler** and **controller** images. The Makefile sets `REGISTRY`, `RELEASE_VERSION`, `PLATFORMS`, `BUILDER`, and related defaults (see [section 6](#6-optional-makefile-overrides)).

For the secondary scheduler you only need the **kube-scheduler** image unless you use plugins that require the controller (e.g. coscheduling with PodGroup).

Default image names after build:

```text
gcr.io/k8s-staging-scheduler-plugins/kube-scheduler:<RELEASE_VERSION>
gcr.io/k8s-staging-scheduler-plugins/controller:<RELEASE_VERSION>
```

`<RELEASE_VERSION>` is generated automatically (date + `git describe`). List local images with `podman images`.

### Build

```bash
make build-images
```

### Tag and push with Podman

Tag the built scheduler image to the name your registry / `SecondaryScheduler` CR expects, then push:

```bash
podman login quay.io

podman tag \
  gcr.io/k8s-staging-scheduler-plugins/kube-scheduler:<RELEASE_VERSION> \
  quay.io/<user-or-org>/secondary-scheduler-plugins:<RELEASE_VERSION>

podman push quay.io/<user-or-org>/secondary-scheduler-plugins:<RELEASE_VERSION>
```

Replace `<RELEASE_VERSION>` with the tag shown by `podman images` (same value on both sides of `podman tag`).

Controller (only if needed):

```bash
podman tag \
  gcr.io/k8s-staging-scheduler-plugins/controller:<RELEASE_VERSION> \
  quay.io/<user-or-org>/secondary-scheduler-plugins-controller:<RELEASE_VERSION>

podman push quay.io/<user-or-org>/secondary-scheduler-plugins-controller:<RELEASE_VERSION>
```

## 5. Examples by Kubernetes version

### Kubernetes 1.34

```bash
git checkout release-1.34
make verify
make build-images
podman images   # note kube-scheduler tag
podman tag gcr.io/k8s-staging-scheduler-plugins/kube-scheduler:<tag> \
  quay.io/mparrade/secondary-scheduler-plugins:<tag>
podman push quay.io/mparrade/secondary-scheduler-plugins:<tag>
```

Upstream reference: `registry.k8s.io/scheduler-plugins/kube-scheduler:v0.34.7` (built with k8s v1.34.7).

### Kubernetes 1.32

Same flow on `release-1.32`. Upstream reference: `kube-scheduler:v0.32.7`.

## 6. Optional Makefile overrides

Override only when the defaults are not what you need:

| Variable | Default |
|----------|---------|
| `RELEASE_VERSION` | `v<date>-<git describe>` |
| `REGISTRY` | `gcr.io/k8s-staging-scheduler-plugins` |
| `PLATFORMS` | `linux/amd64,linux/arm64,...` |
| `BUILDER` | `podman` |

Example: `make build-images REGISTRY=localhost/scheduler-plugins PLATFORMS=linux/amd64 EXTRA_ARGS="--load"`

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
- [doc/develop.md](doc/develop.md) — upstream development guide
- [README-flavour-cluster-wide.md](README-flavour-cluster-wide.md) — cluster-wide plugin and OpenShift deployment
- [RELEASE.md](RELEASE.md) — upstream release process
