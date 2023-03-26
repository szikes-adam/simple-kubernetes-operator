package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sov1alpha1 "github.com/szikes-adam/simple-kubernetes-operator/api/v1alpha1"
)

var _ = Describe("SimpleOperator controller", func() {

	const (
		soObjectName      = "simple-operator"
		soObjectNamespace = "default"
		soObjectKind      = "SimpleOperator"

		objectName      = "so-object"
		objectNamespace = "default"

		finalizerName = "simpleoperator.szikes.io/finalizer"
		patchingName  = "banzaicloud.com/last-applied"

		timeout  = time.Second * 10
		interval = time.Millisecond * 250

		ordinaryHost  = "so.io"
		differentHost = "diff.so.io"

		ordinaryImage  = "hello-world"
		differentImage = "diff-hello-world"

		noReplicas        int32 = 0
		ordinaryReplicas  int32 = 3
		differentReplicas int32 = 5
	)

	createdObjectKey := types.NamespacedName{Name: objectName, Namespace: objectNamespace}
	soObjectKey := types.NamespacedName{Name: soObjectName, Namespace: soObjectNamespace}

	ctx := context.Background()

	waitForReconciledState := func(expectedReplicas int32) *sov1alpha1.SimpleOperator {
		soObject := &sov1alpha1.SimpleOperator{}
		Eventually(func() string {
			if err := k8sClient.Get(ctx, soObjectKey, soObject); err != nil {
				return err.Error()
			}

			// TODO: add soObject.Status.LastUpdated checking
			if soObject.Status.DeploymentState != sov1alpha1.Reconciled ||
				soObject.Status.DeploymentErrorMsg != "" ||
				soObject.Status.ServiceState != sov1alpha1.Reconciled ||
				soObject.Status.ServiceErrorMsg != "" ||
				soObject.Status.IngressState != sov1alpha1.Reconciled ||
				soObject.Status.IngressErrorMsg != "" ||
				soObject.Status.AvabilableReplicas != expectedReplicas {
				return "Some objects are not reconciled yet"
			}
			return ""
		}, timeout, interval).Should(Equal(""))
		return soObject
	}

	waitForCreatedObject := func(object client.Object) {
		Eventually(func() string {
			if err := k8sClient.Get(ctx, createdObjectKey, object); err != nil {
				return err.Error()
			}
			return ""
		}, timeout, interval).Should(Equal(""))
	}

	waitForDeletedOjbect := func(object client.Object) {
		Eventually(func() string {
			if err := k8sClient.Get(ctx, createdObjectKey, object); err != nil {
				return ""
			}
			return "The object is not deleted yet"
		}, timeout, interval).Should(Equal(""))
	}

	createSimpleOperatorObject := func(objectName, objectNamespace, host, image string, replicas int32) *sov1alpha1.SimpleOperator {
		return &sov1alpha1.SimpleOperator{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "simpleoperator.szikes.io/v1alpha1",
				Kind:       soObjectKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      soObjectName,
				Namespace: soObjectNamespace,
			},
			Spec: sov1alpha1.SimpleOperatorSpec{
				Host:     host,
				Image:    image,
				Replicas: replicas,
			},
		}
	}

	doReconcile := func(replicas int32) {
		Eventually(func() string {
			object := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, createdObjectKey, object); err != nil {
				return err.Error()
			}

			object.Status.Replicas = replicas
			object.Status.ReadyReplicas = replicas
			object.Status.AvailableReplicas = replicas

			if err := k8sClient.Status().Update(ctx, object); err != nil {
				return err.Error()
			}

			return ""
		}, timeout, interval).Should(Equal(""))
	}

	deleteSimpleOperatorSafely := func() {
		soObject := &sov1alpha1.SimpleOperator{}
		if err := k8sClient.Get(ctx, soObjectKey, soObject); err != nil {
			if errors.IsNotFound(err) {
				return
			}
			Expect(err.Error()).To(Equal(""))
		}
		if err := k8sClient.Delete(ctx, soObject); err != nil {
			if errors.IsNotFound(err) {
				return
			}
			Expect(err.Error()).To(Equal(""))
		}

		Eventually(func() string {
			if err := k8sClient.Get(ctx, soObjectKey, soObject); err != nil {
				if errors.IsNotFound(err) {
					return ""
				}
				return err.Error()
			}

			controllerutil.RemoveFinalizer(soObject, finalizerName)

			if err := k8sClient.Update(ctx, soObject); err != nil {
				if errors.IsNotFound(err) {
					return ""
				}
				return err.Error()
			}
			return "The SimpleOperator object is not deleted yet"
		}, timeout, interval).Should(Equal(""))
	}

	Describe("Given an ordinary SimpleOperator object", func() {

		BeforeEach(func() {
			soObject := createSimpleOperatorObject(objectName, objectNamespace, ordinaryHost, ordinaryImage, ordinaryReplicas)
			Expect(k8sClient.Create(ctx, soObject)).Should(Succeed())
		})

		AfterEach(func() {
			deleteSimpleOperatorSafely()
		})

		Context("When we are just right after the creation of SimpleOperator object", func() {
			It("Then the controller adds finalizer to SimpleOperator object", func() {
				soObject := &sov1alpha1.SimpleOperator{}
				Eventually(func() string {
					if err := k8sClient.Get(ctx, soObjectKey, soObject); err != nil {
						return err.Error()
					}
					if len(soObject.ObjectMeta.Finalizers) > 0 && soObject.ObjectMeta.Finalizers[0] != finalizerName {
						return "The finalizer is not added yet"
					}
					return ""
				}, timeout, interval).Should(Equal(""))
			})

			It("Then the controller creates Deployment object based on SimpleOperator object", func() {
				doReconcile(ordinaryReplicas)

				object := &appsv1.Deployment{}
				waitForCreatedObject(object)

				Expect(*object.Spec.Replicas).To(Equal(ordinaryReplicas))
				Expect(object.Spec.Template.Spec.Containers[0].Image).To(Equal(ordinaryImage))
			})

			It("Then the controller adds patching, finalizer, and controller reference to Deployment", func() {
				object := &appsv1.Deployment{}
				waitForCreatedObject(object)

				keys := maps.Keys(object.ObjectMeta.Annotations)
				Expect(keys).To(ContainElement(patchingName))
				Expect(object.ObjectMeta.Finalizers).To(ContainElement(finalizerName))
				Expect(object.ObjectMeta.OwnerReferences[0].Kind).To(Equal(soObjectKind))
			})

			It("Then the controller creates Service object", func() {
				object := &corev1.Service{}
				waitForCreatedObject(object)
			})

			It("Then the controller adds patching, finalizer, and controller reference to Service", func() {
				object := &corev1.Service{}
				waitForCreatedObject(object)

				keys := maps.Keys(object.ObjectMeta.Annotations)
				Expect(keys).To(ContainElement(patchingName))
				Expect(object.ObjectMeta.Finalizers).To(ContainElement(finalizerName))
				Expect(object.ObjectMeta.OwnerReferences[0].Kind).To(Equal(soObjectKind))
			})

			It("Then the controller creates Ingress object based on SimpleOperator object", func() {
				object := &networkingv1.Ingress{}
				waitForCreatedObject(object)

				Expect(object.Spec.TLS[0].Hosts).To(ContainElement(ordinaryHost))
				Expect(object.Spec.Rules[0].Host).To(Equal(ordinaryHost))
			})

			It("Then the controller adds patching, finalizer, and controller reference to Ingress", func() {
				object := &networkingv1.Ingress{}
				waitForCreatedObject(object)

				keys := maps.Keys(object.ObjectMeta.Annotations)
				Expect(keys).To(ContainElement(patchingName))
				Expect(object.ObjectMeta.Finalizers).To(ContainElement(finalizerName))
				Expect(object.ObjectMeta.OwnerReferences[0].Kind).To(Equal(soObjectKind))
			})
		})

		Context("When the controller is waiting for reconcilation", func() {
			It("Then the controller can reach the reconciled state", func() {
				doReconcile(ordinaryReplicas)
				waitForReconciledState(ordinaryReplicas)
			})
		})

		Context("When a new version of SimpleOperator object is applying", func() {

			BeforeEach(func() {
				doReconcile(ordinaryReplicas)
				waitForReconciledState(ordinaryReplicas)

				soObject := &sov1alpha1.SimpleOperator{}
				Expect(k8sClient.Get(ctx, soObjectKey, soObject)).Should(Succeed())

				soObject.Spec.Host = differentHost
				soObject.Spec.Image = differentImage
				soObject.Spec.Replicas = differentReplicas

				Expect(k8sClient.Update(ctx, soObject)).Should(Succeed())
			})

			It("Then the controller can reach the reconciled state again", func() {
				doReconcile(differentReplicas)
				soObject := waitForReconciledState(differentReplicas)

				Expect(soObject.Spec.Host).To(Equal(differentHost))
				Expect(soObject.Spec.Image).To(Equal(differentImage))
				Expect(soObject.Spec.Replicas).To(Equal(differentReplicas))

				object := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, createdObjectKey, object); err != nil {
					Expect(err.Error()).To(Equal(""))
				}

				Expect(object.Status.AvailableReplicas).To(Equal(differentReplicas))
			})
		})

		Context("When a deletion is happening on SimpleOperator object during reconciling", func() {

			// It("Then the controller deletes Deployment", func() {
			// 	deleteSimpleOperatorSafely()
			// 	object := &appsv1.Deployment{}
			// 	waitForDeletedOjbect(object)
			// })

			// It("Then the controller deletes Service", func() {
			// 	deleteSimpleOperatorSafely()
			// 	object := &corev1.Service{}
			// 	waitForDeletedOjbect(object)
			// })

			// It("Then the controller deletes Ingress", func() {
			// 	deleteSimpleOperatorSafely()
			// 	object := &networkingv1.Ingress{}
			// 	waitForDeletedOjbect(object)
			// })

			It("Then the controller allows to delete SimpleOperator object", func() {
				deleteSimpleOperatorSafely()
			})
		})

		Context("When a deletion is happening on SimpleOperator object after reconiliation", func() {
			It("Then the controller deletes Deployment", func() {
				doReconcile(ordinaryReplicas)
				waitForReconciledState(ordinaryReplicas)
				deleteSimpleOperatorSafely()
				object := &appsv1.Deployment{}
				waitForDeletedOjbect(object)
			})

			It("Then the controller deletes Service", func() {
				doReconcile(ordinaryReplicas)
				waitForReconciledState(ordinaryReplicas)
				deleteSimpleOperatorSafely()
				object := &corev1.Service{}
				waitForDeletedOjbect(object)
			})

			It("Then the controller deletes Ingress", func() {
				doReconcile(ordinaryReplicas)
				waitForReconciledState(ordinaryReplicas)
				deleteSimpleOperatorSafely()
				object := &networkingv1.Ingress{}
				waitForDeletedOjbect(object)
			})

			It("Then the controller allows to delete SimpleOperator object", func() {
				doReconcile(ordinaryReplicas)
				waitForReconciledState(ordinaryReplicas)
				deleteSimpleOperatorSafely()
			})
		})

		// TODO: missing functionality from controller
		// Context("When the objects were created before by unknown entity", func() {
		// 	// TODO: what shall the controller do in this case?
		// 	It("Then the controller ...")
		// })

		// Context("When the objects were created with finalizer before by unknown entity", func() {
		// 	// TODO: what shall the controller do in this case?
		// 	It("Then the controller ...")
		// })

		// Context("When the objects were created with controller reference before by unknown entity", func() {
		// 	// TODO: what shall the controller do in this case?
		// 	It("Then the controller ...")
		// })
	})

	// TODO: In BDD, the TCs cannot depend on each other; therefore, the controller shall stop and start again in some "Then"s.
	// This work involves some function calling from suite_test.go.
	// Describe("Given a marked for deletion SimpleOperator object", func() {

	// 	BeforeEach(func() {
	// 		soObject := createSimpleOperatorObject(objectName, objectNamespace, ordinaryHost, ordinaryImage, ordinaryReplicas)
	// 		soObject.ObjectMeta.Finalizers = append(soObject.ObjectMeta.Finalizers, finalizerName)
	// 		var gracePeriod int64 = 0
	// 		soObject.ObjectMeta.DeletionGracePeriodSeconds = &gracePeriod
	// 		deletionTimeStamp := metav1.Now()
	// 		soObject.ObjectMeta.DeletionTimestamp = &deletionTimeStamp
	// 		Expect(k8sClient.Create(ctx, soObject)).Should(Succeed())
	// 	})

	// 	Context("When there are not any created objects", func() {
	// 		It("Then controller allows to delete the SimpleOperator object", func() {
	// 			Eventually(func() string {
	// 				soObject := &sov1alpha1.SimpleOperator{}
	// 				if err := k8sClient.Get(ctx, soObjectKey, soObject); err != nil {
	// 					if errors.IsNotFound(err) {
	// 						return ""
	// 					}
	// 					return err.Error()
	// 				}

	// 				if err := k8sClient.Delete(ctx, soObject); err != nil {
	// 					return err.Error()
	// 				}

	// 				return "SimpleOperator object is not deleted yet"
	// 			}, timeout, interval).Should(Equal(""))
	// 		})
	// 	})

	// 	Context("When there are created objects", func() {

	// 		waitForDeletedObject := func(object client.Object) {
	// 			Eventually(func() string {
	// 				if err := k8sClient.Get(ctx, createdObjectKey, object); err != nil {
	// 					if errors.IsNotFound(err) {
	// 						return ""
	// 					}
	// 					return err.Error()
	// 				}
	// 				return "Object is not deleted yet"
	// 			}, timeout, interval).Should(Equal(""))
	// 		}

	// 		It("Then controller deletes Deployment object", func() {
	// 			doReconcile(ordinaryReplicas)
	// 			waitForReconciledState(ordinaryReplicas)

	// 			object := &appsv1.Deployment{}
	// 			waitForDeletedObject(object)
	// 		})

	// 		It("Then controller deletes Service object", func() {
	// 			doReconcile(ordinaryReplicas)
	// 			waitForReconciledState(ordinaryReplicas)

	// 			object := &corev1.Service{}
	// 			waitForDeletedObject(object)
	// 		})

	// 		It("Then controller deletes Ingress object", func() {
	// 			doReconcile(ordinaryReplicas)
	// 			waitForReconciledState(ordinaryReplicas)

	// 			object := &networkingv1.Ingress{}
	// 			waitForDeletedObject(object)
	// 		})

	// 		It("Then controller allows to delete SimpleOperator object", func() {
	// 			doReconcile(ordinaryReplicas)
	// 			waitForReconciledState(ordinaryReplicas)

	// 			Eventually(func() string {
	// 				soObject := &sov1alpha1.SimpleOperator{}
	// 				if err := k8sClient.Get(ctx, soObjectKey, soObject); err != nil {
	// 					if errors.IsNotFound(err) {
	// 						return ""
	// 					}
	// 					return err.Error()
	// 				}
	// 				return "The SimpleOperator object is not deleted"
	// 			}, timeout, interval).Should(Equal(""))
	// 		})
	// 	})
	// })

	Describe("Given a SimpleOperator object with missing keys (these have built-in default values)", func() {
		BeforeEach(func() {
			soObject := createSimpleOperatorObject(objectName, objectNamespace, ordinaryHost, ordinaryImage, noReplicas)
			Expect(k8sClient.Create(ctx, soObject)).Should(Succeed())
		})

		AfterEach(func() {
			deleteSimpleOperatorSafely()
		})

		Context("When the contoller is applying", func() {

			It("Then the controller fills the missing `replicas` with default values", func() {
				soObject := &sov1alpha1.SimpleOperator{}
				Eventually(func(soObject *sov1alpha1.SimpleOperator) string {
					if err := k8sClient.Get(ctx, soObjectKey, soObject); err != nil {
						return err.Error()
					}
					return ""
				}(soObject), timeout, interval).Should(Equal(""))

				Expect(soObject.Spec.Replicas).To(Equal(int32(1)))
			})
		})
	})
})
