KIND_CLUSTER := spectral-dev
SPCTRLD_IMAGE := spctrl:dev
SPCTRLD_NAMESPACE := spectral
SPCTRLD_DEPLOYMENT := spctrld

kind-up:
	kind create cluster \
		--config infra/kind/cluster.yaml

kind-down:
	kind delete cluster --name spectral-dev

.PHONY: observability-apply
observability-apply:
	kubectl apply -k infra/k8s/observability
	kubectl rollout status deployment/otel-collector \
		-n observability \
		--timeout=120s

.PHONY: spctrld-build
spctrld-build:
	docker build --tag $(SPCTRLD_IMAGE) .

.PHONY: spctrld-load
spctrld-load:
	kind load docker-image $(SPCTRLD_IMAGE) \
		--name $(KIND_CLUSTER)

.PHONY: spctrld-apply
spctrld-apply:
	kubectl apply -k infra/k8s/spectral

.PHONY: spctrld-rollout
spctrld-rollout:
	kubectl rollout restart \
		deployment/$(SPCTRLD_DEPLOYMENT) \
		-n $(SPCTRLD_NAMESPACE)
	kubectl rollout status \
		deployment/$(SPCTRLD_DEPLOYMENT) \
		-n $(SPCTRLD_NAMESPACE) \
		--timeout=120s

.PHONY: spctrld-dev
spctrld-dev:
	$(MAKE) spctrld-build
	$(MAKE) spctrld-load
	$(MAKE) spctrld-apply
	$(MAKE) spctrld-rollout

.PHONY: spctrld-logs
spctrld-logs:
	kubectl logs \
	-n $(SPCTRLD_NAMESPACE) \
	--follow \
	deployment/$(SPCTRLD_DEPLOYMENT)

.PHONY: spctrld-status
spctrld-status:
	kubectl get deployments,pods,services \
		-n $(SPCTRLD_NAMESPACE) \
		--output wide

.PHONY: bootstrap
bootstrap:
	KIND_CLUSTER="$(KIND_CLUSTER)" ./scripts/bootstrap
	
run:
	go run ./cmd/spctrld
