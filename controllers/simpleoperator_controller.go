/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	simpleoperatorv1alpha1 "github.com/szikes-adam/simple-kubernetes-operator/api/v1alpha1"
)

// SimpleOperatorReconciler reconciles a SimpleOperator object
type SimpleOperatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=simpleoperator.szikes.io,resources=simpleoperators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=simpleoperator.szikes.io,resources=simpleoperators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=simpleoperator.szikes.io,resources=simpleoperators/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SimpleOperator object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *SimpleOperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	deploymentReq := &simpleoperatorv1alpha1.SimpleOperator{}

	if err := r.Get(ctx, req.NamespacedName, deploymentReq); err != nil {
		log.Error(err, "unable to fetch deployment request")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	pod := CreateDeployment(deploymentReq)
	if err := r.Create(ctx, pod); err != nil {
		log.Error(err, "unable to create pod")
		return ctrl.Result{}, err
	}

	log.Info("pod is created")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SimpleOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&simpleoperatorv1alpha1.SimpleOperator{}).
		Complete(r)
}

func CreateDeployment(deploymentReq *simpleoperatorv1alpha1.SimpleOperator) *corev1.Pod {
	imageName := strings.Split(deploymentReq.Spec.Image, ":")[0]
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      imageName,
			Namespace: deploymentReq.Namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  imageName,
					Image: deploymentReq.Spec.Image,
					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 6080,
							Protocol:      corev1.ProtocolTCP,
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyOnFailure,
		},
	}
}