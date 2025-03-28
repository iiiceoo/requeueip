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

	"github.com/iiiceoo/iprange"
)

const (
	IPv4 = "IPv4"
	IPv6 = "IPv6"
)

// IPToName converts IP address to resource name.
func IPToName(ip net.IP) string {
	n := ip.String()
	n = strings.Replace(n, ".", "-", 3)
	n = strings.Replace(n, ":", "-", 7)

	// e.g. fd00:6:1::
	if strings.HasSuffix(n, "--") {
		n += "0"
	}

	return n
}

// NameToCIDRIP converts resource name to IP address(CIDR) based on the IP
// version.
func NameToCIDRIP(version, name string) (*net.IPNet, error) {
	ip, err := NameToIP(version, name)
	if err != nil {
		return nil, err
	}

	var mask net.IPMask
	if version == IPv4 {
		mask = net.CIDRMask(32, 32)
	} else {
		mask = net.CIDRMask(128, 128)
	}

	return &net.IPNet{
		IP:   ip,
		Mask: mask,
	}, nil
}

// NameToIP converts resource name to IP address based on the IP version.
func NameToIP(version, name string) (net.IP, error) {
	ipStr, err := nameToIPString(version, name)
	if err != nil {
		return nil, err
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("failed to parse %s IP address: %s", version, name)
	}

	return ip, nil
}

// NamesToIPIPRanges converts resource names to IPRanges (IP) based on the IP
// version.
func NamesToIPIPRanges(version string, names ...string) (*iprange.IPRanges, error) {
	ips := make([]string, 0, len(names))
	for _, n := range names {
		ip, err := nameToIPString(version, n)
		if err != nil {
			return nil, err
		}
		ips = append(ips, ip)
	}

	return iprange.Parse(ips...)
}

// nameToString converts resource name to IP address string based on the IP
// version.
func nameToIPString(version, name string) (string, error) {
	var sep string
	switch version {
	case IPv4:
		sep = "."
	case IPv6:
		sep = ":"
	default:
		return "", fmt.Errorf("invalid IP version: %s", version)
	}

	return strings.ReplaceAll(name, "-", sep), nil
}
