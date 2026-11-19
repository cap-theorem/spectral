# Bare Spectral/Foodbolt pods

`scripts/foodbolt-pods` creates one unmanaged Spectral seed Pod and a headless Service per `FOODBOLT_RUN`. Each invocation then creates the requested number of unmanaged Foodbolt Pods with a Spectral sidecar. Invoke it again with the **same run ID** for other Foodbolt services; all their peers bootstrap through the same seed. Every Pod uses `restartPolicy: Never`, so Kubernetes does not restart exited containers or replace pods lost with a node.

The script requires a kubeconfig for the K3s cluster at `https://100.90.92.118:6443` and refuses to run while pods from the old `spectral` Deployments remain. It does not shut them down for you. From fish, in the repository root:

```fish
set -gx KUBECONFIG "$HOME/.kube/spectral-k3s.yaml"
kubectl -n spectral get deployments
kubectl -n spectral scale deployment/spctrld --replicas=0
# Run this only if the seed Deployment exists:
kubectl -n spectral scale deployment/spctrld-seed --replicas=0
kubectl -n spectral get pods  # Wait until the old spctrld pods are gone.

set -gx FOODBOLT_RUN demo-1
set -gx SPCTRLD_TAG sha-3aa9d86
set -gx FOODBOLT_TAG sha-abcdef1  # Replace with the published Foodbolt image tag.
env DRY_RUN=1 ./scripts/foodbolt-pods gateway 2  # Print the YAML without contacting Kubernetes.
./scripts/foodbolt-pods gateway 5
./scripts/foodbolt-pods orders 5
kubectl -n foodbolt get pods -l spectral.dev/run=demo-1 -o wide
```

The script creates the `foodbolt` Namespace if necessary. The seed is started and checked before any workers are created. The script keeps the seed Pod on repeated invocations; a failed seed will not be restarted. The optional third argument is a **Kubernetes hostname label value**, such as `ip-172-31-23-240`, rather than a Tailscale machine name. Check values with `kubectl get nodes -L kubernetes.io/hostname`. Without it, Kubernetes attempts to spread workers across nodes.

Set `SPECTRAL_COLLECTOR_URL` if you have separately deployed Spectral's topology collector. The OpenTelemetry collector in `infra/k8s/observability` is a different service and does not provide the topology API. The Foodbolt images must exist in GHCR and be pullable by the cluster.

Start with small batches. The default requests per worker Pod are 35m CPU and 40Mi memory across its two containers; 100 Pods request 3.5 CPUs and about 4 GiB, excluding the seed and other workloads. Actual usage may be higher, and the current three nodes have roughly 2 GiB each. Watch `kubectl top nodes` and Pod events before increasing the count.

For cleanup, including any partially created batch:

```fish
kubectl -n foodbolt delete pods,services -l spectral.dev/run=demo-1
```

Do not reapply `infra/k8s/overlays/cloud` during this test: doing so restores its Deployments to 19 workers plus one seed.
