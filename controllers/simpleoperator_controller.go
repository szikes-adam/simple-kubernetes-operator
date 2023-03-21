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
	"time"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	//networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sov1alpha1 "github.com/szikes-adam/simple-kubernetes-operator/api/v1alpha1"
)

const resourceName = "so-resource"
const finalizerName = "simpleoperator.szikes.io/finalizer"

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

	log.V(1).Info("Reconciling")

	sor := &sov1alpha1.SimpleOperator{}
	if err := r.Get(ctx, req.NamespacedName, sor); err != nil {

		if errors.IsNotFound(err) {
			log.V(1).Info("Custom resource object does NOT exist")

			currentDeploy := &appsv1.Deployment{}
			objectKey := types.NamespacedName{Name: resourceName, Namespace: req.Namespace}
			if err := r.Get(ctx, objectKey, currentDeploy); err == nil {

				if !currentDeploy.ObjectMeta.DeletionTimestamp.IsZero() {
					log.V(0).Info("Deployed object marked for deletion, deleting it", "deletionTimestamp", currentDeploy.ObjectMeta.DeletionTimestamp)

					if err := r.Delete(ctx, currentDeploy); err != nil && !errors.IsNotFound(err) {
						log.Error(err, "Unable to delete deployment")
						return ctrl.Result{RequeueAfter: time.Second * 3}, err
					}

					controllerutil.RemoveFinalizer(currentDeploy, finalizerName)

					if err := r.Update(ctx, currentDeploy); err != nil {
						log.Error(err, "Unable to update")
						return ctrl.Result{}, err
					}
				} else {
					log.Error(nil, "Dangling deployed object")
				}
			}

			return ctrl.Result{}, nil

		} else {
			log.Error(err, "Unable to fetch simpleoperator resource")
		}

		return ctrl.Result{}, err
	}

	log.V(1).Info("Custom resource object exists")

	var latestErr error = nil
	var latestRes ctrl.Result = ctrl.Result{RequeueAfter: time.Second * 3}

	sor.Status.LastUpdated = readTimeInRFC3339()
	sor.Status.DeploymentState = sov1alpha1.Reconciled
	sor.Status.ServiceState = sov1alpha1.Reconciled
	sor.Status.IngressState = sov1alpha1.Reconciled
	sor.Status.DeploymentErrorMsg = ""
	sor.Status.ServiceErrorMsg = ""
	sor.Status.IngressErrorMsg = ""

	// check deployment

	currentDeploy := &appsv1.Deployment{}
	objectKey := types.NamespacedName{Name: resourceName, Namespace: req.Namespace}
	if latestErr := r.Get(ctx, objectKey, currentDeploy); latestErr == nil {

		if *currentDeploy.Spec.Replicas != sor.Spec.Replicas ||
			currentDeploy.Spec.Template.Spec.Containers[0].Image != sor.Spec.Image {

			log.V(0).Info("Deployment mismatches to custom resource object, updating it", "expectedReplicas", sor.Spec.Replicas, "setReplicas", *currentDeploy.Spec.Replicas, "expectedImage", sor.Spec.Image, "setImage", currentDeploy.Spec.Template.Spec.Containers[0].Image)

			expected := createExpectedDeployment(sor)
			if latestErr := r.Update(ctx, expected); latestErr == nil {
				sor.Status.DeploymentState = sov1alpha1.UpdatingChange
			} else {
				log.Error(latestErr, "Unable to update")
				sor.Status.DeploymentState = sov1alpha1.FailedToUpdateChange
			}

		} else if currentDeploy.Status.AvailableReplicas != sor.Spec.Replicas {
			log.V(0).Info("Deployment is reconciling", "expectedReplicas", sor.Spec.Replicas, "currentReplicas", currentDeploy.Status.AvailableReplicas)
			sor.Status.DeploymentState = sov1alpha1.Reconciling
		}

		sor.Status.AvabilableReplicas = currentDeploy.Status.AvailableReplicas

	} else {
		if errors.IsNotFound(latestErr) {

			log.V(0).Info("Deployment is NOT found, creating it")

			deploy := createExpectedDeployment(sor)
			if latestErr = ctrl.SetControllerReference(sor, deploy, r.Scheme); latestErr != nil {
				log.Error(latestErr, "Unable to set reference")
				return latestRes, latestErr
			}

			controllerutil.AddFinalizer(deploy, finalizerName)

			if latestErr := r.Create(ctx, deploy); latestErr == nil {
				sor.Status.DeploymentState = sov1alpha1.Creating

			} else if !errors.IsAlreadyExists(latestErr) {
				log.Error(latestErr, "Unable to create the expected deployment")
				sor.Status.DeploymentState = sov1alpha1.FailedToCreate
				sor.Status.DeploymentErrorMsg = latestErr.Error()
			}

		} else {
			sor.Status.DeploymentState = sov1alpha1.InternalError
			sor.Status.DeploymentErrorMsg = latestErr.Error()
			log.Error(latestErr, "Unable to fetch deployment resource")
		}
	}

	if sor.Status.DeploymentState == sov1alpha1.Reconciled {
		latestRes = ctrl.Result{}
	}

	if err := r.Status().Update(ctx, sor); err != nil {
		log.V(1).Info("Error when updating status, trying again")
		return ctrl.Result{RequeueAfter: time.Second * 3}, err
	}

	return latestRes, latestErr
}

func (r *SimpleOperatorReconciler) UpdateStatus(sor *sov1alpha1.SimpleOperator, log logr.Logger, ctx context.Context, res reconcile.Result) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, sor); err != nil {
		log.Info("Error when updating status. Let's try again")
		return ctrl.Result{RequeueAfter: time.Second * 3}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SimpleOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sov1alpha1.SimpleOperator{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

// 		WithEventFilter(myPredicate()).
// func myPredicate() predicate.Predicate {
// 	return predicate.Funcs{
// 		CreateFunc: func(e event.CreateEvent) bool {
// 			return true
// 		},
// 		UpdateFunc: func(e event.UpdateEvent) bool {
// 			if _, ok := e.ObjectOld.(*core.Pod); !ok {
// 				// Is Not Pod
// 				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
// 			}
// 			// Is Pod
// 			return false
// 		},
// 		DeleteFunc: func(e event.DeleteEvent) bool {
// 			return !e.DeleteStateUnknown
// 		},
// 	}
// }

func createMetaDeployment(req ctrl.Request) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: req.Namespace,
		},
	}
}

func createExpectedDeployment(sor *sov1alpha1.SimpleOperator) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: sor.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": resourceName},
			},
			Replicas: &sor.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": resourceName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  resourceName,
							Image: sor.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
		},
	}
}

func readTimeInRFC3339() string {
	RFC3339dateLayout := "2006-01-02T15:04:05Z07:00"
	t := metav1.Now()
	return t.Format(RFC3339dateLayout)
}
