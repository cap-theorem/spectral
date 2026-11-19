# Workstation: collector and visz

The demo's collector and visz run on the laptop you present from, not in the K3s cluster: the cluster nodes are small and far away, and the collector does the analysis while visz renders it. The sidecars in the cluster report to the laptop over Tailscale.

```
K3s cluster (peers only)                      this laptop (Docker)
foodbolt pods + spctrld  ──Tailscale──▶  collector  :9400  (ghcr.io/cap-theorem/spectral-collector)
                                         visz       :9500  ──▶ browser
```

## Start

From this directory, with Docker running, and `cap-theorem/visz` checked out next to this repository (or `VISZ_DIR` pointing at it):

```bash
docker compose up -d --build
```

Then open http://localhost:9500. visz is built to read the collector at http://localhost:9400. If your browser remembers an older collector URL, open http://localhost:9500/?collector=http://localhost:9400 once.

`COLLECTOR_TAG` picks the collector image (default `sha-297689b`). Keep it in step with the `SPCTRLD_TAG` the peers run. The collector drops a node one minute after its last report (`--stale-after=1m`).

Peers started with `scripts/demo` report to `http://<this machine's Tailscale IP>:9400` by default. Check that the cluster can reach it:

```bash
curl -s http://$(tailscale ip -4):9400/v1/summary
```

## Stop

```bash
docker compose down
```

The collector keeps the graph in memory only, so restarting it starts from an empty graph that fills again within one report interval (about 30 s). While this laptop sleeps or is off the tailnet, the sidecars' reports fail and the graph goes stale.
