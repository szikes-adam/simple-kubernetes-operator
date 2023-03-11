# simple-kubernetes-operator

## Prerequisite

Having installed docker (engine version 23.0.1, containerd version: 1.6.18), kubectl (v1.26.2), and kind (v0.17.0).

I use a machine with CPU Intel J3455, 8 GB RAM, and having 60 GB free space for /.

Clone or download the repo.

> All commands must executed at level of git project root

Create cluster:
```
kind create cluster --name=simple-operator --config=simple-1-control-2-workers.yaml
```

If everything goes well, the `$HOME/.kube/config` will contain the certificates, context, etc. of `simple-operator` as with name `kind-simple-operator`.

Just run to verify above statement:
```
kubectl cluster-info
```
You must see this:
```
Kubernetes control plane is running at https://127.0.0.1:36279
CoreDNS is running at https://127.0.0.1:36279/api/v1/namespaces/kube-system/services/kube-dns:dns/proxy

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```

Now we have a cluster environment. Let's jump into the interesting part.
