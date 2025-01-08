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

package net

import (
	"fmt"
	"math/big"
	"net"
	"strings"
)

// CIDRToName converts CIDR string to resource name.
func CIDRStringToName(s string) string {
	n := strings.Replace(s, ".", "-", 3)
	n = strings.Replace(n, ":", "-", 7)
	n = strings.Replace(n, "/", "-", 1)

	return n
}

// CIDRToName converts CIDR to resource name.
func CIDRToName(cidr *net.IPNet) string {
	n := cidr.String()
	n = strings.Replace(n, ".", "-", 3)
	n = strings.Replace(n, ":", "-", 7)
	n = strings.Replace(n, "/", "-", 1)

	return n
}

// NameToCIDR converts resource name to CIDR based on the IP version.
func NameToCIDR(version, name string) (*net.IPNet, error) {
	var cidr string
	switch version {
	case IPv4:
		parts := strings.Split(name, "-")
		if len(parts) != 5 {
			return nil, fmt.Errorf("invalid IPv4 CIDR name: %s", name)
		}
		cidr = strings.Join(parts[:4], ".") + "/" + parts[4]
	case IPv6:
		index := strings.LastIndex(name, "-")
		cidr = strings.ReplaceAll(name[:index], "-", ":") + "/" + name[index+1:]
	default:
		return nil, fmt.Errorf("invalid IP version: %s", version)
	}

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	return ipNet, nil
}

// CountFromMaskSize calculates the number of IP addresses based on the size of
// the subnet mask.
func CountFromMaskSize(version string, maskSize int) (*big.Int, error) {
	var bits int
	switch version {
	case IPv4:
		bits = 32
	case IPv6:
		bits = 128
	default:
		return nil, fmt.Errorf("invalid IP version: %s", version)
	}
	count := big.NewInt(1)
	count = count.Lsh(count, uint(bits-maskSize))

	return count, nil
}
