# Implementation of a workerpool

Based on [gamma zero workerpool](https://github.com/gammazero/workerpool/tree/master) but implements the following for local testing on minikube:

- minikube local cluster setup with Deployments for testing and development
- read tasks from a redis queue

## Setup

To setup the mini cluster, check the minikube setup instructions for installation, and run the following commands:
```make start```
to spin up the local cluster

```make build-worker```
to build the workerpool docker image and setup the minikube local docker daemon

```make deploy```
to deploy the redis queue and worker

```make stop``` or ```minikube stop``` to spin down the local cluster
