# Spectral cloud deployment

This overlay deploys one Spectral seed and 19 regular Spectral replicas to the K3s cluster. Regular pods bootstrap through the headless `spctrld-seed` Service (UDP 7000); the seed starts without a bootstrap address. The seed is a peer, not a central relay. Kubernetes can schedule it on any eligible node, and its one-replica Deployment replaces it if it fails. The existing `spctrld-peers` Service still selects regular pods, but is not used for bootstrap in this overlay.

The local kind manifests under `infra/k8s/spectral` are unchanged. The overlay spreads regular pods across available nodes when possible and pulls both workloads from GHCR.

## Publish a commit image

`.github/workflows/ci.yml` builds and publishes `ghcr.io/cap-theorem/spctrld:sha-<first 7 characters of commit SHA>` (and the collector image) when a commit lands on `main`. To publish a pushed feature-branch commit before merging, run the workflow manually:

```sh
gh workflow run ci.yml --ref <branch-name>
```

GitHub requires `workflow_dispatch` to exist on the default branch, so use the local build below until this workflow change is merged. Wait for the CI image job to finish before deploying. For a local publish instead, authenticate with `docker login ghcr.io` using a token with `write:packages`, then run from the repository root:

```sh
sha=$(git rev-parse --short=7 HEAD)
docker buildx build --platform linux/amd64,linux/arm64 \
  --file cmd/spctrld/Dockerfile \
  --build-arg "VERSION=$sha" \
  --tag "ghcr.io/cap-theorem/spctrld:sha-$sha" \
  --push .
```

Commit your changes before publishing. Image tags identify committed source; uncommitted changes in a local build would make the tag misleading. Set `newTag` in `kustomization.yaml` to the tag that was actually published. The checked-in tag refers to the repository HEAD when this overlay was written and is only usable if that image has been published.

## Deploy

Ensure your kubeconfig points to the K3s cluster, not kind, and check the context before applying:

```sh
kubectl config current-context
kubectl apply -k infra/k8s/overlays/cloud
kubectl -n spectral rollout status deployment/spctrld-seed --timeout=300s
kubectl -n spectral rollout status deployment/spctrld --timeout=300s
kubectl -n spectral get deployments,pods -o wide
```

Apply `infra/k8s/observability` separately if you want the configured collector and dashboards. GHCR must allow anonymous pulls for the published image, or the `spectral` namespace must have a GHCR `docker-registry` Secret and the Deployment must reference it using `imagePullSecrets`. Do not commit registry credentials or a cluster-admin kubeconfig.
