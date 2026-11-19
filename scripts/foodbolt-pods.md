# Bare Spectral/Foodbolt pods

`scripts/foodbolt-pods` creates unmanaged Foodbolt Pods, each with a Spectral sidecar, for one peer network identified by `FOODBOLT_RUN`. Every Pod uses `restartPolicy: Never` and has no controller, so Kubernetes never restarts an exited container or replaces a Pod that was deleted or lost with its node. A peer exists only because someone ran the script.

Every peer in a run is a seed. The headless Service `spctrld-peers-<run>` selects all peers of the run, and each sidecar bootstraps through it. spctrld resolves that name again on every seed pull and when it finds itself isolated, so any live peer can admit a new one, and killing a particular peer never blocks joins.

The first invocation for a run, when no peers are live, also starts a sidecar-only Pod, `spctrld-seed-<run>`, so the first workers have someone to join. After that it is an ordinary peer: you can kill it like any other. If every peer in a run has died, clean up the run (see below) or use a new run ID; the script refuses to start a second, isolated seed next to the old one.

## Prerequisites

- A kubeconfig for the K3s cluster at `https://100.90.92.118:6443`.
- The old `spectral` Deployments scaled to zero. The script refuses to run while their pods exist and does not scale them down for you.
- Spectral's topology collector running on your laptop, from `infra/workstation` (see its README). The cluster nodes are small and far away, so the collector and visz run on the machine you present from, and the sidecars report to it over Tailscale.
- `SPECTRAL_COLLECTOR_URL` set to that collector as the cluster sees it, `http://<your Tailscale IP>:9400`, or to an empty string to start peers that do not report. `scripts/demo` sets it for you.

## Usage

From fish, in the repository root:

```fish
set -gx KUBECONFIG "$HOME/.kube/spectral-k3s.yaml"
kubectl -n spectral scale deployment/spctrld --replicas=0
kubectl -n spectral get pods  # Wait until the old spctrld pods are gone.

set -gx FOODBOLT_RUN demo-1
set -gx SPCTRLD_TAG sha-297689b
set -gx FOODBOLT_TAG sha-abcdef1  # Replace with the published Foodbolt image tag.
set -gx SPECTRAL_COLLECTOR_URL http://(tailscale ip -4):9400
env DRY_RUN=1 ./scripts/foodbolt-pods gateway 2  # Print the YAML without contacting Kubernetes.
./scripts/foodbolt-pods gateway 2
./scripts/foodbolt-pods orders 3
./scripts/foodbolt-pods restaurants 2
./scripts/foodbolt-pods payments 2
./scripts/foodbolt-pods dispatch 2 ip-172-31-23-240  # Pin to one machine.
kubectl -n foodbolt get pods -l spectral.dev/run=demo-1 -o wide
```

The optional third argument is a **Kubernetes hostname label value**, such as `ip-172-31-23-240`, not a Tailscale machine name. Check values with `kubectl get nodes -L kubernetes.io/hostname`. Without it, Kubernetes spreads workers across nodes.

Starting `gateway` also creates the NodePort Service `gateway-<run>` and prints its port; reach it from the tailnet at `http://100.90.92.118:<port>`.

## Killing peers

```fish
kubectl -n foodbolt delete pod orders-demo-1-x7k2q                            # Graceful: spctrld sends Leave.
kubectl -n foodbolt delete pod orders-demo-1-x7k2q --grace-period=0 --force   # Crash: no Leave.
```

Neither comes back. Foodbolt keeps state in memory per instance, so resources owned by a killed instance (orders, payments, tickets, deliveries) are gone with it and return 404.

## Capacity

Start with small batches. Each worker Pod requests 35m CPU and 40Mi memory across its two containers; 100 Pods request 3.5 CPUs and about 4 GiB. Actual usage may be higher, and the current three nodes have roughly 2 GiB each. Watch `kubectl top nodes` and Pod events before increasing the count.

## Cleanup

```fish
kubectl -n foodbolt delete pods,services -l spectral.dev/run=demo-1
```

Do not reapply `infra/k8s/overlays/cloud` during a run: it restores the 20-replica `spctrld` Deployment.
