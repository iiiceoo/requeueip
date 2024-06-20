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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func (c *rpcClient) parseClaims(ctx context.Context, metadata *metav1.ObjectMeta, replicas int32) ([]requeueipv1.IPPoolClaimSpec, error) {
	v4Str := metadata.Annotations[consts.AnnoIPv4Subnets]
	v6Str := metadata.Annotations[consts.AnnoIPv6Subnets]
	if v4Str == "" && v6Str == "" {
		var ns corev1.Namespace
		if err := c.client.Get(ctx, types.NamespacedName{Name: metadata.Namespace}, &ns); err != nil {
			return nil, err
		}
		v4Str = ns.Annotations[consts.AnnoIPv4Subnets]
		v6Str = ns.Annotations[consts.AnnoIPv6Subnets]
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

	return claims, nil
}

func (c *rpcClient) ensureClaim(ctx context.Context, spec *requeueipv1.IPPoolClaimSpec, object client.Object) error {
	name := object.GetName() + "-" + getUIDHash(spec.Version, string(object.GetUID()))
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

func getUIDHash(version, uid string) string {
	id := fmt.Sprintf("%s-%s", uid, version)
	h := fnv.New32a()
	h.Write([]byte(id))

	return rand.SafeEncodeString(fmt.Sprint(h.Sum32()))
}

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
