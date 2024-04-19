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
	"net"
	"strings"
)

const (
	IPv4 = "IPv4"
	IPv6 = "IPv6"
)

// CIDRToName converts CIDR to a resource name.
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
		cidr = strings.ReplaceAll(name, "-", ":")
	default:
		return nil, fmt.Errorf("invalid IP version: %s", version)
	}

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s CIDR %s: %w", version, cidr, err)
	}

	return ipNet, nil
}

// IPToName converts IP address to resource name.
func IPToName(ip net.IP) string {
	n := ip.String()
	n = strings.Replace(n, ".", "-", 3)
	n = strings.Replace(n, ":", "-", 7)

	return n
}

// NameToIP converts resource name to IP address based on the IP version.
func NameToIP(version, name string) (net.IP, error) {
	var sep string
	switch version {
	case IPv4:
		sep = "."
	case IPv6:
		sep = ":"
	default:
		return nil, fmt.Errorf("invalid IP version: %s", version)
	}

	ipStr := strings.ReplaceAll(name, "-", sep)
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("failed to parse %s IP address: %s", version, name)
	}

	return ip, nil
}
