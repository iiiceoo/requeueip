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

package run

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	v1 "github.com/iiiceoo/requeueip/oapi/v1"
)

func CmdAdd(args *skel.CmdArgs) (err error) {
	ipamConf, confVersion, err := LoadIPAMConfig(args.StdinData, args.Args)
	if err != nil {
		return fmt.Errorf("failed to load IPAM config: %v", err)
	}

	zc := zap.Config{
		Level:             zap.NewAtomicLevelAt(zapcore.Level(-ipamConf.LogLevel)),
		Development:       false,
		Encoding:          "console",
		EncoderConfig:     zap.NewProductionEncoderConfig(),
		DisableStacktrace: true,
		OutputPaths:       []string{ipamConf.LogPath},
		ErrorOutputPaths:  []string{ipamConf.LogPath},
	}
	z, err := zc.Build()
	if err != nil {
		return fmt.Errorf("failed to build zap logger from config: %v", err)
	}
	logger := zapr.NewLogger(z).WithValues(
		"action", "add",
		"containerID", args.ContainerID,
		"netns", args.Netns,
		"ifName", args.IfName,
	)

	envs := IPAMEnvArgs{}
	if err = types.LoadArgs(args.Args, &envs); err != nil {
		logger.Error(nil, err.Error())
		return err
	}

	logger = logger.WithValues(
		"podNamespace", envs.PodNamespace,
		"podName", envs.PodName,
		"podUID", envs.PodUID,
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	result, err := assign(ctx, args, &envs, ipamConf)
	if err != nil {
		logger.Error(nil, err.Error())
		return err
	}
	logger.Info("Successfully assigned IP addresses", "result", *result)

	return types.PrintResult(result, confVersion)
}

func assign(ctx context.Context, args *skel.CmdArgs, envs *IPAMEnvArgs, ipamConfig *IPAMConfig) (*current.Result, error) {
	client, err := newUnixClient(ipamConfig.UnixSocketPath)
	if err != nil {
		return nil, err
	}

	// Send health check.
	if _, err := client.HealthWithResponse(ctx); err != nil {
		return nil, err
	}

	// Send IPAM request.
	resp, err := client.CmdAddWithResponse(ctx, v1.CmdAddJSONRequestBody{
		PodNamespace: string(envs.PodNamespace),
		PodName:      string(envs.PodName),
		ContainerID:  args.ContainerID,
		IfName:       args.IfName,
	})
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("failed to assign IP addresses: %s", *resp.JSONDefault)
	}

	result := &current.Result{
		CNIVersion: current.ImplementedSpecVersion,
	}

	// TODO(iiiceoo): Implement DNS and Routes.
	for _, c := range resp.JSON200.Ips {
		ip, ipNet, err := net.ParseCIDR(c.Address)
		if err != nil {
			return nil, err
		}
		result.IPs = append(result.IPs, &current.IPConfig{
			Address: net.IPNet{IP: ip, Mask: ipNet.Mask},
			// TODO(iiiceoo): Implement gateway.
		})
	}

	return result, nil
}

func newUnixClient(socketPath string) (*v1.ClientWithResponses, error) {
	dialer := func(ctx context.Context, network, address string) (net.Conn, error) {
		d := &net.Dialer{}
		return d.DialContext(ctx, "unix", socketPath)
	}
	transport := &http.Transport{
		DialContext: dialer,
	}
	client := &http.Client{
		Transport: transport,
	}

	return v1.NewClientWithResponses("http://unix:/", v1.WithHTTPClient(client))
}
