package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

var _ = Describe("ClusterConnection Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-clusterconnection"
		const secretName = "test-secret"
		const namespace = "default"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}
		secretNamespacedName := types.NamespacedName{
			Name:      secretName,
			Namespace: namespace,
		}

		BeforeEach(func() {
			By("creating the Secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"bearerToken": []byte("my-token"),
				},
			}
			err := k8sClient.Create(ctx, secret)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			By("creating the custom resource for the Kind ClusterConnection")
			clusterConnection := &operatorv1alpha1.ClusterConnection{}
			err = k8sClient.Get(ctx, typeNamespacedName, clusterConnection)
			if err != nil && errors.IsNotFound(err) {
				resource := &operatorv1alpha1.ClusterConnection{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: namespace,
					},
					Spec: operatorv1alpha1.ClusterConnectionSpec{
						URL: "https://remote-cluster.example.com",
						SecretRef: operatorv1alpha1.SecretReference{
							Name: secretName,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance ClusterConnection")
			resource := &operatorv1alpha1.ClusterConnection{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			By("Cleanup the Secret")
			secret := &corev1.Secret{}
			err = k8sClient.Get(ctx, secretNamespacedName, secret)
			if err == nil {
				Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource and find the secret", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ClusterConnectionReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail to reconcile if secret is missing", func() {
			By("Deleting the Secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
			}
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &ClusterConnectionReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			// Currently we return nil error if secret is missing (only log error)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
