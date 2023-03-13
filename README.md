# simple-kubernetes-operator (so)

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

## Install & Run

# Install Custom Resource Definition (CRD)

Install CRD:
```
kubectl apply -f so-crd.yaml
```

After installation the `simpleoperators` CRD will be available as an ordinary resource:
```
kubectl api-resources | grep simpl
simpleoperators                   so           szikes.io/v1alpha1                     true         SimpleOperator
```

# Create custom object

Modify the `so-create.yaml` based on your needs then execute:
```
kubectl apply -f so-create.yaml
```

Simple object check:
```
kubectl get so
```
```
NAME                    AGE
simpleoperator-szikes   6s
```

Verbose object check:
```
kubectl describe so simpleoperator-szikes
```
```
Name:         simpleoperator-szikes
Namespace:    default
Labels:       <none>
Annotations:  <none>
API Version:  szikes.io/v1alpha1
Kind:         SimpleOperator
Metadata:
  Creation Timestamp:  2023-03-12T14:55:35Z
  Generation:          1
  Managed Fields:
    API Version:  szikes.io/v1alpha1
    Fields Type:  FieldsV1
    fieldsV1:
      f:metadata:
        f:annotations:
          .:
          f:kubectl.kubernetes.io/last-applied-configuration:
      f:spec:
        .:
        f:host:
        f:image:
        f:replicas:
    Manager:         kubectl-client-side-apply
    Operation:       Update
    Time:            2023-03-12T14:55:35Z
  Resource Version:  131720
  UID:               26c25c50-dd5d-4af9-a612-30675797afb6
Spec:
  Host:      host?
  Image:     hello-world:latest
  Replicas:  5
Events:      <none>
```

## Controller creation

Install go (1.19), kubebuilder (3.9.1) at first.

Move to git project and execute:
```
kubebuilder init --domain szikes.io --repo github.com/szikes-adam/simple-kubernetes-operator

kubebuilder create api --group simpleoperator --version v1alpha1 --kind SimpleOperator
```
+ extend manually the api/v1alpha1/simpleoperator_types.go based on https://book.kubebuilder.io/reference/markers/crd-validation.html

## Build controller

If you made API changes then run:
```
make manifests
```

But you can skip the previous step because the following will genreate CRD and install on cluster:
```
make install
```

```
export ENABLE_WEBHOOKS=false
make run
```
