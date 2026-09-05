In order to easily set up Kubernetes on local machines, I have adopted to use `kind` (Kubernetes in Docker).

### Basic Tests
Create and destroy the cluster used for local development

```bash
make kind-up
kubectl get nodes
make kind-down
kind get clusters
```

Note: depending on your installation you may need to run some commands under `sudo` to prevent errors.
