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

package ipam

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math/big"
	"strconv"
	"sync"

	"github.com/iiiceoo/iprange"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	oapiv1 "github.com/iiiceoo/requeueip/oapi/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/net"
)

const (
	kindReplicaSet  = "ReplicaSet"
	kindDeployment  = "Deployment"
	kindStatefulSet = "StatefulSet"
)

var workloadSupports = []string{
	appsv1.SchemeGroupVersion.String() + "/" + kindReplicaSet,
	appsv1.SchemeGroupVersion.String() + "/" + kindDeployment,
	appsv1.SchemeGroupVersion.String() + "/" + kindStatefulSet,
}

type Allocator interface {
	// Get assigns IP addresses to Pod.
	Get(ctx context.Context, namespace, podName string, options *Options) ([]oapiv1.IPConfig, error)
}

// Options for IP allocation.
type Options struct {
	// Count of IPv4 addresses to be allocated.
	IPv4 int

	// Count of IPv6 addresses to be allocated.
	IPv6 int
}

func NewAllocator(c client.Client, reader client.Reader) Allocator {
	return &allocator{
		client: c,
		reader: reader,
	}
}

type allocator struct {
	client client.Client
	reader client.Reader
}

// ipAssignment contains the count of IPv4 and IPv6 addresses to be assigned
// as well as the slices of currently assigned IP addresses.
type ipAssignment struct {
	v4  int
	v6  int
	ips []oapiv1.IPConfig
}

func (a *allocator) Get(ctx context.Context, namespace, podName string, options *Options) ([]oapiv1.IPConfig, error) {
	// Do not consider creating Informer for Pod in DaemonSet as it would cost
	// a significant amount of memory.
	var pod corev1.Pod
	if err := a.reader.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      podName,
	}, &pod); err != nil {
		return nil, err
	}

	if !isPodAlive(&pod) {
		return nil, fmt.Errorf("dead Pod: %s/%s", pod.Namespace, pod.Name)
	}

	assignment, err := a.retrieveExistingIPs(ctx, &pod, options)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve the previous IP assignment: %v", err)
	}
	if assignment.v4 == 0 && assignment.v6 == 0 {
		return assignment.ips, nil
	}

	workload, err := a.getWorkload(ctx, &pod)
	if err != nil {
		return nil, fmt.Errorf("failed to get the top controller of Pod %s/%s: %v", pod.Namespace, pod.Name, err)
	}

	// TODO(iiiceoo): StatefulSet.
	ips, err := a.assign(ctx, &pod, workload, assignment)
	if err != nil {
		return nil, fmt.Errorf("failed to assign IP addresses: %v", err)
	}

	return ips, nil
}

// retrieveExistingIPs retrieves the previous IP assignment records.
func (a *allocator) retrieveExistingIPs(ctx context.Context, pod *corev1.Pod, options *Options) (*ipAssignment, error) {
	// It is reasonable to use IPs in the cache, as the interval between two
	// cmdAdd requests for the same Pod is completely sufficient for Informer
	// sync, which usually occurs when the sandbox is not successfully created.
	var riList requeueipv1.IPList
	if err := a.client.List(
		ctx,
		&riList,
		client.MatchingLabels{consts.LabelRefPodUID: string(pod.UID)},
		client.UnsafeDisableDeepCopy,
	); err != nil {
		return nil, err
	}

	v4, v6 := options.IPv4, options.IPv6
	ips := make([]oapiv1.IPConfig, 0, len(riList.Items))
	for i := 0; i < len(riList.Items); i++ {
		ri := &riList.Items[i]
		if !ri.DeletionTimestamp.IsZero() {
			continue
		}

		version, ok := ri.Labels[consts.LabelIPVersion]
		if !ok {
			return nil, fmt.Errorf("IP %s/%s without version label", ri.Namespace, ri.Name)
		}

		// If NameToCIDRIP does not return an error, then version must be valid.
		ip, err := net.NameToCIDRIP(version, ri.Name)
		if err != nil {
			return nil, err
		}
		if version == net.IPv4 {
			v4--
		} else {
			v6--
		}

		ips = append(ips, oapiv1.IPConfig{
			Address: ip.String(),
			Gateway: nil,
		})
	}

	if v4 < 0 || v6 < 0 {
		return nil, fmt.Errorf(
			"%d IPv4 and %d IPv6 addresses were expected to be assigned, but %d previous IP assignment records were found: %v",
			options.IPv4, options.IPv6,
			len(ips), ips,
		)
	}

	return &ipAssignment{
		v4:  v4,
		v6:  v6,
		ips: ips,
	}, nil
}

// getWorkload gets the supported top controller of Pod.
func (a *allocator) getWorkload(ctx context.Context, pod *corev1.Pod) (client.Object, error) {
	owner := metav1.GetControllerOf(pod)
	if owner == nil {
		return nil, errors.New("orphan Pod")
	}

	var errUnsupportedWorkload = errors.New("unsupported workload")
	gvk := owner.APIVersion + "/" + owner.Kind
	if !slices.Contains(workloadSupports, gvk) {
		return nil, fmt.Errorf("%w: %s", errUnsupportedWorkload, gvk)
	}

	var workload client.Object
	if owner.Kind == kindStatefulSet {
		var sts appsv1.StatefulSet
		if err := a.client.Get(ctx, types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      owner.Name,
		}, &sts); err != nil {
			return nil, err
		}
		workload = &sts
	}

	if owner.Kind == kindReplicaSet {
		var rs appsv1.ReplicaSet
		if err := a.client.Get(ctx, types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      owner.Name,
		}, &rs); err != nil {
			return nil, err
		}

		owner := metav1.GetControllerOf(&rs)
		if owner == nil {
			return nil, fmt.Errorf("%w: %s", errUnsupportedWorkload, gvk)
		}

		gvk = owner.APIVersion + "/" + owner.Kind
		if !slices.Contains(workloadSupports, gvk) {
			return nil, fmt.Errorf("%w: %s", errUnsupportedWorkload, gvk)
		}

		var deploy appsv1.Deployment
		if err := a.client.Get(ctx, types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      owner.Name,
		}, &deploy); err != nil {
			return nil, err
		}
		workload = &deploy
	}

	if !workload.GetDeletionTimestamp().IsZero() {
		return nil, fmt.Errorf("terminating %s %s/%s", gvk, workload.GetNamespace(), workload.GetName())
	}

	return workload, nil
}

// assign assigns IP addresses to Pod.
func (a *allocator) assign(
	ctx context.Context,
	pod *corev1.Pod,
	workload client.Object,
	assignment *ipAssignment,
) ([]oapiv1.IPConfig, error) {
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	ipsCh := make(chan []oapiv1.IPConfig, 2)
	if assignment.v4 > 0 {
		wg.Add(1)
		go func() {
			ips, err := a.assignIPs(ctx, net.IPv4, assignment.v4, pod, workload)
			if err != nil {
				errCh <- err
				return
			}
			ipsCh <- ips
		}()
	}
	if assignment.v6 > 0 {
		wg.Add(1)
		go func() {
			ips, err := a.assignIPs(ctx, net.IPv6, assignment.v6, pod, workload)
			if err != nil {
				errCh <- err
				return
			}
			ipsCh <- ips
		}()
	}

	wg.Wait()
	close(errCh)
	close(ipsCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) != 0 {
		return nil, utilerrors.NewAggregate(errs)
	}

	for ips := range ipsCh {
		assignment.ips = append(assignment.ips, ips...)
	}

	return assignment.ips, nil
}

// assignIPs assigns IP addresses of the specified version to Pod, which
// belong to the IPPool corresponding to the workload.
func (a *allocator) assignIPs(
	ctx context.Context,
	version string,
	count int,
	pod *corev1.Pod,
	workload client.Object,
) ([]oapiv1.IPConfig, error) {
	pool, err := a.waitForIPPoolReady(ctx, version, count, workload)
	if err != nil {
		gvk := workload.GetObjectKind().GroupVersionKind()
		return nil, fmt.Errorf(
			"failed to get %s IPPool for %s/%s %s/%s: %v",
			version,
			gvk.GroupVersion().String(),
			gvk.Kind,
			workload.GetNamespace(),
			workload.GetName(),
			err,
		)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, count)
	ipCh := make(chan *oapiv1.IPConfig, count)
	for i := 0; i < count; i++ {
		i := i
		wg.Add(1)
		go func() {
			ip, err := a.assignIP(ctx, pool, i, pod)
			if err != nil {
				errCh <- err
				return
			}
			ipCh <- ip
		}()
	}

	wg.Wait()
	close(errCh)
	close(ipCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) != 0 {
		return nil, fmt.Errorf("%s: %v", version, utilerrors.NewAggregate(errs))
	}

	ips := make([]oapiv1.IPConfig, 0, count)
	for ip := range ipCh {
		ips = append(ips, *ip)
	}

	return ips, nil
}

var errIPPoolNotReady = errors.New("IPPool is not ready")

// isIPPoolNotReady asserts whether the err is errIPPoolNotReady.
func isIPPoolNotReady(err error) bool {
	return errors.Is(err, errIPPoolNotReady)
}

// waitForIPPoolReady waits for IPPool to be ready until it can assign IP
// addresses for Pod.
func (a *allocator) waitForIPPoolReady(
	ctx context.Context,
	version string,
	count int,
	workload client.Object,
) (*requeueipv1.IPPool, error) {
	var rp *requeueipv1.IPPool
	if err := retry.OnError(retry.DefaultBackoff, isIPPoolNotReady, func() error {
		var rpList requeueipv1.IPPoolList
		if err := a.client.List(
			ctx,
			&rpList,
			client.MatchingLabels{
				consts.LabelIPVersion:      version,
				consts.LabelRefWorkloadUID: string(workload.GetUID()),
			},
			client.Limit(1),
			client.UnsafeDisableDeepCopy,
		); err != nil {
			return err
		}

		if len(rpList.Items) == 0 {
			return fmt.Errorf("%w: resource not found", errIPPoolNotReady)
		}

		rp = &rpList.Items[0]
		if rp.Status.Count == nil {
			return errIPPoolNotReady
		}

		free, err := strconv.Atoi(rp.Status.Count.Free)
		if err != nil {
			return err
		}
		if free < count {
			return fmt.Errorf(
				"%w: IPPool %s has insufficient available IP addresses; expected %d but found only %d",
				errIPPoolNotReady,
				rp.Name, count, free,
			)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return rp, nil
}

// assignIP assigns an IP address to Pod from IPPool.
func (a *allocator) assignIP(ctx context.Context, pool *requeueipv1.IPPool, num int, pod *corev1.Pod) (*oapiv1.IPConfig, error) {
	// version + Pod UID + num can uniquely identify an IP address.
	h := fnv.New32a()
	id := fmt.Sprintf("%s-%s-%d", pool.Spec.Version, pod.UID, num)
	h.Write([]byte(id))
	hash := h.Sum32()

	// Avoid creating IP obj repeatedly.
	ri := &requeueipv1.IP{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				consts.LabelIPVersion: pool.Spec.Version,
				consts.LabelRefIPPool: pool.Name,
				consts.LabelRefPodUID: string(pod.UID),
			},
		},
	}

	// Try to retry as much as possible, the cost of cmdAdd failure far outweighs
	// the cost of retry.
	if err := retry.OnError(retry.DefaultBackoff, apierrors.IsAlreadyExists, func() error {
		if err := a.client.Get(ctx, types.NamespacedName{Name: pool.Name}, pool); err != nil {
			return err
		}

		free, err := iprange.Parse(pool.Status.Free...)
		if err != nil {
			return err
		}

		index := new(big.Int).Mod(big.NewInt(int64(hash)), free.Size())
		index.Add(index, consts.BigInt[1])

		iter := free.IPIterator()
		ip := iter.NextN(index)
		ri.Name = net.IPToName(ip)
		if err := a.client.Create(ctx, ri); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return err
			}

			// Linear probing.
			ip = iter.Next()
			if ip == nil {
				iter.Reset()
				ip = iter.Next()
			}

			ri.Name = net.IPToName(ip)
			if err := a.client.Create(ctx, ri); err != nil {
				return fmt.Errorf("failed to get a non-conflicting IP address from IPPool %s: %v", pool.Name, err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	ip, err := net.NameToCIDRIP(pool.Spec.Version, ri.Name)
	if err != nil {
		return nil, err
	}

	return &oapiv1.IPConfig{
		Address: ip.String(),
		Gateway: nil,
	}, nil
}

// isPodAlive checks if Pod is alive.
func isPodAlive(pod *corev1.Pod) bool {
	if !pod.DeletionTimestamp.IsZero() {
		return false
	}
	if pod.Status.Phase == corev1.PodSucceeded &&
		pod.Spec.RestartPolicy != corev1.RestartPolicyAlways {
		return false
	}
	if pod.Status.Phase == corev1.PodFailed &&
		pod.Spec.RestartPolicy == corev1.RestartPolicyNever {
		return false
	}
	if pod.Status.Phase == corev1.PodFailed &&
		pod.Status.Reason == "Evicted" {
		return false
	}

	return true
}
