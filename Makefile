.PHONY: start stop clean

start:
	./setup_cluster.sh
stop:
	minikube stop

clean:
	minikube delete

build-worker:
	docker build -t worker-pool:latest
	eval $$(minikube docker-env) && docker build -t worker-pool:latest .

deploy: build-worker
	kubectl apply -f worker-pool-deployment.yaml

redeploy: build-worker
	kubectl rollout restart deployment worker-pool
