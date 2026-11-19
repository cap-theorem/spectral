# Demo controls

`scripts/demo` drives one Foodbolt-on-Spectral run on the K3s cluster: look at it, add peers, kill them in every way that matters, and send orders through it. It builds on `scripts/foodbolt-pods`, so every peer is an unmanaged Pod with `restartPolicy: Never`: nothing you kill comes back unless you start it again.

`scripts/demo help` lists every command with the defaults it will use.

## Defaults

| Variable | Default |
|---|---|
| `KUBECONFIG` | `~/.kube/spectral-k3s.yaml` |
| `FOODBOLT_RUN` | `demo-1` |
| `SPCTRLD_TAG` | `sha-297689b` |
| `FOODBOLT_TAG` | `sha-5be11a9` |
| `SPECTRAL_COLLECTOR_URL` | `http://<this machine's Tailscale IP>:9400`, i.e. a collector running on your laptop |
| `DEMO_SSH_USER` | `ubuntu` |

New peers report to `SPECTRAL_COLLECTOR_URL`, and `status` and `graph` read its summary from there. The collector must be reachable from the cluster over Tailscale, and it stops getting reports when your laptop sleeps.

## Commands

```fish
scripts/demo status                      # every pod, its machine, and whether the app and spctrld are up, dead or leaving
scripts/demo watch                       # the same, refreshed every 2s: keep this open on a second screen
scripts/demo machines                    # nodes, their Tailscale IPs, peers per node
scripts/demo graph                       # collector: nodes, edges, components, degree, spectral gap
scripts/demo logs orders-demo-1-kv       # follow the service's logs; add spctrld for the sidecar's

scripts/demo up orders 2                 # two new orders peers
scripts/demo up dispatch 1 ip-172-31-23-240   # pinned to one machine
scripts/demo up-random 5                 # five peers of random services
scripts/demo up-all                      # one of every service

scripts/demo kill orders-demo-1-kv       # graceful: SIGTERM, spctrld sends Leave
scripts/demo crash orders-demo-1-kv      # instant: no Leave, peers must detect it
scripts/demo kill-random 3               # three random workers, gracefully
scripts/demo crash-random 2 payments     # two random payments peers, instantly
scripts/demo kill-app payments-demo-1-n6 # stop only the service; spctrld keeps running
scripts/demo kill-spctrld orders-demo-1-c9 --hard   # stop only the sidecar, with SIGKILL
scripts/demo crash-machine ip-172-31-23-240         # every peer on one machine, instantly

scripts/demo order 10                    # ten orders as random customers, each with its route
scripts/demo order 1 dave                # dave's card is declined: expect a 402
scripts/demo gateway                     # the gateway URL, for your own curl

scripts/demo down                        # delete the whole run
```

A pod name can be shortened to any unique prefix. Random kills only pick live workers, never the seed pod; kill the seed by name if you want it gone.

`kill-app` and `kill-spctrld` stop one container through `crictl` on the node, over SSH to the node's Tailscale IP, so you need SSH access as `ubuntu` with passwordless sudo, which the Ansible setup already gives you. Without `--hard`, the container gets SIGTERM and 10s to exit; with it, SIGKILL. The stopped container is never restarted, and `status` shows it as `dead(<exit code>)`. A pod whose service is dead keeps a live sidecar, so it stays in the overlay graph while its lease expires and lookups stop returning it; a pod whose sidecar is dead leaves the graph while its service keeps running and fails its lookups.

`crash-machine` deletes pods; it does not stop the machine. To take a real node down, stop K3s on it (`sudo systemctl stop k3s-agent`) and note that its pods then sit in `Unknown` rather than disappearing.

## Things to show

- **Join**: `watch` in one terminal, `up-random 5` in another, and the collector's node count and gap update as the new peers join through any live peer.
- **Routing around failure**: `order 5`, then `crash-random 1 orders`, then `order 5` again: the routes change and orders keep succeeding.
- **Total loss of a service**: kill every `payments` peer and orders fail with `payments unavailable`; `up payments` and they succeed again within a lease.
- **Losing a machine**: `crash-machine` one node, then `graph`: the overlay stays in one component.
