/*
Copyright 2024.

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

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/banzaicloud/k8s-objectmatcher/patch"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
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

const (
	objectName    = "so-object"
	finalizerName = "simpleoperator.szikes.io/finalizer"
	secretName    = "tls-cert"
)

var gentleRequeueAfterInterval = 3 * time.Second

// SimpleOperatorReconciler reconciles a SimpleOperator object
type SimpleOperatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=simpleoperator.szikes.io,resources=simpleoperators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=simpleoperator.szikes.io,resources=simpleoperators/status,verbs=update
//+kubebuilder:rbac:groups=simpleoperator.szikes.io,resources=simpleoperators/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get
//+kubebuilder:rbac:groups=apps,resources=deployments/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services/finalizers,verbs=update
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SimpleOperator object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *SimpleOperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.V(1).Info("Reconciling")

	soObject := &sov1alpha1.SimpleOperator{}
	err := r.Get(ctx, req.NamespacedName, soObject)
	if err != nil {
		if errors.IsNotFound(err) {
			log.V(1).Info("Custom object does NOT exist")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get custom object", "error", err.Error())
		return ctrl.Result{}, err
	}

	log.V(1).Info("Custom object exists")

	requeue := ctrl.Result{RequeueAfter: gentleRequeueAfterInterval}

	if controllerutil.ContainsFinalizer(soObject, finalizerName) {

		if !soObject.ObjectMeta.DeletionTimestamp.IsZero() {
			res, err := r.cleanupObjects(ctx, req)
			if err == nil {
				log.V(1).Info("Clean up objects succeeded", "obejctName", req.Name, "namespace", req.Namespace)
			} else {
				log.Error(err, "Failed to clean up objects, even if the custom object is marked for deletion", "error", err.Error(), "obejctName", req.Name, "namespace", req.Namespace)
			}
			return res, err
		}

	} else {
		controllerutil.AddFinalizer(soObject, finalizerName)
		err = r.updateWithRetry(ctx, soObject)
		if err != nil {
			log.Error(err, "Failed to add finalizer to custom object", "error", err.Error(), "obejctName", req.Name, "namespace", req.Namespace)
			return requeue, err
		}
	}

	deployRes, deployInfo, deployErr := r.reconcileBasedOnCustomObject(ctx, req, soObject, &appsv1.Deployment{}, newDeploymentObject(soObject))
	if deployErr != nil {
		log.V(0).Error(deployErr, "failed to reconcile", "error", deployErr.Error())
		return deployRes, deployErr
	}
	log.V(1).Info("Reconile on deployment object succeeded", "detail", deployInfo, "obejctName", req.Name, "namespace", req.Namespace)

	svcRes, svcInfo, svcErr := r.reconcileBasedOnCustomObject(ctx, req, soObject, &corev1.Service{}, newServiceObject(soObject))
	if svcErr != nil {
		log.V(0).Error(svcErr, "failed to reconcile", "error", svcErr.Error())
		return mergeCtrlResults(deployRes, svcRes), svcErr
	}
	log.V(1).Info("Reconile on service object succeeded", "detail", svcInfo, "obejctName", req.Name, "namespace", req.Namespace)

	ingRes, ingInfo, ingErr := r.reconcileBasedOnCustomObject(ctx, req, soObject, &networkingv1.Ingress{}, newIngressObject(soObject))
	if err != nil {
		log.V(0).Error(ingErr, "failed to reconcile", "error", ingErr.Error())
	} else if ingErr == nil {
		log.V(1).Info("Reconile on ingress object succeeded", "detial", ingInfo, "obejctName", req.Name, "namespace", req.Namespace)
	}
	return mergeCtrlResults(deployRes, svcRes, ingRes), ingErr
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

func (r *SimpleOperatorReconciler) cleanupObjects(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	requeue := ctrl.Result{RequeueAfter: gentleRequeueAfterInterval}

	res, err := r.deleteDeployedObject(ctx, req, &networkingv1.Ingress{})
	if err != nil {
		return res, err
	}

	res, err = r.deleteDeployedObject(ctx, req, &corev1.Service{})
	if err != nil {
		return res, err
	}

	res, err = r.deleteDeployedObject(ctx, req, &appsv1.Deployment{})
	if err != nil {
		return res, err
	}

	soObject := &sov1alpha1.SimpleOperator{}
	err = r.Get(ctx, req.NamespacedName, soObject)
	if err != nil {
		return requeue, fmt.Errorf("failed to get custom object: %w", err)
	}

	controllerutil.RemoveFinalizer(soObject, finalizerName)
	err = r.updateWithRetry(ctx, soObject)
	if err != nil {
		return requeue, fmt.Errorf("failed to remove finalizer on custom object: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *SimpleOperatorReconciler) deleteDeployedObject(ctx context.Context, req ctrl.Request, emptyObject client.Object) (ctrl.Result, error) {
	current := emptyObject
	objectKey := types.NamespacedName{Name: objectName, Namespace: req.Namespace}

	err := r.Get(ctx, objectKey, current)
	if err == nil {
		controllerutil.RemoveFinalizer(current, finalizerName)

		err = r.updateWithRetry(ctx, current)
		if err != nil {
			return ctrl.Result{RequeueAfter: gentleRequeueAfterInterval}, fmt.Errorf("failed to remove finalizer on object: %w, objectName: %v, objectKind: %v", err, objectName, resourceKindToString(current))
		}

		err := r.Delete(ctx, current)
		if err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: gentleRequeueAfterInterval}, fmt.Errorf("failed to delete object %w, objectName: %v, objectKind: %v", err, objectName, resourceKindToString(current))
		}
	}
	return ctrl.Result{}, nil
}

func (r *SimpleOperatorReconciler) reconcileBasedOnCustomObject(ctx context.Context, req ctrl.Request, soObject *sov1alpha1.SimpleOperator, empty client.Object, expected client.Object) (ctrl.Result, string, error) {
	var info strings.Builder
	var err error = nil
	var res ctrl.Result = ctrl.Result{RequeueAfter: gentleRequeueAfterInterval}
	var statusState string = sov1alpha1.Reconciled
	var statusErrMsg string = ""

	current := empty

	objectKey := types.NamespacedName{Name: objectName, Namespace: req.Namespace}
	err = r.Get(ctx, objectKey, current)
	if err == nil {
		res, status, msg, err := r.patchObject(ctx, soObject.Spec.Replicas, current, expected)
		statusState = status
		statusErrMsg = msg
		if err != nil {
			return res, info.String(), err
		}
		info.WriteString(fmt.Sprintf("handle patch on object succeeded: %s", msg))
	} else {
		if errors.IsNotFound(err) {
			res, status, msg, err := r.createObject(ctx, req, soObject, current, expected)
			statusState = status
			statusErrMsg = msg
			if err != nil {
				return res, info.String(), err
			}
			info.WriteString(fmt.Sprintf("object is NOT found: %s", msg))
		} else {
			err = fmt.Errorf("failed to get object: %w, objectName: %s, namespace: %s, kind: %s",
				err, expected.GetName(), expected.GetNamespace(), resourceKindToString(expected))
			statusState = sov1alpha1.InternalError
			statusErrMsg = err.Error()
			return res, info.String(), err
		}
	}

	if statusState == sov1alpha1.Reconciled {
		info.WriteString("reconciled")
		res = ctrl.Result{}
	}

	err = r.Get(ctx, req.NamespacedName, soObject)
	if err != nil {
		err = fmt.Errorf("failed to get custom object, juste before updating: %w, objecName: %v, namespace: %v", err, soObject.GetName(), soObject.GetNamespace())
		return res, err.Error(), err
	}

	deployment, ok := current.(*appsv1.Deployment)
	if ok {
		soObject.Status.AvabilableReplicas = deployment.Status.AvailableReplicas
	}

	status := threeWayStatusMerge(empty, soObject, statusState, statusErrMsg)
	soObject.Status = *status

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Status().Update(ctx, soObject)
	})
	if err != nil {
		return ctrl.Result{RequeueAfter: gentleRequeueAfterInterval}, info.String(), err
	}

	return res, info.String(), err
}

func (r *SimpleOperatorReconciler) createObject(ctx context.Context, req ctrl.Request, soObject *sov1alpha1.SimpleOperator, empty client.Object, expected client.Object) (res ctrl.Result, status string, msg string, err error) {
	status = sov1alpha1.FailedToCreate
	controllerutil.AddFinalizer(expected, finalizerName)

	err = ctrl.SetControllerReference(soObject, expected, r.Scheme)
	if err != nil {
		err = fmt.Errorf("failed to set contoller reference on object: %w, objectName: %v, namespace: %v, kind: %v",
			err, expected.GetName(), expected.GetNamespace(), resourceKindToString(expected))
		return ctrl.Result{RequeueAfter: gentleRequeueAfterInterval},
			status,
			err.Error(),
			err
	}

	err = patch.DefaultAnnotator.SetLastAppliedAnnotation(expected)
	if err != nil {
		err = fmt.Errorf("failed to set LastAppliedAnnotation on object: %w, objectName: %v, namespace: %v, kind: %v",
			err, expected.GetName(), expected.GetNamespace(), resourceKindToString(expected))
		return ctrl.Result{RequeueAfter: gentleRequeueAfterInterval},
			status,
			err.Error(),
			err
	}

	err = r.Create(ctx, expected)
	if err == nil {
		status = sov1alpha1.Creating
	} else if !errors.IsAlreadyExists(err) {
		status = sov1alpha1.FailedToCreate
		err = fmt.Errorf("failed to create expected object: %w, objectName: %v, namespace: %v, kind: %v",
			err, expected.GetName(), expected.GetNamespace(), resourceKindToString(expected))
		return ctrl.Result{RequeueAfter: gentleRequeueAfterInterval},
			status,
			err.Error(),
			err
	}
	return ctrl.Result{},
		status,
		fmt.Sprintf("creating object: objectName: %v, namespace: %v, kind: %v",
			expected.GetName(), expected.GetNamespace(), resourceKindToString(expected)),
		nil
}

func (r *SimpleOperatorReconciler) patchObject(ctx context.Context, expectedReplicasCount int32, current client.Object, expected client.Object) (res ctrl.Result, status string, msg string, err error) {
	status = sov1alpha1.InternalError

	opts := []patch.CalculateOption{
		patch.IgnoreStatusFields(),
		patch.IgnoreField("metadata"),
	}

	patchResult, err := patch.DefaultPatchMaker.Calculate(current.(runtime.Object), expected.(runtime.Object), opts...)
	if err != nil {
		err = fmt.Errorf("failed to calculate patch: %w, objectName: %v, namespace: %v, kind: %v", err, current.GetName(), current.GetNamespace(), resourceKindToString(current))
		return ctrl.Result{RequeueAfter: gentleRequeueAfterInterval},
			status,
			err.Error(),
			err
	}

	if !patchResult.IsEmpty() {
		err = r.updateWithRetry(ctx, expected)
		if err != nil {
			msg = fmt.Sprintf("failed to update objet for applying patch: %s, objectName: %v, namespace: %v, kind: %v", err.Error(), current.GetName(), current.GetNamespace(), resourceKindToString(current))
			status = sov1alpha1.FailedToUpdateChange
		} else {
			msg = "updated patch on oboject based on the expectation"
			status = sov1alpha1.UpdatingChange
		}
	} else if deployment, ok := current.(*appsv1.Deployment); ok &&
		(deployment.Status.AvailableReplicas != expectedReplicasCount) {
		msg = fmt.Sprintf("object is reconciling: expectedReplicas: %v, currentAvailableReplicas: %v, objectName: %v, namespace: %v, kind: %v",
			expectedReplicasCount, deployment.Status.AvailableReplicas, current.GetName(), current.GetNamespace(), resourceKindToString(current))
		status = sov1alpha1.Reconciling
	}
	status = sov1alpha1.Reconciled
	return ctrl.Result{}, status, msg, nil
}

// this solves the error of Update(): "the object has been modified; please apply your changes to the latest version and try again"
func (r *SimpleOperatorReconciler) updateWithRetry(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Update(ctx, obj, opts...)
	})
	if retryErr != nil {
		return retryErr
	}
	return nil
}

// WithEventFilter(myPredicate()).
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

func mergeCtrlResults(results ...ctrl.Result) ctrl.Result {
	res := ctrl.Result{}
	for _, result := range results {
		res.RequeueAfter = max(res.RequeueAfter, result.RequeueAfter)
	}
	return res
}

func resourceKindToString(obj any) string {
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

func threeWayStatusMerge(obj any, soObject *sov1alpha1.SimpleOperator, statusState string, statusErrMsg string) *sov1alpha1.SimpleOperatorStatus {
	status := sov1alpha1.SimpleOperatorStatus{
		LastUpdated:        metav1.Now(),
		AvabilableReplicas: soObject.Status.AvabilableReplicas,
		DeploymentState:    soObject.Status.DeploymentState,
		DeploymentErrorMsg: soObject.Status.DeploymentErrorMsg,
		ServiceState:       soObject.Status.ServiceState,
		ServiceErrorMsg:    soObject.Status.ServiceErrorMsg,
		IngressState:       soObject.Status.IngressState,
		IngressErrorMsg:    soObject.Status.IngressErrorMsg,
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

func newDeploymentObject(soObject *sov1alpha1.SimpleOperator) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectName,
			Namespace: soObject.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": objectName},
			},
			Replicas: &soObject.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": objectName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  objectName,
							Image: soObject.Spec.Image,
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

func newServiceObject(soObject *sov1alpha1.SimpleOperator) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectName,
			Namespace: soObject.Namespace,
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

func newIngressObject(soObject *sov1alpha1.SimpleOperator) *networkingv1.Ingress {
	pathType := networkingv1.PathType("Prefix")
	nginx := "nginx"
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectName,
			Namespace: soObject.Namespace,
			Annotations: map[string]string{
				"cert-manager.io/cluster-issuer":             "letsencrypt-staging",
				"nginx.ingress.kubernetes.io/rewrite-target": "/$1",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &nginx,
			TLS: []networkingv1.IngressTLS{
				{
					Hosts: []string{
						soObject.Spec.Host,
					},
					SecretName: secretName,
				},
			},
			Rules: []networkingv1.IngressRule{
				{
					Host: soObject.Spec.Host,
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
