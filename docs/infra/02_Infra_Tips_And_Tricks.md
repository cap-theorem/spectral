### Infra Tips

Since I know not everyone here is experienced with k8s, here are some commands which you might find useful

See state of whole cluster
```shell
sudo kubectl get nodes
```

Checking the state of pods within a namespace
```shell
kubectl get pods -n observability -o wide
# Or use
kubectl get pods -n observability --watch
# Or use
kubectl get deployments,pods,services,configmaps -n observability
```

Follow the logs of a pod
```shell
kubectl logs -n observability deployment/otel-collector --follow 
```

Get quick documentation on manifest fields
```shell
kubectl explain deployment.spec.template.spec.containers | less
```

For validating k8s manifests, use the tool [kubeconform](https://github.com/yannh/kubeconform)
Validating manifests with it looks like
```shell
kubectl kustomize infra/k8s/spectral | kubeconform -strict -summary
```

After getting conforming kustomize output, dry-run `kubectl apply` before making changes
```shell
sudo kubectl apply -k infra/k8s/observability/ \
    --dry-run=server \
    --validate=strict
```
`-k` flag applies all manifests in a directory to the cluster
Note: apply a namespace manifest before running this, if creating a new namespace

Check status of newly applied manifests
```shell
kubectrl rollout status deployment/spctrld -n spectrl
```
