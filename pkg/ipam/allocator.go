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
	"time"

	"github.com/iiiceoo/iprange"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	oapiv1 "github.com/iiiceoo/requeueip/oapi/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/net"
)

var workloadSupports = []string{
	appsv1.SchemeGroupVersion.String() + "/" + consts.KindReplicaSet,
	appsv1.SchemeGroupVersion.String() + "/" + consts.KindDeployment,
	appsv1.SchemeGroupVersion.String() + "/" + consts.KindStatefulSet,
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
	v4ToAssign int
	v6ToAssign int
	ips        []oapiv1.IPConfig
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

	workload, err := a.getWorkload(ctx, &pod)
	if err != nil {
		return nil, fmt.Errorf("failed to get the top controller of Pod %s/%s: %v", pod.Namespace, pod.Name, err)
	}

	assignment, err := a.retrieveExistingIPs(ctx, &pod, workload, options)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve the previous IP assignment: %v", err)
	}
	if assignment.v4ToAssign == 0 && assignment.v6ToAssign == 0 {
		return assignment.ips, nil
	}

	ips, err := a.assign(ctx, &pod, workload, assignment)
	if err != nil {
		return nil, fmt.Errorf("failed to assign IP addresses: %v", err)
	}

	return ips, nil
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
		return nil, fmt.Errorf("%v: %s", errUnsupportedWorkload, gvk)
	}

	var workload client.Object
	if owner.Kind == consts.KindStatefulSet {
		var sts appsv1.StatefulSet
		if err := a.client.Get(ctx, types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      owner.Name,
		}, &sts); err != nil {
			return nil, err
		}
		workload = &sts
	}

	if owner.Kind == consts.KindReplicaSet {
		var rs appsv1.ReplicaSet
		if err := a.client.Get(ctx, types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      owner.Name,
		}, &rs); err != nil {
			return nil, err
		}

		owner := metav1.GetControllerOf(&rs)
		if owner == nil {
			return nil, fmt.Errorf("%v: %s", errUnsupportedWorkload, gvk)
		}

		gvk = owner.APIVersion + "/" + owner.Kind
		if !slices.Contains(workloadSupports, gvk) {
			return nil, fmt.Errorf("%v: %s", errUnsupportedWorkload, gvk)
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

// retrieveExistingIPs retrieves the previous IP assignment records.
func (a *allocator) retrieveExistingIPs(
	ctx context.Context,
	pod *corev1.Pod,
	workload client.Object,
	options *Options,
) (*ipAssignment, error) {
	labels := map[string]string{}
	if workload.GetObjectKind().GroupVersionKind().Kind == consts.KindStatefulSet {
		labels[consts.LabelRefSTSUID] = string(workload.GetUID())
		labels[consts.LabelRefPod] = pod.Name
	} else {
		labels[consts.LabelRefPodUID] = string(pod.UID)
	}

	// It is reasonable to use IPs in the cache, as the interval between two
	// cmdAdd requests for the same Pod is completely sufficient for Informer
	// sync, which usually occurs when the sandbox is not successfully created.
	var riList requeueipv1.IPList
	if err := a.client.List(
		ctx,
		&riList,
		client.MatchingLabels(labels),
		client.InNamespace(pod.Namespace),
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
			"excepted to assign %d IPv4 and %d IPv6 addresses, but %d previous IP records were found: %v",
			options.IPv4,
			options.IPv6,
			len(ips),
			ips,
		)
	}

	return &ipAssignment{
		v4ToAssign: v4,
		v6ToAssign: v6,
		ips:        ips,
	}, nil
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
	if assignment.v4ToAssign > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ips, err := a.assignIPs(ctx, net.IPv4, assignment.v4ToAssign, pod, workload)
			if err != nil {
				errCh <- err
				return
			}
			ipsCh <- ips
		}()
	}
	if assignment.v6ToAssign > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ips, err := a.assignIPs(ctx, net.IPv6, assignment.v6ToAssign, pod, workload)
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
	pool, err := a.selectIPPool(ctx, version, count, pod, workload)
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	errCh := make(chan error, count)
	ipCh := make(chan *oapiv1.IPConfig, count)
	for i := 0; i < count; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			ip, err := a.assignIP(ctx, pool, i, pod, workload)
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

// selectIPPool gets the IPPool selected by the selection rules specified by Pod
// annotations.
func (a *allocator) selectIPPool(
	ctx context.Context,
	version string,
	count int,
	pod *corev1.Pod,
	workload client.Object,
) (*requeueipv1.IPPool, error) {
	ap, as := consts.AnnoIPv4IPPools, consts.AnnoIPv4Subnets
	if version == net.IPv6 {
		ap, as = consts.AnnoIPv6IPPools, consts.AnnoIPv6Subnets
	}

	// Currently only supports specifying a single IPPool.
	poolName, ok := pod.Annotations[ap]
	if ok {
		pool, err := a.waitForIPPoolReady(ctx, version, pod.Namespace, poolName, count)
		if err != nil {
			return nil, fmt.Errorf("failed to get the specified %s IPPool %s: %v", version, poolName, err)
		}
		return pool, nil
	}

	// Do not perform excessive validation on subnets value here, it should be the
	// responsibility of the controller.
	_, ok = pod.Annotations[as]
	if !ok {
		return nil, errors.New("no IPPool selection rule is specified")
	}

	pool, err := a.waitForAutoIPPoolReady(ctx, version, count, workload)
	if err != nil {
		gvk := workload.GetObjectKind().GroupVersionKind()
		return nil, fmt.Errorf(
			"failed to get auto-created %s IPPool for %s/%s %s/%s: %v",
			version,
			gvk.GroupVersion().String(),
			gvk.Kind,
			workload.GetNamespace(),
			workload.GetName(),
			err,
		)
	}

	return pool, nil
}

var backoff = wait.Backoff{
	Steps:    6,
	Duration: 10 * time.Millisecond,
	Factor:   5.0,
	Jitter:   0.1,
}

var (
	errUnreadyIPPool   = errors.New("unready IPPool")
	errInsufficientIPs = errors.New("available IP addresses are insufficient")
)

// isUnreadyIPPool asserts whether the err is errUnreadyIPPool.
func isUnreadyIPPool(err error) bool {
	return errors.Is(err, errUnreadyIPPool)
}

// waitForIPPoolReady waits for IPPool to be ready until it can assign IP
// addresses for Pod.
func (a *allocator) waitForIPPoolReady(
	ctx context.Context,
	version, namespace, name string,
	count int,
) (*requeueipv1.IPPool, error) {
	var rp requeueipv1.IPPool
	if err := retry.OnError(backoff, isUnreadyIPPool, func() error {
		if err := a.client.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, &rp); err != nil {
			return err
		}

		if rp.Spec.Version != version {
			return fmt.Errorf("try to assign %s addresses from %s IPPool %s", version, rp.Spec.Version, rp.Name)
		}

		if rp.Status.Count == nil {
			return errUnreadyIPPool
		}

		free, err := strconv.Atoi(rp.Status.Count.Free)
		if err != nil {
			return err
		}
		if free < count {
			return fmt.Errorf("%w: %w, expected %d but found only %d", errUnreadyIPPool, errInsufficientIPs, count, free)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &rp, nil
}

// waitForAutoIPPoolReady waits for auto-created IPPool to be ready until it can
// assign IP addresses for Pod.
func (a *allocator) waitForAutoIPPoolReady(
	ctx context.Context,
	version string,
	count int,
	workload client.Object,
) (*requeueipv1.IPPool, error) {
	var rp *requeueipv1.IPPool
	if err := retry.OnError(backoff, isUnreadyIPPool, func() error {
		var rpList requeueipv1.IPPoolList
		if err := a.client.List(
			ctx,
			&rpList,
			client.MatchingLabels{
				consts.LabelIPVersion:      version,
				consts.LabelRefWorkloadUID: string(workload.GetUID()),
			},
			client.InNamespace(workload.GetNamespace()),
			client.Limit(1),
			client.UnsafeDisableDeepCopy,
		); err != nil {
			return err
		}

		if len(rpList.Items) == 0 {
			return errUnreadyIPPool
		}

		rp = &rpList.Items[0]
		if rp.Status.Count == nil {
			return errUnreadyIPPool
		}

		free, err := strconv.Atoi(rp.Status.Count.Free)
		if err != nil {
			return err
		}
		if free < count {
			return fmt.Errorf("%w: %w, expected %d but found only %d", errUnreadyIPPool, errInsufficientIPs, count, free)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return rp, nil
}

// isAlreadyExists asserts whether the action of assigning IP address needs to
// be retried.
func isAlreadyExists(err error) bool {
	return apierrors.IsAlreadyExists(err) || errors.Is(err, errInsufficientIPs)
}

// assignIP assigns an IP address to Pod from IPPool.
func (a *allocator) assignIP(
	ctx context.Context,
	pool *requeueipv1.IPPool,
	num int,
	pod *corev1.Pod,
	workload client.Object,
) (*oapiv1.IPConfig, error) {
	// Avoid creating IP obj repeatedly.
	ri := &requeueipv1.IP{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pool.Namespace,
			Labels: map[string]string{
				consts.LabelIPVersion: pool.Spec.Version,
				consts.LabelRefIPPool: pool.Name,
			},
		},
	}

	// Index ID prefix.
	h := fnv.New32a()
	id := fmt.Sprintf("%s-%d", pool.Spec.Version, num)

	var owner metav1.Object
	if workload.GetObjectKind().GroupVersionKind().Kind == consts.KindStatefulSet {
		// The ID is generated using the Pod name instead of the UID to ensure that a
		// STS Pod will always assign the same IP addresses in case of IPPool delayed
		// scaling down.
		id = fmt.Sprintf("%s-%s", id, pod.Name)

		// The IP addresses used by Pod controlled by StatefulSet will be asynchronously
		// released by the controller. Ensure that the death of Pod does not result in the
		// release of IP addresses.
		owner = workload
		ri.Labels[consts.LabelRefSTSUID] = string(workload.GetUID())
		ri.Labels[consts.LabelRefPod] = pod.Name
	} else {
		// version + num + Pod UID can uniquely identify an IP address.
		id = fmt.Sprintf("%s-%s", id, pod.UID)

		// GC IP addresses.
		owner = pod
		ri.Labels[consts.LabelRefPodUID] = string(pod.UID)
	}

	h.Write([]byte(id))
	hash := h.Sum32()
	if err := controllerutil.SetControllerReference(owner, ri, a.client.Scheme()); err != nil {
		return nil, err
	}

	// Try to retry as much as possible, the cost of cmdAdd failure far outweighs
	// the cost of retry.
	if err := retry.OnError(backoff, isAlreadyExists, func() error {
		if err := a.client.Get(ctx, types.NamespacedName{
			Namespace: pool.Namespace,
			Name:      pool.Name,
		}, pool); err != nil {
			return err
		}

		free, err := iprange.Parse(pool.Status.Free...)
		if err != nil {
			return err
		}

		// Divide by 0 panic in concurrent case.
		size := free.Size()
		if size.Sign() == 0 {
			return fmt.Errorf("%w, IPPool %s is exhausted", errInsufficientIPs, pool.Name)
		}

		index := new(big.Int).Mod(big.NewInt(int64(hash)), size)
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
				return fmt.Errorf("failed to get a non-conflicting IP address from IPPool %s: %w", pool.Name, err)
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
