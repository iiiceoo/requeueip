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

	"github.com/olekukonko/tablewriter"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/net"
)

var _ = Describe("Performance", Label("perf"), func() {
	var e2eIndex int
	var prefix, suffix string
	var count uint32
	var v4Subnet, v6Subnet *requeueipv1.Subnet

	BeforeEach(func(ctx SpecContext) {
		e2eIndex = 2
		prefix = "perf"
		atomic.AddUint32(&count, 1)
		p := GinkgoParallelProcess()
		suffix = fmt.Sprintf("c%d-%d", p, count)
		offset := e2eIndex*16 + int(count)*2
		v4Subnet = &requeueipv1.Subnet{
			TypeMeta: metav1.TypeMeta{
				Kind:       consts.KindSubnet,
				APIVersion: consts.RAPIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-ipv4-subnet-%s", prefix, suffix),
			},
			Spec: requeueipv1.SubnetSpec{
				Version: ptr.To(net.IPv4),
				CIDR:    fmt.Sprintf("10.%d.%d.0/23", p, offset),
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
				Version: ptr.To(net.IPv6),
				CIDR:    fmt.Sprintf("fd00:%d:%d::/119", p, offset),
			},
		}
		By(fmt.Sprintf("Creating IPv6 Subnet %s with CIDR %s", v6Subnet.Name, v6Subnet.Spec.CIDR))
		err = c.Create(ctx, v6Subnet)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func(ctx SpecContext) {
		By(fmt.Sprintf("Deleting IPv4 Subnet %s", v4Subnet.Name))
		err := c.Delete(ctx, v4Subnet)
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("Deleting IPv6 Subnet %s", v6Subnet.Name))
		err = c.Delete(ctx, v6Subnet)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Deployment", Serial, func() {
		var selector map[string]string
		var deployT *appsv1.Deployment

		BeforeEach(func(ctx SpecContext) {
			selector = map[string]string{"e2e": prefix}
			deployT = &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       consts.KindDeployment,
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels:    selector,
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
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
		})

		AfterEach(func(ctx SpecContext) {
			By("Deleting Deployments for perf testing")
			err := c.DeleteAllOf(
				ctx,
				&appsv1.Deployment{},
				client.MatchingLabels(selector),
				client.InNamespace(deployT.Namespace),
			)
			Expect(err).NotTo(HaveOccurred())

			By("GCing IPPoolClaims and IPPools")
			gcTime := time.Now()
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{Name: v4Subnet.Name}, v4Subnet)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v4Subnet.Status.BlockCount).NotTo(BeNil())
				g.Expect(v4Subnet.Status.BlockCount.Used).To(Equal("0"))

				err = c.Get(ctx, types.NamespacedName{Name: v6Subnet.Name}, v6Subnet)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v6Subnet.Status.BlockCount).NotTo(BeNil())
				g.Expect(v6Subnet.Status.BlockCount.Used).To(Equal("0"))
			}).WithTimeout(5 * time.Minute).WithPolling(time.Second).WithContext(ctx).Should(Succeed())
			AddReportEntry("GC", time.Since(gcTime))
		})

		ReportAfterEach(func(report SpecReport) {
			if len(report.ReportEntries) == 0 {
				return
			}

			table := tablewriter.NewWriter(GinkgoWriter)
			table.SetHeader([]string{"Step", "Duration"})
			for _, e := range report.ReportEntries {
				table.Append([]string{e.Name, e.StringRepresentation()})
			}
			GinkgoWriter.Println("Perf Report:")
			table.Render()
		})

		It("with a large number of replicas", func(ctx SpecContext) {
			deployT.Name = fmt.Sprintf("%s-deploy-%s", prefix, suffix)
			v := fmt.Sprintf("%s-%s-%s", prefix, busybox, suffix)
			deployT.Spec.Selector.MatchLabels["run"] = v
			deployT.Spec.Template.Labels["run"] = v
			replicas := int32(200)
			deployT.Spec.Replicas = ptr.To(replicas)

			By(fmt.Sprintf("Creating Deployment %s/%s with %d replicas", deployT.Namespace, deployT.Name, replicas))
			err := c.Create(ctx, deployT)
			Expect(err).NotTo(HaveOccurred())

			By("Retrieving auto-created IPPools")
			retrieveTime := time.Now()
			var v4IPPool, v6IPPool *requeueipv1.IPPool
			Eventually(func(g Gomega) {
				var rpList requeueipv1.IPPoolList
				err := c.List(
					ctx,
					&rpList,
					client.MatchingLabels{
						consts.LabelIPVersion:      net.IPv4,
						consts.LabelRefWorkloadUID: string(deployT.UID),
					},
					client.InNamespace(deployT.Namespace),
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
						consts.LabelRefWorkloadUID: string(deployT.UID),
					},
					client.InNamespace(deployT.Namespace),
					client.UnsafeDisableDeepCopy,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rpList.Items).To(HaveLen(1))
				g.Expect(rpList.Items[0].Spec.Version).To(Equal(net.IPv6))
				v6IPPool = &rpList.Items[0]
			}).WithTimeout(3 * time.Second).WithPolling(100 * time.Millisecond).WithContext(ctx).Should(Succeed())
			AddReportEntry("IPPools Retrieved", time.Since(retrieveTime))

			By(fmt.Sprintf("Waiting for IPPool %s/%s (IPv4) and %s/%s (IPv6) to be ready",
				v4IPPool.Namespace, v4IPPool.Name,
				v6IPPool.Namespace, v6IPPool.Name,
			))
			scaleTime := time.Now()
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
			}).WithTimeout(3 * time.Second).WithPolling(100 * time.Millisecond).WithContext(ctx).Should(Succeed())
			AddReportEntry("IPPools Scaled", time.Since(scaleTime))

			By("Waiting IPAM")
			ipamTime := time.Now()
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: v4IPPool.Namespace,
					Name:      v4IPPool.Name,
				}, v4IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v4IPPool.Status.Count).NotTo(BeNil())
				g.Expect(v4IPPool.Status.Count.Used).To(Equal(strconv.Itoa(int(replicas))))

				err = c.Get(ctx, types.NamespacedName{
					Namespace: v6IPPool.Namespace,
					Name:      v6IPPool.Name,
				}, v6IPPool)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(v6IPPool.Status.Count).NotTo(BeNil())
				g.Expect(v6IPPool.Status.Count.Used).To(Equal(strconv.Itoa(int(replicas))))
			}).WithTimeout(2 * time.Minute).WithPolling(time.Second).WithContext(ctx).Should(Succeed())
			AddReportEntry("IPAM", time.Since(ipamTime))

			By(fmt.Sprintf("Waiting for Deployment %s/%s to be ready", deployT.Namespace, deployT.Name))
			Eventually(func(g Gomega) {
				err := c.Get(ctx, types.NamespacedName{
					Namespace: deployT.Namespace,
					Name:      deployT.Name,
				}, deployT)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(deployT.Status.ObservedGeneration).To(Equal(deployT.Generation))
				g.Expect(deployT.Status.UpdatedReplicas).To(Equal(replicas))
				g.Expect(deployT.Status.ReadyReplicas).To(Equal(replicas))
			}).WithTimeout(time.Minute).WithPolling(time.Second).WithContext(ctx).Should(Succeed())
			AddReportEntry("Workload Ready", time.Since(retrieveTime))
		})

		It("1:1 mapping of IP addresses", func(ctx SpecContext) {
			totalReplicas := 512
			replicas := int32(16)
			deployCount := totalReplicas / int(replicas)
			deploys := make([]*appsv1.Deployment, 0, deployCount)
			for i := 0; i < deployCount; i++ {
				deploy := deployT.DeepCopy()
				deploy.Name = fmt.Sprintf("%s-deploy-%s-%d", prefix, suffix, i)
				v := fmt.Sprintf("%s-%s-%s-%d", prefix, busybox, suffix, i)
				deploy.Spec.Selector.MatchLabels["run"] = v
				deploy.Spec.Template.Labels["run"] = v
				deploy.Spec.Replicas = ptr.To(replicas)
				deploys = append(deploys, deploy)
			}

			By(fmt.Sprintf("Creating %d Deployments with %d replicas", deployCount, replicas))
			for _, d := range deploys {
				err := c.Create(ctx, d)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Retrieving auto-created IPPools")
			retrieveTime := time.Now()
			Eventually(func(g Gomega) {
				var rpList requeueipv1.IPPoolList
				err := c.List(
					ctx,
					&rpList,
					client.InNamespace(deployT.Namespace),
					client.UnsafeDisableDeepCopy,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rpList.Items).To(HaveLen(deployCount * 2))
			}).WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).WithContext(ctx).Should(Succeed())
			AddReportEntry("IPPools Retrieved", time.Since(retrieveTime))

			By("Waiting for IPPools to be ready")
			scaleTime := time.Now()
			Eventually(func(g Gomega) {
				var rpList requeueipv1.IPPoolList
				err := c.List(
					ctx,
					&rpList,
					client.InNamespace(deployT.Namespace),
					client.UnsafeDisableDeepCopy,
				)
				g.Expect(err).NotTo(HaveOccurred())

				for i := 0; i < len(rpList.Items); i++ {
					rp := &rpList.Items[i]
					g.Expect(rp.Status.Count).NotTo(BeNil())
					g.Expect(rp.Status.Count.Total).To(Equal(strconv.Itoa(int(replicas))))
				}
			}).WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).WithContext(ctx).Should(Succeed())
			AddReportEntry("IPPools Scaled", time.Since(scaleTime))

			By("Waiting IPAM")
			ipamTime := time.Now()
			Eventually(func(g Gomega) {
				var rpList requeueipv1.IPPoolList
				err := c.List(
					ctx,
					&rpList,
					client.InNamespace(deployT.Namespace),
					client.UnsafeDisableDeepCopy,
				)
				g.Expect(err).NotTo(HaveOccurred())

				for i := 0; i < len(rpList.Items); i++ {
					rp := &rpList.Items[i]
					g.Expect(rp.Status.Count).NotTo(BeNil())
					g.Expect(rp.Status.Count.Used).To(Equal(strconv.Itoa(int(replicas))))
				}
			}).WithTimeout(5 * time.Minute).WithPolling(time.Second).WithContext(ctx).Should(Succeed())
			AddReportEntry("IPAM", time.Since(ipamTime))

			By("Waiting for Deployments to be ready")
			Eventually(func(g Gomega) {
				var deployList appsv1.DeploymentList
				err := c.List(
					ctx,
					&deployList,
					client.MatchingLabels(selector),
					client.InNamespace(deployT.Namespace),
					client.UnsafeDisableDeepCopy,
				)
				g.Expect(err).NotTo(HaveOccurred())

				for i := 0; i < len(deployList.Items); i++ {
					deploy := &deployList.Items[i]
					g.Expect(deploy.Status.ObservedGeneration).To(Equal(deploy.Generation))
					g.Expect(deploy.Status.UpdatedReplicas).To(Equal(replicas))
					g.Expect(deploy.Status.ReadyReplicas).To(Equal(replicas))
				}
			}).WithTimeout(time.Minute).WithPolling(time.Second).WithContext(ctx).Should(Succeed())
			AddReportEntry("Workloads Ready", time.Since(retrieveTime))
		})
	})
})
