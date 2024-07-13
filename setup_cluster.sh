#!/bin/bash

set -e

minikube start

while ! kubectl get nodes; do
	echo "Waiting for Minikube to be ready..."
	sleep 5
done

minikube addons enable ingress
minikube addons enable metrics-server

kubectl create deployment redis --image=redis
kubectl expose deployment redis --port=6379 --target-port=6379

while [[ $(kubectl get pods -l app=redis -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do
	echo "Waiting for Redis pod to be ready..."
	sleep 5
done

kubectl apply -f worker-pool-deployment.yaml

while [[ $(kubectl get pods -l app=worker-pool -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do
	echo "Waiting for worker pool pod to be ready..."
	sleep 5
done

echo "Minikube cluster is ready with Redis and worker pool deployed!"

kubectl port-forward svc/redis 6379:6379 &

echo "Redis is accessible on localhost:6379"

minikube ip
kubectl get all --all-namespaces
