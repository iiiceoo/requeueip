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

package controller

import (
	"math/big"

	"github.com/iiiceoo/iprange"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/net"
)

var (
	zero = big.NewInt(0)
	one  = big.NewInt(1)
)

// parseRangesFromIPs aggregates multiple IPs into unmerged IPRanges.
func parseRangesFromIPs(version string, ips []requeueipv1.IP) (*iprange.IPRanges, error) {
	ipStrs := make([]string, 0, len(ips))
	for i := 0; i < len(ips); i++ {
		ip, err := net.NameToIP(version, ips[i].Name)
		if err != nil {
			return nil, err
		}
		ipStrs = append(ipStrs, ip.String())
	}

	return iprange.Parse(ipStrs...)
}

// parseRangesFromIPBlocks aggregates multiple IPBlocks into unmerged IPRanges.
func parseRangesFromIPBlocks(version string, blocks []requeueipv1.IPBlock) (*iprange.IPRanges, error) {
	blockStrs := make([]string, 0, len(blocks))
	for i := 0; i < len(blocks); i++ {
		ipNet, err := net.NameToCIDR(version, blocks[i].Name)
		if err != nil {
			return nil, err
		}
		blockStrs = append(blockStrs, ipNet.String())
	}

	return iprange.Parse(blockStrs...)
}
