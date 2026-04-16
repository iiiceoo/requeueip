/*
Copyright 2024 The RequeueIP Authors.

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

package e2e_test

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/net"
)

var _ = Describe("Scale", Label("common", "scale"), func() {
	var e2eIndex int
	var prefix, suffix string
	var count uint32
	var v4Subnet, v6Subnet *requeueipv1.Subnet
	var replicas int32
	var selector map[string]string

	BeforeEach(func(ctx SpecContext) {
		e2eIndex = 0
		prefix = "scale"
		atomic.AddUint32(&count, 1)
		p := GinkgoParallelProcess()
		suffix = fmt.Sprintf("c%d-%d", p, count)
		offset := e2eIndex*16 + int(count)
		v4Subnet = &requeueipv1.Subnet{
			TypeMeta: metav1.TypeMeta{
				Kind:       consts.KindSubnet,
				APIVersion: consts.RAPIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-ipv4-subnet-%s", prefix, suffix),
			},
			Spec: requeueipv1.SubnetSpec{
				Version:   ptr.To(net.IPv4),
				CIDR:      fmt.Sprintf("10.%d.%d.0/24", p, offset),
				BlockSize: ptr.To(int32(31)),
			},
		}
		By(fmt.Sprintf("Creating IPv4 Subnet %s with CIDR %s", v4Subnet.Name, v4Subnet.Spec.CIDR))
		err := c.Create(ctx, v4Subnet)
		Expect(err).NotTo(HaveOccurred())

		v6Subnet = &requeueipv1.Subnet{
			TypeMeta: metav1.TypeMeta{
				Kind:       consts.KindSubnet,
				APIVersion: consts.RAPIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-ipv6-subnet-%s", prefix, suffix),
			},
			Spec: requeueipv1.SubnetSpec{
				Version:   ptr.To(net.IPv6),
				CIDR:      fmt.Sprintf("fd00:%d:%d::/120", p, offset),
				BlockSize: ptr.To(int32(127)),
			},
		}
		By(fmt.Sprintf("Creating IPv6 Subnet %s with CIDR %s", v6Subnet.Name, v6Subnet.Spec.CIDR))
		err = c.Create(ctx, v6Subnet)
		Expect(err).NotTo(HaveOccurred())

		replicas = 2
		selector = map[string]string{"run": fmt.Sprintf("%s-%s-%s", prefix, busybox, suffix)}
	})

	AfterEach(func(ctx SpecContext) {
		By(fmt.Sprintf("Deleting IPv4 Subnet %s", v4Subnet.Name))
		err := c.Delete(ctx, v4Subnet)
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("Deleting IPv6 Subnet %s", v6Subnet.Name))
		err = c.Delete(ctx, v6Subnet)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("auto-created IPPool", func() {
		var sts *appsv1.StatefulSet
		var v4IPPool, v6IPPool *requeueipv1.IPPool

		BeforeEach(func(ctx SpecContext) {
			sts = &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind:       consts.KindDeployment,
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      fmt.Sprintf("%s-sts-%s", prefix, suffix),
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(replicas),
					Selector: &metav1.LabelSelector{
						MatchLabels: selector,
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								consts.AnnoMultusDefaultNetwork: "kube-system/calico-requeueip",
								consts.AnnoIPv4Subnets:          v4Subnet.Name,
								consts.AnnoIPv6Subnets:          v6Subnet.Name,
							},
							Labels: selector,
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:            busybox,
									Image:           busyboxImage,
									ImagePullPolicy: corev1.PullIfNotPresent,
									Command: []string{
										"/bin/sh",
										"-c",
										"trap : TERM INT; sleep infinity & wait",
									},
								},
							},
						},
					},
				},
			}
			By(fmt.Sprintf("Creating StatefulSet %s/%s", sts.Namespace, sts.Name))
			err := c.Create(ctx, sts)
			Expect(err).NotTo(HaveOccurred())

			By("Retrieving auto-created IPPools")
			Eventually(func(g Gomega) {
				var rpList requeueipv1.IPPoolList
				err := c.List(
					ctx,
					&rpList,
					client.MatchingLabels{
						consts.LabelIPVersion:      net.IPv4,
						consts.LabelRefWorkloadUID: string(sts.UID),
					},
					client.InNamespace(sts.Namespace),
					client.UnsafeDisableDeepCopy,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rpList.Items).To(HaveLen(1))
				g.Expect(rpList.Items[0].Spec.Version).To(Equal(net.IPv4))
				v4IPPool = &rpList.Items[0]

				err = c.List(
					ctx,
					&rpList,
					client.MatchingLabels{
						consts.LabelIPVersion:      net.IPv6,
						consts.LabelRefWorkloadUID: string(sts.UID),
					},
					client.InNamespace(sts.Namespace),
					client.UnsafeDisableDeepCopy,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rpList.Items).To(HaveLen(1))
				g.Expect(rpList.Items[0].Spec.Version).To(Equal(net.IPv6))
				v6IPPool = &rpList.Items[0]
			}).WithTimeout(3 * time.Second).WithPolling(500 * time.Millisecond).WithContext(ctx).Should(Succeed())

			By(fmt.Sprintf("Waiting for IPPool %s/%s (IPv4) and %s/%s (IPv6) to be ready",
				v4IPPool.Namespace, v4IPPool.Name,
				v6IPPool.Namespace, v6IPPool.Name,
			))
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: v4IPPool.Namespace,
					Name:      v4IPPool.Name,
				}, v4IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v4IPPool.Status.Count).NotTo(BeNil())
				g.Expect(v4IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(replicas))))

				err = c.Get(ctx, types.NamespacedName{
					Namespace: v6IPPool.Namespace,
					Name:      v6IPPool.Name,
				}, v6IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v6IPPool.Status.Count).NotTo(BeNil())
				g.Expect(v6IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(replicas))))
			}).WithTimeout(3 * time.Second).WithPolling(500 * time.Millisecond).WithContext(ctx).Should(Succeed())

			By(fmt.Sprintf("Waiting for StatefulSet %s/%s to be ready", sts.Namespace, sts.Name))
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: sts.Namespace,
					Name:      sts.Name,
				}, sts)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sts.Status.ObservedGeneration).To(Equal(sts.Generation))
				g.Expect(sts.Status.UpdatedReplicas).To(Equal(replicas))
				g.Expect(sts.Status.ReadyReplicas).To(Equal(replicas))
			}).WithTimeout(20 * time.Second).WithPolling(2 * time.Second).WithContext(ctx).Should(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			By(fmt.Sprintf("Deleting StatefulSet %s/%s", sts.Namespace, sts.Name))
			err := c.Delete(ctx, sts)
			Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())

			By("GCing IPPoolClaims and IPPools")
			Eventually(func(g Gomega) {
				var v4Claim requeueipv1.IPPoolClaim
				err := c.Get(ctx, types.NamespacedName{
					Namespace: v4IPPool.Namespace,
					Name:      v4IPPool.Name,
				}, &v4Claim)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

				err = c.Get(ctx, types.NamespacedName{
					Namespace: v4IPPool.Namespace,
					Name:      v4IPPool.Name,
				}, v4IPPool)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

				var v6Claim requeueipv1.IPPoolClaim
				err = c.Get(ctx, types.NamespacedName{
					Namespace: v6IPPool.Namespace,
					Name:      v6IPPool.Name,
				}, &v6Claim)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

				err = c.Get(ctx, types.NamespacedName{
					Namespace: v6IPPool.Namespace,
					Name:      v6IPPool.Name,
				}, v6IPPool)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(10 * time.Second).WithPolling(time.Second).WithContext(ctx).Should(Succeed())
		})

		It("up", func(ctx SpecContext) {
			target := replicas + 1
			By(fmt.Sprintf("Scaling up StatefulSet %s/%s to %d replicas", sts.Namespace, sts.Name, target))
			sts.Spec.Replicas = ptr.To(target)
			err := c.Update(ctx, sts)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Waiting for the size of IPPools to scale up to %d", target))
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: v4IPPool.Namespace,
					Name:      v4IPPool.Name,
				}, v4IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v4IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(target))))

				err = c.Get(ctx, types.NamespacedName{
					Namespace: v6IPPool.Namespace,
					Name:      v6IPPool.Name,
				}, v6IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v6IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(target))))
			}).WithTimeout(3 * time.Second).WithPolling(500 * time.Millisecond).WithContext(ctx).Should(Succeed())

			By(fmt.Sprintf("Waiting for StatefulSet %s/%s to scale up to %d replicas", sts.Namespace, sts.Name, target))
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: sts.Namespace,
					Name:      sts.Name,
				}, sts)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sts.Status.ObservedGeneration).To(Equal(sts.Generation))
				g.Expect(sts.Status.UpdatedReplicas).To(Equal(target))
				g.Expect(sts.Status.ReadyReplicas).To(Equal(target))
			}).WithTimeout(10 * time.Second).WithPolling(time.Second).WithContext(ctx).Should(Succeed())
		})

		It("down", func(ctx SpecContext) {
			target := replicas - 1
			By(fmt.Sprintf("Scaling down StatefulSet %s/%s to %d replicas", sts.Namespace, sts.Name, target))
			sts.Spec.Replicas = ptr.To(target)
			err := c.Update(ctx, sts)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Waiting for StatefulSet %s/%s to scale down to %d replicas", sts.Namespace, sts.Name, target))
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: sts.Namespace,
					Name:      sts.Name,
				}, sts)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sts.Status.ObservedGeneration).To(Equal(sts.Generation))
				g.Expect(sts.Status.UpdatedReplicas).To(Equal(target))
				g.Expect(sts.Status.ReadyReplicas).To(Equal(target))
			}).WithTimeout(10 * time.Second).WithPolling(time.Second).WithContext(ctx).Should(Succeed())

			By(fmt.Sprintf("Waiting for the size of IPPools to scale down to %d", target))
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: v4IPPool.Namespace,
					Name:      v4IPPool.Name,
				}, v4IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v4IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(target))))

				err = c.Get(ctx, types.NamespacedName{
					Namespace: v6IPPool.Namespace,
					Name:      v6IPPool.Name,
				}, v6IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v6IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(target))))
			}).WithTimeout(10 * time.Second).WithPolling(time.Second).WithContext(ctx).Should(Succeed())
		})
	})

	Describe("pre-requested IPPool", func() {
		var v4Claim, v6Claim *requeueipv1.IPPoolClaim
		var deploy *appsv1.Deployment
		var v4IPPool, v6IPPool requeueipv1.IPPool

		BeforeEach(func(ctx SpecContext) {
			v4Claim = &requeueipv1.IPPoolClaim{
				TypeMeta: metav1.TypeMeta{
					Kind:       consts.KindIPPoolClaim,
					APIVersion: consts.RAPIVersion,
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      fmt.Sprintf("%s-ipv4-ippool-%s", prefix, suffix),
				},
				Spec: requeueipv1.IPPoolClaimSpec{
					Version:  net.IPv4,
					Subnets:  []string{v4Subnet.Name},
					Replicas: replicas,
				},
			}
			By(fmt.Sprintf("Creating IPv4 IPPoolClaim %s/%s with subnets [%s] and replicas %d",
				v4Claim.Namespace, v4Claim.Name,
				v4Subnet.Name, replicas,
			))
			err := c.Create(ctx, v4Claim)
			Expect(err).NotTo(HaveOccurred())

			v6Claim = &requeueipv1.IPPoolClaim{
				TypeMeta: metav1.TypeMeta{
					Kind:       consts.KindIPPoolClaim,
					APIVersion: consts.RAPIVersion,
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      fmt.Sprintf("%s-ipv6-ippool-%s", prefix, suffix),
				},
				Spec: requeueipv1.IPPoolClaimSpec{
					Version:  net.IPv6,
					Subnets:  []string{v6Subnet.Name},
					Replicas: replicas,
				},
			}
			By(fmt.Sprintf("Creating IPv6 IPPoolClaim %s/%s with subnets [%s] and replicas %d",
				v6Claim.Namespace, v6Claim.Name,
				v6Subnet.Name, replicas,
			))
			err = c.Create(ctx, v6Claim)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Waiting for IPPool %s/%s (IPv4) and %s/%s (IPv6) to be ready",
				v4Claim.Namespace, v4Claim.Name,
				v6Claim.Namespace, v6Claim.Name,
			))
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: v4Claim.Namespace,
					Name:      v4Claim.Name,
				}, &v4IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v4IPPool.Status.Count).NotTo(BeNil())
				g.Expect(v4IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(replicas))))

				err = c.Get(ctx, types.NamespacedName{
					Namespace: v6Claim.Namespace,
					Name:      v6Claim.Name,
				}, &v6IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v6IPPool.Status.Count).NotTo(BeNil())
				g.Expect(v6IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(replicas))))
			}).WithTimeout(3 * time.Second).WithPolling(500 * time.Millisecond).WithContext(ctx).Should(Succeed())

			deploy = &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       consts.KindDeployment,
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      fmt.Sprintf("%s-deploy-%s", prefix, suffix),
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(replicas),
					Selector: &metav1.LabelSelector{
						MatchLabels: selector,
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								consts.AnnoMultusDefaultNetwork: "kube-system/calico-requeueip",
								consts.AnnoIPv4IPPools:          v4IPPool.Name,
								consts.AnnoIPv6IPPools:          v6IPPool.Name,
							},
							Labels: selector,
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:            busybox,
									Image:           busyboxImage,
									ImagePullPolicy: corev1.PullIfNotPresent,
									Command: []string{
										"/bin/sh",
										"-c",
										"trap : TERM INT; sleep infinity & wait",
									},
								},
							},
						},
					},
				},
			}
			By(fmt.Sprintf("Creating Deployment %s/%s", deploy.Namespace, deploy.Name))
			err = c.Create(ctx, deploy)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Waiting for Deployment %s/%s to be ready", deploy.Namespace, deploy.Name))
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: deploy.Namespace,
					Name:      deploy.Name,
				}, deploy)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(deploy.Status.ObservedGeneration).To(Equal(deploy.Generation))
				g.Expect(deploy.Status.UpdatedReplicas).To(Equal(replicas))
				g.Expect(deploy.Status.ReadyReplicas).To(Equal(replicas))
			}).WithTimeout(10 * time.Second).WithPolling(time.Second).WithContext(ctx).Should(Succeed())

			// Avoid conflicts during updates.
			err = c.Get(ctx, types.NamespacedName{
				Namespace: v4Claim.Namespace,
				Name:      v4Claim.Name,
			}, v4Claim)
			Expect(err).NotTo(HaveOccurred())

			err = c.Get(ctx, types.NamespacedName{
				Namespace: v6Claim.Namespace,
				Name:      v6Claim.Name,
			}, v6Claim)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func(ctx SpecContext) {
			By(fmt.Sprintf("Deleting Deployment %s/%s", deploy.Namespace, deploy.Name))
			err := c.Delete(ctx, deploy)
			Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())

			By(fmt.Sprintf("Deleting IPv4 IPPoolClaim %s/%s", v4Claim.Namespace, v4Claim.Name))
			err = c.Delete(ctx, v4Claim)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Deleting IPv6 IPPoolClaim %s/%s", v6Claim.Namespace, v6Claim.Name))
			err = c.Delete(ctx, v6Claim)
			Expect(err).NotTo(HaveOccurred())

			By("GCing IPPools")
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: v4IPPool.Namespace,
					Name:      v4IPPool.Name,
				}, &v4IPPool)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

				err = c.Get(ctx, types.NamespacedName{
					Namespace: v6IPPool.Namespace,
					Name:      v6IPPool.Name,
				}, &v6IPPool)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(10 * time.Second).WithPolling(time.Second).WithContext(ctx).Should(Succeed())
		})

		It("up", func(ctx SpecContext) {
			target := replicas + 1
			By(fmt.Sprintf("Scaling up IPPoolClaims to %d replicas", target))
			v4Claim.Spec.Replicas = target
			err := c.Update(ctx, v4Claim)
			Expect(err).NotTo(HaveOccurred())

			v6Claim.Spec.Replicas = target
			err = c.Update(ctx, v6Claim)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Waiting for the size of IPPools to scale up to %d", target))
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: v4IPPool.Namespace,
					Name:      v4IPPool.Name,
				}, &v4IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v4IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(target))))

				err = c.Get(ctx, types.NamespacedName{
					Namespace: v6IPPool.Namespace,
					Name:      v6IPPool.Name,
				}, &v6IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v6IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(target))))
			}).WithTimeout(3 * time.Second).WithPolling(500 * time.Millisecond).WithContext(ctx).Should(Succeed())
		})

		It("down", func(ctx SpecContext) {
			target := replicas - 1
			By(fmt.Sprintf("Scaling up IPPoolClaims to %d replicas", target))
			v4Claim.Spec.Replicas = target
			err := c.Update(ctx, v4Claim)
			Expect(err).NotTo(HaveOccurred())

			v6Claim.Spec.Replicas = target
			err = c.Update(ctx, v6Claim)
			Expect(err).NotTo(HaveOccurred())

			By("Checking that IPPools will not scale down until there are enough IP addresses available for use")
			Consistently(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: v4IPPool.Namespace,
					Name:      v4IPPool.Name,
				}, &v4IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v4IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(replicas))))

				err = c.Get(ctx, types.NamespacedName{
					Namespace: v6IPPool.Namespace,
					Name:      v6IPPool.Name,
				}, &v6IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v4IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(replicas))))
			}).WithTimeout(time.Second).WithPolling(500 * time.Millisecond).WithContext(ctx).Should(Succeed())

			By(fmt.Sprintf("Scaling down Deployment %s/%s to %d replicas", deploy.Namespace, deploy.Name, target))
			deploy.Spec.Replicas = ptr.To(target)
			err = c.Update(ctx, deploy)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Waiting for the size of IPPools to scale down to %d", target))
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: v4IPPool.Namespace,
					Name:      v4IPPool.Name,
				}, &v4IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v4IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(target))))

				err = c.Get(ctx, types.NamespacedName{
					Namespace: v6IPPool.Namespace,
					Name:      v6IPPool.Name,
				}, &v6IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v6IPPool.Status.Count.Total).To(Equal(strconv.Itoa(int(target))))
			}).WithTimeout(10 * time.Second).WithPolling(time.Second).WithContext(ctx).Should(Succeed())
		})
	})
})
