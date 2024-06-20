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
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"

	oapiv1 "github.com/iiiceoo/requeueip/oapi/v1"
)

func New(c client.Client, reader client.Reader) oapiv1.StrictServerInterface {
	return &ipamService{
		allocator: NewAllocator(c, reader),
	}
}

type ipamService struct {
	allocator Allocator
}

func (s *ipamService) Health(ctx context.Context, request oapiv1.HealthRequestObject) (oapiv1.HealthResponseObject, error) {
	return oapiv1.Health200Response{}, nil
}

func (s *ipamService) CmdAdd(ctx context.Context, request oapiv1.CmdAddRequestObject) (oapiv1.CmdAddResponseObject, error) {
	// TODO(iiiceoo): Support multiple IPv4 or IPv6 addresses.
	v4, v6 := 0, 0
	if request.Body.Ipv4 {
		v4 = 1
	}
	if request.Body.Ipv6 {
		v6 = 1
	}

	ips, err := s.allocator.Get(
		ctx,
		request.Body.PodNamespace,
		request.Body.PodName,
		&Options{
			IPv4: v4,
			IPv6: v6,
		},
	)
	if err != nil {
		return oapiv1.CmdAdddefaultJSONResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       err.Error(),
		}, nil
	}

	return oapiv1.CmdAdd200JSONResponse{Ips: ips}, nil
}
