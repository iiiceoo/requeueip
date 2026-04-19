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

package workload

import (
	"context"
	"fmt"
	"hash/fnv"
	"reflect"
	"strings"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/net"
)

type rpcClient struct {
	client client.Client
}

func newRPCClient(c client.Client) *rpcClient {
	return &rpcClient{
		client: c,
	}
}

// ensureClaimsForWorkload ensures IPPoolClaims declared by the workload
// template annotations.
func (c *rpcClient) ensureClaimsForWorkload(
	ctx context.Context,
	object client.Object,
	annotations map[string]string,
	replicas *int32,
) error {
	// The workload is terminating, do nothing, OwnerReference will ensure that
	// the relevant IPPoolClaims are recycled.
	if !object.GetDeletionTimestamp().IsZero() {
		return nil
	}

	claims := parseClaims(annotations, *replicas)
	return c.ensureClaims(ctx, claims, object)
}

// ensureClaims updates IPPoolClaims with specified specs, or creates them if
// they do not exist.
func (c *rpcClient) ensureClaims(ctx context.Context, specs []requeueipv1.IPPoolClaimSpec, object client.Object) error {
	n := len(specs)
	if n == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Go(func() {
			if err := c.ensureClaim(ctx, &specs[i], object); err != nil {
				errCh <- err
			}
		})
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) != 0 {
		return utilerrors.NewAggregate(errs)
	}

	return nil
}

// ensureClaim updates IPPoolClaim with the specified spec, or creates it if it
// does not exist.
func (c *rpcClient) ensureClaim(ctx context.Context, spec *requeueipv1.IPPoolClaimSpec, object client.Object) error {
	// version + wordload UID can uniquely identify an IPPoolClaim.
	h := fnv.New32a()
	id := fmt.Sprintf("%s-%s", spec.Version, object.GetUID())
	h.Write([]byte(id))
	name := fmt.Sprintf("%s-%s", object.GetName(), rand.SafeEncodeString(fmt.Sprint(h.Sum32())))

	var rpc requeueipv1.IPPoolClaim
	if err := c.client.Get(ctx, types.NamespacedName{
		Namespace: object.GetNamespace(),
		Name:      name,
	}, &rpc); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		rpc := &requeueipv1.IPPoolClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: object.GetNamespace(),
			},
			Spec: *spec,
		}

		controllerutil.AddFinalizer(rpc, consts.RFinalizer)
		if err := controllerutil.SetControllerReference(object, rpc, c.client.Scheme()); err != nil {
			return err
		}

		return client.IgnoreAlreadyExists(c.client.Create(ctx, rpc))
	}

	if reflect.DeepEqual(&rpc.Spec, spec) {
		return nil
	}

	rpc.Spec = *spec
	return c.client.Update(ctx, &rpc)
}

// parseClaims parses workload template annotations as IPPoolClaim specs.
func parseClaims(annotations map[string]string, replicas int32) []requeueipv1.IPPoolClaimSpec {
	v4Str, v6Str := "", ""
	if len(annotations) != 0 {
		v4Str = annotations[consts.AnnoIPv4Subnets]
		v6Str = annotations[consts.AnnoIPv6Subnets]
	}

	if v4Str == "" && v6Str == "" {
		return nil
	}

	v4Subnets := parseArray(v4Str)
	v6Subnets := parseArray(v6Str)

	var claims []requeueipv1.IPPoolClaimSpec
	if len(v4Subnets) != 0 {
		claims = append(claims, requeueipv1.IPPoolClaimSpec{
			Version:  net.IPv4,
			Subnets:  v4Subnets,
			Replicas: replicas,
		})
	}
	if len(v6Subnets) != 0 {
		claims = append(claims, requeueipv1.IPPoolClaimSpec{
			Version:  net.IPv6,
			Subnets:  v6Subnets,
			Replicas: replicas,
		})
	}

	return claims
}

// parseArray parses a comma-separated string into a slice.
func parseArray(arrStr string) []string {
	if arrStr == "" {
		return nil
	}

	var res []string
	parts := strings.Split(arrStr, ",")
	for _, p := range parts {
		p = strings.Trim(p, " ")
		if p != "" {
			res = append(res, p)
		}
	}

	return res
}
