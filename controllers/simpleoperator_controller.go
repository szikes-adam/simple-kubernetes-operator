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

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sov1alpha1 "github.com/szikes-adam/simple-kubernetes-operator/api/v1alpha1"
)

const objectName = "so-object"
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
			log.V(1).Info("Custom object does NOT exist")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Unable to get custom object")
		return ctrl.Result{}, err
	}

	log.V(1).Info("Custom object exists")

	requeue := ctrl.Result{RequeueAfter: time.Second * 3}

	if controllerutil.ContainsFinalizer(sor, finalizerName) {

		if !sor.ObjectMeta.DeletionTimestamp.IsZero() {
			return cleanupObjects(r, &log, ctx, req)
		}

	} else {
		log.V(0).Info("Newly added custom object, adding finalizer")
		controllerutil.AddFinalizer(sor, finalizerName)
		if err := r.Update(ctx, sor); err != nil {
			log.Error(err, "Unable to add finalizer to customer object")
			return requeue, err
		}
	}

	deployRes, deployErr := reconcileBasedOnCustomObject(r, &log, ctx, req, sor, &appsv1.Deployment{}, createExpectedDeployment(sor))
	if deployErr != nil {
		return deployRes, deployErr
	}

	svcRes, svcErr := reconcileBasedOnCustomObject(r, &log, ctx, req, sor, &corev1.Service{}, createExpectedService(sor))
	if svcErr != nil {
		if deployRes.RequeueAfter != 0 {
			svcRes = deployRes
		}
		return svcRes, svcErr
	}

	ingRes, ingErr := reconcileBasedOnCustomObject(r, &log, ctx, req, sor, &networkingv1.Ingress{}, createExpectedIngress(sor))
	if deployRes.RequeueAfter != 0 {
		ingRes = deployRes
	} else if svcRes.RequeueAfter != 0 {
		ingRes = svcRes
	}
	return ingRes, ingErr
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

func getObjctKind(obj interface{}) string {
	switch obj.(type) {
	case *appsv1.Deployment:
		return "Deployment"
	case *corev1.Service:
		return "Service"
	case *networkingv1.Ingress:
		return "Ingress"
	}
	return ""
}

func deleteDeployedObject(r *SimpleOperatorReconciler, log *logr.Logger, ctx context.Context, req ctrl.Request, emptyObject client.Object) (ctrl.Result, error) {
	current := emptyObject
	objectKey := types.NamespacedName{Name: objectName, Namespace: req.Namespace}
	if err := r.Get(ctx, objectKey, current); err == nil {

		log.V(0).Info("Deleting deployed object", "objectName", objectName, "objectKind", getObjctKind(current))

		controllerutil.RemoveFinalizer(current, finalizerName)

		if err := r.Update(ctx, current); err != nil {
			log.Error(err, "Unable to update object for removing finalizer")
			return ctrl.Result{RequeueAfter: time.Second * 3}, err
		}

		if err := r.Delete(ctx, current); err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Unable to delete deployed object", "objectName", objectName, "objectKind", getObjctKind(current))
			return ctrl.Result{RequeueAfter: time.Second * 3}, err
		}
	}

	return ctrl.Result{}, nil
}

func cleanupObjects(r *SimpleOperatorReconciler, log *logr.Logger, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	requeue := ctrl.Result{RequeueAfter: time.Second * 3}

	log.V(0).Info("Custom object is marked for deletion, deleting it together with deployed objects")

	if res, err := deleteDeployedObject(r, log, ctx, req, &networkingv1.Ingress{}); err != nil {
		return res, err
	}

	if res, err := deleteDeployedObject(r, log, ctx, req, &corev1.Service{}); err != nil {
		return res, err
	}

	if res, err := deleteDeployedObject(r, log, ctx, req, &appsv1.Deployment{}); err != nil {
		return res, err
	}

	sor := &sov1alpha1.SimpleOperator{}
	if err := r.Get(ctx, req.NamespacedName, sor); err != nil {
		log.Error(err, "Unable to get custom object before removing finalizer")
		return requeue, err
	}

	log.V(0).Info("Removing finalizer from custom object")
	controllerutil.RemoveFinalizer(sor, finalizerName)
	if err := r.Update(ctx, sor); err != nil {
		log.Error(err, "Unable to remove finalizer from customer object")
		return requeue, err
	}

	return ctrl.Result{}, nil
}

func threeWayStatusMerge(obj interface{}, sor *sov1alpha1.SimpleOperator, statusState string, statusErrMsg string) *sov1alpha1.SimpleOperatorStatus {
	status := sov1alpha1.SimpleOperatorStatus{
		LastUpdated:        readTimeInRFC3339(),
		AvabilableReplicas: sor.Status.AvabilableReplicas,
		DeploymentState:    sor.Status.DeploymentState,
		DeploymentErrorMsg: sor.Status.DeploymentErrorMsg,
		ServiceState:       sor.Status.ServiceState,
		ServiceErrorMsg:    sor.Status.ServiceErrorMsg,
		IngressState:       sor.Status.IngressState,
		IngressErrorMsg:    sor.Status.IngressErrorMsg,
	}

	switch obj.(type) {
	case *appsv1.Deployment:
		status.DeploymentState = statusState
		status.DeploymentErrorMsg = statusErrMsg
	case *corev1.Service:
		status.ServiceState = statusState
		status.ServiceErrorMsg = statusErrMsg
	case *networkingv1.Ingress:
		status.IngressState = statusState
		status.IngressErrorMsg = statusErrMsg
	}
	return &status
}

func reconcileBasedOnCustomObject(r *SimpleOperatorReconciler, l *logr.Logger, ctx context.Context, req ctrl.Request, sor *sov1alpha1.SimpleOperator, empty client.Object, expected client.Object) (ctrl.Result, error) {
	var err error = nil
	var res ctrl.Result = ctrl.Result{RequeueAfter: time.Second * 3}
	var statusState string = sov1alpha1.Reconciled
	var statusErrMsg string = ""

	current := empty
	log := l.WithValues("objectName", objectName, "objectKind", getObjctKind(current))

	objectKey := types.NamespacedName{Name: objectName, Namespace: req.Namespace}
	if err := r.Get(ctx, objectKey, current); err == nil {

		opts := []patch.CalculateOption{
			patch.IgnoreStatusFields(),
			patch.IgnoreField("metadata"),
		}

		patchResult, err := patch.DefaultPatchMaker.Calculate(current.(runtime.Object), expected.(runtime.Object), opts...)
		if err != nil {
			return res, err
		}

		if !patchResult.IsEmpty() {
			log.V(0).Info("Updating the currently deployed object based on the contoller expectation")

			if err := r.Update(ctx, expected); err == nil {
				statusState = sov1alpha1.UpdatingChange
			} else {
				log.Error(err, "Unable to update the currently deployed object based on the contoller expectation")
				statusState = sov1alpha1.FailedToUpdateChange
			}

		} else if deployment, ok := current.(*appsv1.Deployment); ok && (deployment.Status.AvailableReplicas != sor.Spec.Replicas) {
			l.V(0).Info("Deployed object is reconciling", "expectedReplicas", sor.Spec.Replicas, "currentAvailableReplicas", deployment.Status.AvailableReplicas)
			statusState = sov1alpha1.Reconciling
		}
	} else {
		if errors.IsNotFound(err) {

			log.V(0).Info("Deployed object is NOT found, creating it")

			controllerutil.AddFinalizer(expected, finalizerName)

			if err = ctrl.SetControllerReference(sor, expected, r.Scheme); err != nil {
				log.Error(err, "Unable to set controller reference on deployed object")
				return res, err
			}

			if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(expected); err != nil {
				log.Error(err, "Unable to set LastAppliedAnnotation on deployed object")
				return res, err
			}

			if latestErr := r.Create(ctx, expected); latestErr == nil {
				statusState = sov1alpha1.Creating

			} else if !errors.IsAlreadyExists(latestErr) {
				log.Error(latestErr, "Unable to create the expected object")
				statusState = sov1alpha1.FailedToCreate
				statusErrMsg = latestErr.Error()
			}

		} else {
			statusState = sov1alpha1.InternalError
			statusErrMsg = err.Error()
			log.Error(err, "Unable to get deployed object")
		}
	}

	if statusState == sov1alpha1.Reconciled {
		res = ctrl.Result{}
	}

	if err != nil {
		return res, err
	}

	if deployment, ok := current.(*appsv1.Deployment); ok {
		sor.Status.AvabilableReplicas = deployment.Status.AvailableReplicas
	}

	if err = r.Get(ctx, req.NamespacedName, sor); err != nil {
		log.Error(err, "Unable to get custom object, just before updating it")
		return res, err
	}

	status := threeWayStatusMerge(empty, sor, statusState, statusErrMsg)
	sor.Status = *status

	if err := r.Status().Update(ctx, sor); err != nil {
		if errors.IsConflict(err) {
			log.V(1).Info("Unable to update status of custom object due to ResourceVersion mismatch, retrying the update")
			res = ctrl.Result{RequeueAfter: time.Second * 3}
		} else {
			return ctrl.Result{RequeueAfter: time.Second * 3}, err
		}
	}

	return res, err
}

func createExpectedDeployment(sor *sov1alpha1.SimpleOperator) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectName,
			Namespace: sor.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": objectName},
			},
			Replicas: &sor.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": objectName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  objectName,
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

func createExpectedService(sor *sov1alpha1.SimpleOperator) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectName,
			Namespace: sor.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": objectName},
			Ports: []corev1.ServicePort{
				{
					Port: 80,
				},
			},
		},
	}
}

func createExpectedIngress(sor *sov1alpha1.SimpleOperator) *networkingv1.Ingress {
	pathType := networkingv1.PathType("Prefix")
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectName,
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
											Name: objectName,
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
