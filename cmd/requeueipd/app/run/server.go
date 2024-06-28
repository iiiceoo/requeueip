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
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	oapiv1 "github.com/iiiceoo/requeueip/oapi/v1"
	"github.com/iiiceoo/requeueip/pkg/ipam"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(requeueipv1.AddToScheme(scheme))
}

func run(ctx context.Context) error {
	zc := zap.Config{
		Level:             zap.NewAtomicLevelAt(zapcore.Level(-arg.v)),
		Development:       false,
		Encoding:          "console",
		EncoderConfig:     zap.NewDevelopmentEncoderConfig(),
		DisableStacktrace: true,
		OutputPaths:       []string{"stderr"},
		ErrorOutputPaths:  []string{"stderr"},
	}
	z, err := zc.Build()
	if err != nil {
		return err
	}
	logger := zapr.NewLogger(z)

	// In order to get logger from ctx in the backend of RequeueIP daemon.
	ctrl.SetLogger(logger)

	// TODO(iiiceoo): Set up Pyroscope client (push mode).
	if arg.pyroscopeAddr != "" {
		logger.Info("Start Pyroscope profiler", "mode", "push", "server", arg.pyroscopeAddr)
	}

	logger.Info("Load RequeueIP daemon config", "file", arg.file)
	if err := config.load(arg.file); err != nil {
		return err
	}

	logger.Info("Runtime args", "args", arg)
	logger.V(1).Info("Runtime config", "config", config)

	logger.Info("Create controller-runtime manager")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Logger:                 logger,
		Scheme:                 scheme,
		HealthProbeBindAddress: arg.probeAddr,
	})
	if err != nil {
		return err
	}

	// Add liveness and readiness probe endpoint.
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return err
	}

	logger.V(1).Info("Clean Unix socket", "path", arg.unixSocketPath)
	if err := os.RemoveAll(arg.unixSocketPath); err != nil {
		return err
	}

	logger.Info("Create IPAM HTTP Unix server")
	server := &http.Server{
		Handler: oapiv1.Handler(oapiv1.NewStrictHandler(ipam.New(
			mgr.GetClient(),
			mgr.GetAPIReader(),
		), nil)),
	}

	listener, err := net.Listen("unix", arg.unixSocketPath)
	if err != nil {
		return err
	}
	defer listener.Close()

	errCh := make(chan error, 2)
	go func() {
		logger.Info("Start controller-runtime manager")
		if err := mgr.Start(ctx); err != nil {
			errCh <- err
		}
	}()

	go func() {
		logger.Info("Start IPAM HTTP Unix server", "socket", arg.unixSocketPath)
		if err := server.Serve(listener); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		// The DeletionGracePeriodSeconds of Pod is generally 30s.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
