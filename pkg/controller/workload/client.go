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
	"reflect"
	"strings"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func (c *rpcClient) parseClaims(
	ctx context.Context,
	metadata *metav1.ObjectMeta,
	replicas int32,
) ([]requeueipv1.IPPoolClaimSpec, error) {
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
	labels := map[string]string{
		consts.LabelIPVersion:   strings.ToLower(spec.Version),
		consts.LabelWorkloadUID: string(object.GetUID()),
	}

	var rpcList requeueipv1.IPPoolClaimList
	if err := c.client.List(ctx, &rpcList, client.MatchingLabels(labels), client.Limit(1)); err != nil {
		return err
	}

	if len(rpcList.Items) == 0 {
		rpc := &requeueipv1.IPPoolClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      object.GetName() + "-" + uuid.New().String(),
				Namespace: object.GetNamespace(),
				Labels:    labels,
			},
			Spec: *spec,
		}
		controllerutil.AddFinalizer(rpc, consts.RFinalizer)
		if err := controllerutil.SetControllerReference(object, rpc, c.client.Scheme()); err != nil {
			return err
		}
		return client.IgnoreAlreadyExists(c.client.Create(ctx, rpc))
	}

	if reflect.DeepEqual(rpcList.Items[0].Spec, *spec) {
		return nil
	}
	rpcList.Items[0].Spec = *spec

	return c.client.Update(ctx, &rpcList.Items[0])
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
