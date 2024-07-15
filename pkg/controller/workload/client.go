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

	str2duration "github.com/xhit/go-str2duration/v2"
	corev1 "k8s.io/api/core/v1"
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

// parseClaims parses Pod or Namespace annotations as IPPoolClaim spec.
func (c *rpcClient) parseClaims(
	ctx context.Context,
	namespace string,
	annotations map[string]string,
	replicas int32,
) ([]requeueipv1.IPPoolClaimSpec, error) {
	if annotations == nil {
		return nil, nil
	}

	v4Str := annotations[consts.AnnoIPv4Subnets]
	v6Str := annotations[consts.AnnoIPv6Subnets]
	if v4Str == "" && v6Str == "" {
		var ns corev1.Namespace
		if err := c.client.Get(ctx, types.NamespacedName{Name: namespace}, &ns); err != nil {
			return nil, err
		}
		v4Str = ns.Annotations[consts.AnnoIPv4Subnets]
		v6Str = ns.Annotations[consts.AnnoIPv6Subnets]
	}
	v4Subnets := parseArray(v4Str)
	v6Subnets := parseArray(v6Str)

	delay := annotations[consts.AnnoScaleDownDelay]
	if delay == "" {
		delay = "0"
	}
	_, err := str2duration.ParseDuration(delay)
	if err != nil {
		return nil, err
	}

	var claims []requeueipv1.IPPoolClaimSpec
	if len(v4Subnets) != 0 {
		claims = append(claims, requeueipv1.IPPoolClaimSpec{
			Version:        net.IPv4,
			Subnets:        v4Subnets,
			Replicas:       replicas,
			ScaleDownDelay: &delay,
		})
	}
	if len(v6Subnets) != 0 {
		claims = append(claims, requeueipv1.IPPoolClaimSpec{
			Version:        net.IPv6,
			Subnets:        v6Subnets,
			Replicas:       replicas,
			ScaleDownDelay: &delay,
		})
	}

	return claims, nil
}

// ensureClaims updates IPPoolClaims with specified specs, or creates them if
// they do not exist.
func (c *rpcClient) ensureClaims(ctx context.Context, specs []requeueipv1.IPPoolClaimSpec, object client.Object) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(specs))
	for i := 0; i < len(specs); i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.ensureClaim(ctx, &specs[i], object); err != nil {
				errCh <- err
			}
		}()
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
