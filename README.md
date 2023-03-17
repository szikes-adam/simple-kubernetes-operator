# simple-kubernetes-operator (so)

![CI](https://github.com/szykes/simple-kubernetes-operator/actions/workflows/ci.yml/badge.svg) ![Docker](https://github.com/szykes/simple-kubernetes-operator/actions/workflows/docker.yml/badge.svg)

As the git project names says this is a really simple kubernetes operator implementation.

> All commands must executed at level of git project root

## Prerequisite

Having installed `docker` (`engine` version 23.0.1, `containerd` version: 1.6.18), `kubectl` (v1.26.2), and `kind` (v0.17.0) on a Linux based server.

Server has CPU Intel J3455, 8 GB RAM, and having 60 GB free space for /.

Clone or download the repo.

### Setup a `kind` based cluster

Create cluster with `kind`:
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

Now we have a cluster environment.

Reference:
[phoenixNAP - Guide to Running Kubernetes with Kind](https://phoenixnap.com/kb/kubernetes-kind)

### Setup `NGINX` as Ingress in `kind`

Setup `NGINX` for `kind`:
```
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
```

Wait until the `NGINX` is deployed:
```
kubectl wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=90s
```

Check what resources are deployed:
```
kubectl get all --namespace ingress-nginx
```

Reference:
[kind - Ingress](https://kind.sigs.k8s.io/docs/user/ingress/#ingress-nginx)

### Domain name question

My advantage is I own a domain name and the network infrastructre has been already prepared to use it. However, in an ordinary home this is not available, so I would like to give you some hints what you need to do. But before doing anything you need to check wheter your router is behind [CGN](https://www.a10networks.com/glossary/what-is-carrier-grade-nat-cgn-cgnat/). Being under CGN makes harder your life to use HTTP-01 challange, either asking your ISP to give public IP address, using the VPN, or switching to DNS-01 challange can help in this case.

If the WAN IP address on your router and your IP address on [whatsmyip](https://www.whatsmyip.org) are not matching, it will mean your router is under CGN.

Hints:
* Set static IP address for the `kind` runner machine in router -> find DHCP server settings on the router and manually assgin IP to MAC address of machine.
* Open port 80 & 443 -> find Port Forwading menu and set internal and external ports to 443 and use static IP address for internal IP address.
* Sign up on [no-ip](https://www.noip.com) and create a No-IP Hostname -> After the login navigate to Dynamic DNS, No-IP Hostnames and Create Hostname. You can use whatever hostname but leave the Record Type on DNS Host (A).

Please do not forget ISP gives you dynamic IP address to your router that may change, so you need to update the IP address of you No-IP Hostname. I don't know wheter there is an automatic way.

Let's use the [FQDN](https://www.techtarget.com/whatis/definition/fully-qualified-domain-name-FQDN) e.g.: `szykes.ddns.net` in Ingress.

### Setup `cert-manager` with `Let's Encrypt` for `NGINX`

Setup `cert-manager`:
```
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.11.0/cert-manager.yaml
```

Check what resources are deployed:
```
kubectl get pods --namespace cert-manager
```

Install the staging issuer:
```
kubectl create -f staging-issuer.yml
```

Check the current status of certificate creation:
```
kubectl get certificate -o wide
```

I use staging issuer because I can verify TLS certifcate mechanism in this way without bothering the production side of `Let's encrypt`.

If everything goes well, you will see something like this:




<img width="553" alt="Screenshot 2023-03-17 at 19 31 36" src="https://user-images.githubusercontent.com/8822138/226025230-db70d767-340a-4268-a7f1-d72986f59cbb.png">

Reference:
[DigitalOcean - How to Set Up an Nginx Ingress with Cert-Manager on DigitalOcean Kubernetes](https://www.digitalocean.com/community/tutorials/how-to-set-up-an-nginx-ingress-with-cert-manager-on-digitalocean-kubernetes#step-4-installing-and-configuring-cert-manager)
[cert-manager - Troubleshooting Problems with ACME / Let's Encrypt Certificates](https://cert-manager.io/docs/troubleshooting/acme/)
[kubernetes - Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/#tls)

## Project creation

### Custom Resource Controller

Install `go` (1.19) & `kubebuilder` (3.9.1) at first.

Change directory to git project and execute:
```
kubebuilder init --domain szikes.io --repo github.com/szikes-adam/simple-kubernetes-operator

kubebuilder create api --group simpleoperator --version v1alpha1 --kind SimpleOperator
```
+ extend manually the api/v1alpha1/simpleoperator_types.go based on [kubebuilder - CRD validation](https://book.kubebuilder.io/reference/markers/crd-validation.html)

Reference:
[kubebuilder - Tutorial: Building CronJob](https://book.kubebuilder.io/cronjob-tutorial/cronjob-tutorial.html)
[kubebuilder - Adding a new API](https://book.kubebuilder.io/cronjob-tutorial/new-api.html)

### GitHub Actions

### CI

It builds, vets, and runs test using by `make`.

Triggered by pushing new commit on `main` and pull request.

File location in project:
`.github/workflows/ci.yml`

Reference:
[GitHub - Building and testing Go](https://docs.github.com/en/actions/automating-builds-and-tests/building-and-testing-go)
[banzaicloud/koperator - ci.yml](https://github.com/banzaicloud/koperator/blob/master/.github/workflows/ci.yml)

### Docker

It builds docker image by using `Dockerfile` at the project root.

The images are availble on `ghcr.io`.

Building and pushing docker images are triggered by pushing new commit on `main` and tag with the following version format `'*.*.*'`. For example: 2.10.5

File location in project:
`.github/workflows/docker.yml`

Reference:
[GitHub - Publishing Docker images](https://docs.github.com/en/actions/publishing-packages/publishing-docker-images)
[banzaicloud/koperator - docker.yml](https://github.com/banzaicloud/koperator/blob/master/.github/workflows/docker.yml)

### Accessing docker images

At first read & do: [Creating a personal access token (PAT)](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-personal-access-token)

Login with docker on the machine that needs access:
```
docker login ghcr.io
```
> It will ask for your username on GitHub and your PAT

If everything does well, you will see this:
```
WARNING! Your password will be stored unencrypted in /home/buherton/.docker/config.json.
Configure a credential helper to remove this warning. See
https://docs.docker.com/engine/reference/commandline/login/#credentials-store

Login Succeeded
```

Verifying access by:
```
docker pull ghcr.io/szykes/simple-kubernetes-operator:main
```

You should see similar to this:
```
main: Pulling from szykes/simple-kubernetes-operator
10f855b03c8a: Pull complete
fe5ca62666f0: Pull complete
b438aade3922: Pull complete
fcb6f6d2c998: Pull complete
e8c73c638ae9: Pull complete
1e3d9b7d1452: Pull complete
4aa0ea1413d3: Pull complete
7c881f9ab25e: Pull complete
5627a970d25e: Pull complete
aefd672debf9: Pull complete
Digest: sha256:48e6d8e4cd8252ba3044a1baae7deac41e1be42d80320c3b27d6fae2f14c4cc0
Status: Downloaded newer image for ghcr.io/szykes/simple-kubernetes-operator:main
ghcr.io/szykes/simple-kubernetes-operator:main
```

Reference:
[GitHub - Working with the Container registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)

## Build, Install, Run

### Build controller

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

Reference:
[kubebuilder - Running and deploying the controller](https://book.kubebuilder.io/cronjob-tutorial/running.html)

## Further development

Not all areas of this project were deeply investigated and built due to limited time.

Here is the list that I would do in a next phase:
* Use `:latest` tag for docker image
* Have a proper versioning (rc, beta, etc.) for git project and docker image

* Use TLS between within cluster
* Encrypt Secrets
