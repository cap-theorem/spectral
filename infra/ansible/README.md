## Cluster hosts

The inventory contains only the `spectral-h1divp-*` Tailscale machines. `spectral-h1divp-1` (`100.90.92.118`) is the K3s server; `spectral-h1divp-2` (`100.65.82.89`) and `spectral-h1divp-3` (`100.114.90.93`) are agents. K3s servers also run workloads by default. Check that the inventory's `ansible_user: ubuntu` matches the SSH account on each machine.

## Bootstrap and new nodes

The existing two-node cluster on `spectral-h1divp-1` and `spectral-h1divp-2` does not need to be reset. To check SSH access and rerun the playbook for those nodes from the repository root:

```sh
cd infra/ansible
ansible 'spectral-h1divp-1,spectral-h1divp-2' -m ping
ansible-playbook playbooks/cluster.yaml --limit 'spectral-h1divp-1,spectral-h1divp-2'
```

Once SSH works on `spectral-h1divp-3`, add it by running:

```sh
ansible all -m ping
ansible-playbook playbooks/cluster.yaml
```

The server must be included in each cluster playbook run because the agent play reads the join token registered by the server play. K3s advertises each node's Tailscale IP and configures Flannel to use `tailscale0`. On the server, check membership with `sudo k3s kubectl get nodes -o wide`.

## Local kubeconfig

To sync the admin kubeconfig to the machine running Ansible, run from `infra/ansible`:

```sh
ansible-playbook playbooks/kubeconfig.yaml
kubectl --kubeconfig "$HOME/.kube/spectral-k3s.yaml" get nodes -o wide
KUBECONFIG="$HOME/.kube/spectral-k3s.yaml" k9s
```

The sync playbook contacts only the K3s server. It copies `/etc/rancher/k3s/k3s.yaml` to `~/.kube/spectral-k3s.yaml`, changes its API address to the server's Tailscale IP, and sets local permissions to `0600`. Keep the kubeconfig private; it grants cluster-admin access.
