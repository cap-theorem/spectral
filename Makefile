kind-up:
	kind create cluster \
		--config infra/kind/cluster.yaml
		--wait 30s

kind-down:
	kind delete cluster --name spectral-dev
