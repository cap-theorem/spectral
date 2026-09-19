In order to easily set up Kubernetes on local machines, we are making use of `kind` (*k*ubernetes *in* *d*ocker).

### Basic Tests

1. Create and destroy the cluster used for local development

  ```bash
  make kind-up
  kubectl get nodes
  make kind-down
  kind get clusters
  ```

  Note: depending on your installation you may need to run some commands under `sudo` to prevent errors.
