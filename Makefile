kind-up:
	kind create cluster \
		--config infra/kind/cluster.yaml

kind-down:
	kind delete cluster --name spectral-dev

run:
	go run ./cmd/spctrld
