package controller

import (
	mariadbv1alpha1 "github.com/mariadb-operator/mariadb-operator/api/v1alpha1"
	jobpkg "github.com/mariadb-operator/mariadb-operator/pkg/job"
	"github.com/mariadb-operator/mariadb-operator/pkg/statefulset"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("MariaDB Galera", func() {
	Context("When creating a MariaDB Galera", func() {
		It("Should default", func() {
			By("Creating MariaDB")
			testDefaultMariaDb := mariadbv1alpha1.MariaDB{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-mariadb-galera-default",
					Namespace: testNamespace,
				},
				Spec: mariadbv1alpha1.MariaDBSpec{
					Galera: &mariadbv1alpha1.Galera{
						Enabled: true,
					},
					Replicas: 3,
					Storage: mariadbv1alpha1.Storage{
						Size: ptr.To(resource.MustParse("300Mi")),
					},
				},
			}
			Expect(k8sClient.Create(testCtx, &testDefaultMariaDb)).To(Succeed())
			DeferCleanup(func() {
				deleteMariaDB(&testDefaultMariaDb)
			})

			By("Expecting to eventually default")
			Eventually(func(g Gomega) bool {
				if err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(&testDefaultMariaDb), &testDefaultMariaDb); err != nil {
					return false
				}
				g.Expect(testDefaultMariaDb.Spec.Galera).ToNot(BeZero())
				g.Expect(testDefaultMariaDb.Spec.Galera.Primary).ToNot(BeZero())
				g.Expect(testDefaultMariaDb.Spec.Galera.SST).ToNot(BeZero())
				g.Expect(testDefaultMariaDb.Spec.Galera.ReplicaThreads).ToNot(BeZero())
				g.Expect(testDefaultMariaDb.Spec.Galera.Recovery).ToNot(BeZero())
				g.Expect(testDefaultMariaDb.Spec.Galera.InitContainer).ToNot(BeZero())
				g.Expect(testDefaultMariaDb.Spec.Galera.Config).ToNot(BeZero())
				return true
			}, testTimeout, testInterval).Should(BeTrue())
		})

		galeraKey := types.NamespacedName{
			Name:      "mariadb-galera",
			Namespace: testNamespace,
		}

		It("Should reconcile", func() {
			mdb := mariadbv1alpha1.MariaDB{
				ObjectMeta: metav1.ObjectMeta{
					Name:      galeraKey.Name,
					Namespace: galeraKey.Namespace,
				},
				Spec: mariadbv1alpha1.MariaDBSpec{
					Username: &testUser,
					PasswordSecretKeyRef: &mariadbv1alpha1.GeneratedSecretKeyRef{
						SecretKeySelector: corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: testPwdKey.Name,
							},
							Key: testPwdSecretKey,
						},
					},
					Database: &testDatabase,
					MyCnf: ptr.To(`[mariadb]
					bind-address=*
					default_storage_engine=InnoDB
					binlog_format=row
					innodb_autoinc_lock_mode=2
					max_allowed_packet=256M
					`),
					Galera: &mariadbv1alpha1.Galera{
						Enabled: true,
						GaleraSpec: mariadbv1alpha1.GaleraSpec{
							Primary: mariadbv1alpha1.PrimaryGalera{
								PodIndex:          ptr.To(0),
								AutomaticFailover: ptr.To(true),
							},
							Recovery: &mariadbv1alpha1.GaleraRecovery{
								Enabled: true,
							},
							Config: mariadbv1alpha1.GaleraConfig{
								ReuseStorageVolume: ptr.To(false),
								VolumeClaimTemplate: &mariadbv1alpha1.VolumeClaimTemplate{
									PersistentVolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
										Resources: corev1.VolumeResourceRequirements{
											Requests: corev1.ResourceList{
												"storage": resource.MustParse("100Mi"),
											},
										},
										AccessModes: []corev1.PersistentVolumeAccessMode{
											corev1.ReadWriteOnce,
										},
									},
								},
							},
							InitJob: &mariadbv1alpha1.Job{
								Metadata: &mariadbv1alpha1.Metadata{
									Labels: map[string]string{
										"sidecar.istio.io/inject": "false",
									},
								},
							},
						},
					},
					Replicas: 3,
					Storage: mariadbv1alpha1.Storage{
						Size:                ptr.To(resource.MustParse("300Mi")),
						StorageClassName:    "standard-resize",
						ResizeInUseVolumes:  ptr.To(true),
						WaitForVolumeResize: ptr.To(true),
					},
					Service: &mariadbv1alpha1.ServiceTemplate{
						Type: corev1.ServiceTypeLoadBalancer,
						Metadata: &mariadbv1alpha1.Metadata{
							Annotations: map[string]string{
								"metallb.universe.tf/loadBalancerIPs": testCidrPrefix + ".0.150",
							},
						},
					},
					Connection: &mariadbv1alpha1.ConnectionTemplate{
						SecretName: ptr.To("mdb-galera-conn"),
						SecretTemplate: &mariadbv1alpha1.SecretTemplate{
							Key: &testConnSecretKey,
						},
					},
					PrimaryService: &mariadbv1alpha1.ServiceTemplate{
						Type: corev1.ServiceTypeLoadBalancer,
						Metadata: &mariadbv1alpha1.Metadata{
							Annotations: map[string]string{
								"metallb.universe.tf/loadBalancerIPs": testCidrPrefix + ".0.160",
							},
						},
					},
					PrimaryConnection: &mariadbv1alpha1.ConnectionTemplate{
						SecretName: ptr.To("mdb-galera-conn-primary"),
						SecretTemplate: &mariadbv1alpha1.SecretTemplate{
							Key: &testConnSecretKey,
						},
					},
					SecondaryService: &mariadbv1alpha1.ServiceTemplate{
						Type: corev1.ServiceTypeLoadBalancer,
						Metadata: &mariadbv1alpha1.Metadata{
							Annotations: map[string]string{
								"metallb.universe.tf/loadBalancerIPs": testCidrPrefix + ".0.161",
							},
						},
					},
					SecondaryConnection: &mariadbv1alpha1.ConnectionTemplate{
						SecretName: ptr.To("mdb-galera-conn-secondary"),
						SecretTemplate: &mariadbv1alpha1.SecretTemplate{
							Key: &testConnSecretKey,
						},
					},
					UpdateStrategy: mariadbv1alpha1.UpdateStrategy{
						Type: mariadbv1alpha1.ReplicasFirstPrimaryLast,
					},
				},
			}

			By("Creating MariaDB Galera")
			Expect(k8sClient.Create(testCtx, &mdb)).To(Succeed())
			DeferCleanup(func() {
				deleteMariaDB(&mdb)
			})

			By("Expecting init Job to eventually be completed")
			Eventually(func(g Gomega) bool {
				var job batchv1.Job
				g.Expect(k8sClient.Get(testCtx, mdb.InitKey(), &job)).To(Succeed())

				g.Expect(job.ObjectMeta.Labels).NotTo(BeNil())
				g.Expect(job.ObjectMeta.Labels).To(HaveKeyWithValue("sidecar.istio.io/inject", "false"))

				return jobpkg.IsJobComplete(&job)
			}, testHighTimeout, testInterval).Should(BeTrue())

			By("Expecting MariaDB to be ready eventually")
			Eventually(func() bool {
				if err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(&mdb), &mdb); err != nil {
					return false
				}
				return mdb.IsReady() && mdb.HasGaleraConfiguredCondition() && mdb.HasGaleraReadyCondition()
			}, testHighTimeout, testInterval).Should(BeTrue())

			By("Expecting to create a StatefulSet")
			var sts appsv1.StatefulSet
			Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(&mdb), &sts)).To(Succeed())

			By("Expecting to create a Service")
			var svc corev1.Service
			Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(&mdb), &svc)).To(Succeed())

			By("Expecting to create a primary Service")
			Expect(k8sClient.Get(testCtx, mdb.PrimaryServiceKey(), &svc)).To(Succeed())
			Expect(svc.Spec.Selector["statefulset.kubernetes.io/pod-name"]).To(Equal(statefulset.PodName(mdb.ObjectMeta, 0)))

			By("Expecting to create a secondary Service")
			Expect(k8sClient.Get(testCtx, mdb.SecondaryServiceKey(), &svc)).To(Succeed())

			By("Expecting Connection to be ready eventually")
			Eventually(func() bool {
				var conn mariadbv1alpha1.Connection
				if err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(&mdb), &conn); err != nil {
					return false
				}
				return conn.IsReady()
			}, testTimeout, testInterval).Should(BeTrue())

			By("Expecting primary Connection to be ready eventually")
			Eventually(func() bool {
				var conn mariadbv1alpha1.Connection
				if err := k8sClient.Get(testCtx, mdb.PrimaryConnectioneKey(), &conn); err != nil {
					return false
				}
				return conn.IsReady()
			}, testTimeout, testInterval).Should(BeTrue())

			By("Expecting secondary Connection to be ready eventually")
			Eventually(func() bool {
				var conn mariadbv1alpha1.Connection
				if err := k8sClient.Get(testCtx, mdb.SecondaryConnectioneKey(), &conn); err != nil {
					return false
				}
				return conn.IsReady()
			}, testTimeout, testInterval).Should(BeTrue())

			By("Expecting to create secondary Endpoints")
			var endpoints corev1.Endpoints
			Expect(k8sClient.Get(testCtx, mdb.SecondaryServiceKey(), &endpoints)).To(Succeed())
			Expect(endpoints.Subsets).To(HaveLen(1))
			Expect(endpoints.Subsets[0].Addresses).To(HaveLen(int(mdb.Spec.Replicas) - 1))

			By("Expecting to create a PodDisruptionBudget")
			var pdb policyv1.PodDisruptionBudget
			Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(&mdb), &pdb)).To(Succeed())

			By("Updating MariaDB primary")
			podIndex := 1
			mdb.Spec.Galera.Primary.PodIndex = ptr.To(podIndex)
			Expect(k8sClient.Update(testCtx, &mdb)).To(Succeed())

			By("Expecting MariaDB to eventually change primary")
			Eventually(func() bool {
				if err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(&mdb), &mdb); err != nil {
					return false
				}
				if !mdb.IsReady() || mdb.Status.CurrentPrimaryPodIndex == nil {
					return false
				}
				return *mdb.Status.CurrentPrimaryPodIndex == podIndex
			}, testTimeout, testInterval).Should(BeTrue())

			By("Expecting primary Service to eventually change primary")
			Eventually(func() bool {
				var svc corev1.Service
				if err := k8sClient.Get(testCtx, mdb.PrimaryServiceKey(), &svc); err != nil {
					return false
				}
				return svc.Spec.Selector["statefulset.kubernetes.io/pod-name"] == statefulset.PodName(mdb.ObjectMeta, podIndex)
			}, testTimeout, testInterval).Should(BeTrue())

			By("Recovering Galera cluster after crash")
			testGaleraClusterRecovery(&mdb)

			By("Updating MariaDB")
			testMariadbUpdate(&mdb, "500m")

			By("Resizing MariaDB PVCs")
			testMariadbVolumeResize(&mdb, "400Mi")

			mxsKey := types.NamespacedName{
				Name:      "maxscale-galera",
				Namespace: testNamespace,
			}
			By("Using MariaDB with MaxScale")
			testMariadbMaxscale(&mdb, mxsKey)
		})
	})
})

func testGaleraClusterRecovery(mdb *mariadbv1alpha1.MariaDB) {
	key := client.ObjectKeyFromObject(mdb)
	By("Tearing down all Pods consistently")
	opts := []client.DeleteAllOfOption{
		client.MatchingLabels{
			"app.kubernetes.io/instance": mdb.Name,
		},
		client.InNamespace(mdb.Namespace),
	}
	Expect(k8sClient.DeleteAllOf(testCtx, &corev1.Pod{}, opts...)).To(Succeed())

	By("Expecting MariaDB NOT to be ready eventually")
	Eventually(func() bool {
		if err := k8sClient.Get(testCtx, key, mdb); err != nil {
			return false
		}
		return !mdb.IsReady()
	}, testHighTimeout, testInterval).Should(BeTrue())

	By("Expecting Galera NOT to be ready eventually")
	Eventually(func() bool {
		if err := k8sClient.Get(testCtx, key, mdb); err != nil {
			return false
		}
		return mdb.HasGaleraNotReadyCondition()
	}, testHighTimeout, testInterval).Should(BeTrue())

	By("Expecting MariaDB to be ready eventually")
	Eventually(func() bool {
		if err := k8sClient.Get(testCtx, key, mdb); err != nil {
			return false
		}
		return mdb.IsReady() && mdb.HasGaleraReadyCondition()
	}, testVeryHighTimeout, testInterval).Should(BeTrue())

	By("Expecting Connection to be ready eventually")
	Eventually(func() bool {
		var conn mariadbv1alpha1.Connection
		if err := k8sClient.Get(testCtx, key, &conn); err != nil {
			return false
		}
		return conn.IsReady()
	}, testTimeout, testInterval).Should(BeTrue())

	By("Expecting primary Connection to be ready eventually")
	Eventually(func() bool {
		var conn mariadbv1alpha1.Connection
		if err := k8sClient.Get(testCtx, mdb.PrimaryConnectioneKey(), &conn); err != nil {
			return false
		}
		return conn.IsReady()
	}, testTimeout, testInterval).Should(BeTrue())
}
