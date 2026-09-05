kind-up:
	sudo kind create cluster \
		--config infra/kind/cluster.yaml
		--wait 30s

kind-down:
	sudo kind delete cluster --name spectral-dev
