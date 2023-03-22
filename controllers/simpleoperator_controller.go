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
	"fmt"
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

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sov1alpha1 "github.com/szikes-adam/simple-kubernetes-operator/api/v1alpha1"
)

const resourceName = "so-resource"
const finalizerName = "simpleoperator.szikes.io/finalizer"
const secretName = "tls-cert"

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

	sor.Status.LastUpdated = readTimeInRFC3339()

	var latestErr error = nil
	var latestRes ctrl.Result = ctrl.Result{RequeueAfter: time.Second * 3}

	latestRes, latestErr, sor.Status.DeploymentState, sor.Status.DeploymentErrorMsg = reconcileBasedOnCustomResourceObject(r, &log, ctx, req, sor, &appsv1.Deployment{}, createMetaDeployment(req), createExpectedDeployment(sor))

	if latestErr != nil {
		return latestRes, latestErr
	}

	if err := r.Status().Update(ctx, sor); err != nil {
		log.V(1).Info("Error when updating status, trying again")
		return ctrl.Result{RequeueAfter: time.Second * 3}, err
	}

	// latestRes, latestErr, sor.Status.ServiceState, sor.Status.ServiceErrorMsg = reconcileBasedOnCustomResourceObject(r, &log, ctx, req, sor, &corev1.Service{}, createMetaService(req), createExpectedService(sor))

	// if latestErr != nil {
	// 	return latestRes, latestErr
	// }

	// if err := r.Status().Update(ctx, sor); err != nil {
	// 	log.V(1).Info("Error when updating status, trying again")
	// 	return ctrl.Result{RequeueAfter: time.Second * 3}, err
	// }

	// latestRes, latestErr, sor.Status.IngressState, sor.Status.IngressErrorMsg = reconcileBasedOnCustomResourceObject(r, &log, ctx, req, sor, &networkingv1.Ingress{}, createMetaIngress(req), createExpectedIngress(sor))

	// if latestErr != nil {
	// 	return latestRes, latestErr
	// }

	// if err := r.Status().Update(ctx, sor); err != nil {
	// 	log.V(1).Info("Error when updating status, trying again")
	// 	return ctrl.Result{RequeueAfter: time.Second * 3}, err
	// }

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
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
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

func getUnhandledObjectTypeError(obj interface{}) error {
	return fmt.Errorf("Unhandled resource object type: %T", obj)
}

func compareSides(obj interface{}, sor *sov1alpha1.SimpleOperator) (bool, bool) {
	var isExpectationsMatch bool = true
	var isStatusMatch bool = true

	switch obj.(type) {
	case *appsv1.Deployment:
		currentDeploy := obj.(*appsv1.Deployment)
		isExpectationsMatch = *currentDeploy.Spec.Replicas == sor.Spec.Replicas && currentDeploy.Spec.Template.Spec.Containers[0].Image == sor.Spec.Image
		isStatusMatch = currentDeploy.Status.AvailableReplicas == sor.Spec.Replicas
	case *corev1.Service:
	case *networkingv1.Ingress:

	}
	return isExpectationsMatch, isStatusMatch
}

func logExpectationMismatch(obj interface{}, log *logr.Logger, sor *sov1alpha1.SimpleOperator) {
	switch obj.(type) {
	case *appsv1.Deployment:
		currentDeploy := obj.(*appsv1.Deployment)
		log.V(0).Info("Deployment mismatches to custom resource object, updating it", "expectedReplicas", sor.Spec.Replicas, "setReplicas", *currentDeploy.Spec.Replicas, "expectedImage", sor.Spec.Image, "setImage", currentDeploy.Spec.Template.Spec.Containers[0].Image)
	}
}

func logStatusMismatch(obj interface{}, log *logr.Logger, sor *sov1alpha1.SimpleOperator) {
	switch obj.(type) {
	case *appsv1.Deployment:
		currentDeploy := obj.(*appsv1.Deployment)
		log.V(0).Info("Deployment is reconciling", "expectedReplicas", sor.Spec.Replicas, "currentReplicas", currentDeploy.Status.AvailableReplicas)
	}
}

func updateCustomResourceStatus(obj interface{}, sor *sov1alpha1.SimpleOperator) {
	switch obj.(type) {
	case *appsv1.Deployment:
		currentDeploy := obj.(*appsv1.Deployment)
		sor.Status.AvabilableReplicas = currentDeploy.Status.AvailableReplicas
	}
}

func reconcileBasedOnCustomResourceObject(r *SimpleOperatorReconciler, log *logr.Logger, ctx context.Context, req ctrl.Request, sor *sov1alpha1.SimpleOperator, emptyObject client.Object, metaObject client.Object, expectedObject client.Object) (ctrl.Result, error, string, string) {
	var latestErr error = nil
	var latestRes ctrl.Result = ctrl.Result{RequeueAfter: time.Second * 3}
	var statusState string = sov1alpha1.Reconciled
	var statusErrMsg string = ""

	currentObj := emptyObject

	switch currentObj.(type) {
	case *appsv1.Deployment:
	case *corev1.Service:
	case *networkingv1.Ingress:
	default:
		return ctrl.Result{}, errors.NewInternalError(getUnhandledObjectTypeError(currentObj)), sov1alpha1.InternalError, getUnhandledObjectTypeError(currentObj).Error()
	}

	objectKey := types.NamespacedName{Name: resourceName, Namespace: req.Namespace}
	if latestErr := r.Get(ctx, objectKey, currentObj); latestErr == nil {

		isExpectationsMatch, isStatusMatch := compareSides(currentObj, sor)

		if !isExpectationsMatch {

			logExpectationMismatch(currentObj, log, sor)

			expected := createExpectedDeployment(sor)
			if latestErr := r.Update(ctx, expected); latestErr == nil {
				statusState = sov1alpha1.UpdatingChange
			} else {
				log.Error(latestErr, "Unable to update")
				statusState = sov1alpha1.FailedToUpdateChange
			}

		} else if !isStatusMatch {
			logStatusMismatch(currentObj, log, sor)
			statusState = sov1alpha1.Reconciling
		}

		updateCustomResourceStatus(currentObj, sor)

	} else {
		if errors.IsNotFound(latestErr) {

			log.V(0).Info("Deployment is NOT found, creating it")

			deploy := createExpectedDeployment(sor)
			if latestErr = ctrl.SetControllerReference(sor, deploy, r.Scheme); latestErr != nil {
				log.Error(latestErr, "Unable to set reference")
				return latestRes, latestErr, statusState, statusErrMsg
			}

			controllerutil.AddFinalizer(deploy, finalizerName)

			if latestErr := r.Create(ctx, deploy); latestErr == nil {
				statusState = sov1alpha1.Creating

			} else if !errors.IsAlreadyExists(latestErr) {
				log.Error(latestErr, "Unable to create the expected deployment")
				statusState = sov1alpha1.FailedToCreate
				statusErrMsg = latestErr.Error()
			}

		} else {
			statusState = sov1alpha1.InternalError
			statusErrMsg = latestErr.Error()
			log.Error(latestErr, "Unable to fetch deployment resource")
		}
	}

	if statusState == sov1alpha1.Reconciled {
		latestRes = ctrl.Result{}
	}

	return latestRes, latestErr, statusState, statusErrMsg
}

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

func createMetaService(req ctrl.Request) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: req.Namespace,
		},
	}
}

func createExpectedService(sor *sov1alpha1.SimpleOperator) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: sor.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": resourceName},
			Ports: []corev1.ServicePort{
				{
					Port: 80,
				},
			},
		},
	}
}

func createMetaIngress(req ctrl.Request) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: req.Namespace,
		},
	}
}

func createExpectedIngress(sor *sov1alpha1.SimpleOperator) *networkingv1.Ingress {
	pathType := networkingv1.PathType("Prefix")
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: sor.Namespace,
			Annotations: map[string]string{
				"cert-manager.io/cluster-issuer":             "letsencrypt-staging",
				"kubernetes.io/ingress.class":                "nginx",
				"nginx.ingress.kubernetes.io/rewrite-target": "/$1",
			},
		},
		Spec: networkingv1.IngressSpec{
			TLS: []networkingv1.IngressTLS{
				{
					Hosts: []string{
						sor.Spec.Host,
					},
					SecretName: secretName,
				},
			},
			Rules: []networkingv1.IngressRule{
				{
					Host: sor.Spec.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									PathType: &pathType,
									Path:     "/",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: resourceName,
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
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
