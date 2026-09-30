# Spectral

Spectral is an experimental service discovery system for environments for where service instances frequently start, stop, or fail (i.e. distributed systems). Its goal is to let applications find available service instances without depending on a central registry.

The intended deployment runs a Spectral daemon alongside each application used within a system. These daemons communicate with one another to discover services across a network (currently simulated with Kubernetes). Each daemon maintains connections to a small number of peers, keeping the network connected without requiring every daemon to connect to every other daemon. We are attempting to use expander graphs in order to keep this network reliable.

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

Which will build the daemon's docker image, push it to kind, and tell the cluster to restart spctrld deployments with the updated version.
