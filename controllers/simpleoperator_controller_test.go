package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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
		duration = time.Second * 10
		interval = time.Millisecond * 250

		ordinaryHost  = "so.io"
		differentHost = "diff.so.io"

		ordinaryImage  = "hello-world"
		differentImage = "diff-hello-world"

		noReplicas              = 0
		ordinaryReplicas  int32 = 3
		differentReplicas       = 5
	)

	createdObjectKey := types.NamespacedName{Name: objectName, Namespace: "default"}
	soObjectKey := types.NamespacedName{Name: "simpleoperator-sample", Namespace: "default"}

	Describe("Given an already created simpleoperator object", func() {
		ctx := context.Background()

		BeforeEach(func() {
			soObject := createSimpleOperatorObject(objectName, objectNamespace, ordinaryHost, ordinaryImage, ordinaryReplicas)
			Expect(k8sClient.Create(ctx, soObject)).Should(Succeed())
		})

		AfterEach(func() {
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
				return ""
			}, timeout, interval).Should(Equal(""))
		})

		Context("When we are just right after the creation of simpleoperator object", func() {
			It("Then the controller creates Deployment object based on simpleoperator object", func() {

				object := &appsv1.Deployment{}

				Eventually(func() bool {
					err := k8sClient.Get(ctx, createdObjectKey, object)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				Expect(*object.Spec.Replicas).To(Equal(ordinaryReplicas))
				Expect(object.Spec.Template.Spec.Containers[0].Image).To(Equal(ordinaryImage))

			})
			It("Then the controller creates Service object", func() {

				Eventually(func() bool {
					object := &corev1.Service{}
					err := k8sClient.Get(ctx, createdObjectKey, object)
					return err == nil
				}, timeout, interval).Should(BeTrue())
			})
			It("Then the controller creates Ingress object based on simpleoperator object", func() {

				object := &networkingv1.Ingress{}

				Eventually(func() bool {

					err := k8sClient.Get(ctx, createdObjectKey, object)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				Expect(object.Spec.TLS[0].Hosts).To(ContainElement(ordinaryHost))
				Expect(object.Spec.Rules[0].Host).To(Equal(ordinaryHost))
			})
		})
	})
	Describe("When there is a marked for deletion simpleoperator object", func() {})
})

func createSimpleOperatorObject(objectName, objectNamespace, host, image string, replicas int32) *sov1alpha1.SimpleOperator {
	return &sov1alpha1.SimpleOperator{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "simpleoperator.szikes.io/v1alpha1",
			Kind:       "SimpleOperator",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simpleoperator-sample",
			Namespace: "default",
		},
		Spec: sov1alpha1.SimpleOperatorSpec{
			Host:     host,
			Image:    image,
			Replicas: replicas,
		},
	}
}
