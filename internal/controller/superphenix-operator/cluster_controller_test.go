package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

var _ = Describe("Cluster Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		cluster := &operatorv1alpha1.Cluster{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Cluster")
			err := k8sClient.Get(ctx, typeNamespacedName, cluster)
			if err != nil && errors.IsNotFound(err) {
				resource := &operatorv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: operatorv1alpha1.ClusterSpec{
						DeploymentMode:   operatorv1alpha1.DeploymentModeHyperconverged,
						Region:           "us-east-1",
						AvailabilityZone: "us-east-1a",
						Connection: &operatorv1alpha1.ClusterConnectionSpec{
							Mode: operatorv1alpha1.ConnectionModeLocal,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &operatorv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Cluster")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying that it's marked as reachable")
			updatedCluster := &operatorv1alpha1.Cluster{}
			err = k8sClient.Get(ctx, typeNamespacedName, updatedCluster)
			Expect(err).NotTo(HaveOccurred())

			var connectedCondition *metav1.Condition
			for i := range updatedCluster.Status.Conditions {
				if updatedCluster.Status.Conditions[i].Type == operatorv1alpha1.ConditionTypeConnected {
					connectedCondition = &updatedCluster.Status.Conditions[i]
					break
				}
			}
			Expect(connectedCondition).NotTo(BeNil())
			Expect(connectedCondition.Status).To(Equal(metav1.ConditionTrue))

			By("Creating a cluster with remote connection")
			mgmtClusterName := "remote-cluster"
			mgmtClusterNamespacedName := types.NamespacedName{
				Name:      mgmtClusterName,
				Namespace: "default",
			}
			mgmtCluster := &operatorv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mgmtClusterName,
					Namespace: "default",
				},
				Spec: operatorv1alpha1.ClusterSpec{
					DeploymentMode:   operatorv1alpha1.DeploymentModeHyperconverged,
					Region:           "us-east-1",
					AvailabilityZone: "us-east-1a",
					Connection: &operatorv1alpha1.ClusterConnectionSpec{
						Mode: operatorv1alpha1.ConnectionModeRemote,
						URL:  "https://remote-cluster:6443",
						SecretRef: &operatorv1alpha1.SecretReference{
							Name:      "some-secret",
							Namespace: "default",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mgmtCluster)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, mgmtCluster)
			}()

			By("Reconciling the remote cluster")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: mgmtClusterNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status reports SecretNotFound (as secret doesn't exist)")
			updatedMgmtCluster := &operatorv1alpha1.Cluster{}
			err = k8sClient.Get(ctx, mgmtClusterNamespacedName, updatedMgmtCluster)
			Expect(err).NotTo(HaveOccurred())

			var unreachableCondition *metav1.Condition
			for i := range updatedMgmtCluster.Status.Conditions {
				if updatedMgmtCluster.Status.Conditions[i].Type == operatorv1alpha1.ConditionTypeUnreachable {
					unreachableCondition = &updatedMgmtCluster.Status.Conditions[i]
					break
				}
			}
			Expect(unreachableCondition).NotTo(BeNil())
			Expect(unreachableCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(unreachableCondition.Reason).To(Equal(operatorv1alpha1.ReasonSecretNotFound))
		})

		It("should report ConnectionConfigError when connection is missing fields", func() {
			By("Creating a cluster with empty connection")
			resourceName := "nil-connection-cluster"
			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}
			cluster := &operatorv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: operatorv1alpha1.ClusterSpec{
					DeploymentMode:   operatorv1alpha1.DeploymentModeHyperconverged,
					Region:           "us-east-1",
					AvailabilityZone: "us-east-1a",
					Connection: &operatorv1alpha1.ClusterConnectionSpec{
						Mode: operatorv1alpha1.ConnectionModeRemote,
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cluster)
			}()

			By("Reconciling the cluster")
			controllerReconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status reports ConnectionConfigError")
			updatedCluster := &operatorv1alpha1.Cluster{}
			err = k8sClient.Get(ctx, typeNamespacedName, updatedCluster)
			Expect(err).NotTo(HaveOccurred())

			var unreachableCondition *metav1.Condition
			for i := range updatedCluster.Status.Conditions {
				if updatedCluster.Status.Conditions[i].Type == operatorv1alpha1.ConditionTypeUnreachable {
					unreachableCondition = &updatedCluster.Status.Conditions[i]
					break
				}
			}
			Expect(unreachableCondition).NotTo(BeNil())
			Expect(unreachableCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(unreachableCondition.Reason).To(Equal(operatorv1alpha1.ReasonConnectionConfigError))
		})

		It("should report SecretNotFound when secret is missing", func() {
			By("Creating a cluster with missing secret")
			resourceName := "missing-secret-cluster"
			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}
			cluster := &operatorv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: operatorv1alpha1.ClusterSpec{
					DeploymentMode:   operatorv1alpha1.DeploymentModeHyperconverged,
					Region:           "us-east-1",
					AvailabilityZone: "us-east-1a",
					Connection: &operatorv1alpha1.ClusterConnectionSpec{
						Mode: operatorv1alpha1.ConnectionModeRemote,
						URL:  "https://localhost:6443",
						SecretRef: &operatorv1alpha1.SecretReference{
							Name:      "non-existent-secret",
							Namespace: "default",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cluster)
			}()

			By("Reconciling the cluster")
			controllerReconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status reports ReasonSecretNotFound")
			updatedCluster := &operatorv1alpha1.Cluster{}
			err = k8sClient.Get(ctx, typeNamespacedName, updatedCluster)
			Expect(err).NotTo(HaveOccurred())

			var unreachableCondition *metav1.Condition
			for i := range updatedCluster.Status.Conditions {
				if updatedCluster.Status.Conditions[i].Type == operatorv1alpha1.ConditionTypeUnreachable {
					unreachableCondition = &updatedCluster.Status.Conditions[i]
					break
				}
			}
			Expect(unreachableCondition).NotTo(BeNil())
			Expect(unreachableCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(unreachableCondition.Reason).To(Equal(operatorv1alpha1.ReasonSecretNotFound))
		})

		It("should report InvalidSecret when secret content is missing", func() {
			By("Creating a secret without credentials")
			secretName := "invalid-secret"
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: "default",
				},
				Data: map[string][]byte{
					"some-other-key": []byte("value"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, secret)
			}()

			By("Creating a cluster with invalid secret")
			resourceName := "invalid-secret-cluster"
			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}
			cluster := &operatorv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: operatorv1alpha1.ClusterSpec{
					DeploymentMode:   operatorv1alpha1.DeploymentModeHyperconverged,
					Region:           "us-east-1",
					AvailabilityZone: "us-east-1a",
					Connection: &operatorv1alpha1.ClusterConnectionSpec{
						Mode: operatorv1alpha1.ConnectionModeRemote,
						URL:  "https://localhost:6443",
						SecretRef: &operatorv1alpha1.SecretReference{
							Name:      secretName,
							Namespace: "default",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cluster)
			}()

			By("Reconciling the cluster")
			controllerReconciler := &ClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status reports ReasonInvalidSecret")
			updatedCluster := &operatorv1alpha1.Cluster{}
			err = k8sClient.Get(ctx, typeNamespacedName, updatedCluster)
			Expect(err).NotTo(HaveOccurred())

			var unreachableCondition *metav1.Condition
			for i := range updatedCluster.Status.Conditions {
				if updatedCluster.Status.Conditions[i].Type == operatorv1alpha1.ConditionTypeUnreachable {
					unreachableCondition = &updatedCluster.Status.Conditions[i]
					break
				}
			}
			Expect(unreachableCondition).NotTo(BeNil())
			Expect(unreachableCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(unreachableCondition.Reason).To(Equal(operatorv1alpha1.ReasonInvalidSecret))
		})

		It("should add a finalizer to the Cluster", func() {
			cluster := &operatorv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, typeNamespacedName, cluster)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.Finalizers).To(ContainElement("operator.superphenix.net/finalizer"))
		})

	})
})
