# Spectral

`spectral` is an experimental decentralized service-discovery system for high-churn environments.

Each sidecar participates in a sparse expander overlay, advertises local services, and discovers live providers without relying on a central registry. The design combines DEX-inspired graph maintenance with randomized discovery and epoch-based topology resizing.

## How It Works

- Constant-degree expander overlay for scalable connectivity
- Randomized walks for membership sampling
- Parallel multi-path service lookups
- Stable node identities across topology epochs
- Join, graceful-leave, and crash handling
- Inflation and deflation as population changes

## Quick Start

### Requirements

General development
- Go 1.27+
- Docker

Network simulation
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/)

### Setup

Install the listed requirements, and set up the cluster with
```shell
make bootstrap
```

When you are done developing and need to delete the cluster, make sure to shut it down:
```shell
make kind-down
```

### Development
When you make changes to the spectral daemon while testing, update your local kubernetes cluster with
```shell
make spctrld-dev
```

Which will build the daemon's docker image, push it to kind, and tell the cluster to restart spctrld pods with the updated version.
