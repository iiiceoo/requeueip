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

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/controller"
	"github.com/iiiceoo/requeueip/pkg/controller/workload"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(requeueipv1.AddToScheme(scheme))
}

func run(ctx context.Context) error {
	zc := zap.NewDevelopmentConfig()
	zc.Level = zap.NewAtomicLevelAt(zapcore.Level(-arg.v))
	zc.DisableStacktrace = true
	z, err := zc.Build()
	if err != nil {
		return err
	}
	logger := zapr.NewLogger(z)

	// In order to get logger from ctx in the callbacks of webhook.
	ctrl.SetLogger(logger)

	// TODO(iiiceoo): Set up Pyroscope client (push mode).
	if arg.pyroscopeAddr != "" {
		logger.Info("Start Pyroscope profiler", "mode", "push", "server", arg.pyroscopeAddr)
	}

	logger.Info("Load RequeueIP controller config", "file", arg.file)
	if err := config.load(arg.file); err != nil {
		return err
	}

	logger.Info("Runtime args", "args", arg)
	logger.V(1).Info("Runtime config", "config", config)

	logger.Info("Create controller manager")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Logger: logger,
		Scheme: scheme,
		// TODO(iiiceoo): Customize the quantity of each worker.
		Controller: runtimeconfig.Controller{MaxConcurrentReconciles: arg.workers},
		Metrics:    metricsserver.Options{BindAddress: arg.metricsAddr},
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    arg.webhookHost,
			Port:    arg.webhookPort,
			CertDir: arg.webhookCertDir,
		}),
		HealthProbeBindAddress: arg.probeAddr,
		LeaderElection:         true,
		LeaderElectionID:       consts.RPrefix,
	})
	if err != nil {
		return err
	}

	// Set up IPPool controller.
	if err := controller.NewIPPoolReconciler(
		mgr.GetClient(),
	).SetupWithManager(mgr); err != nil {
		return err
	}

	// Set up Subnet controller.
	if err := controller.NewSubnetReconciler(
		mgr.GetClient(),
	).SetupWithManager(mgr); err != nil {
		return err
	}

	// Set up IPPoolClaim controller.
	if err := controller.NewIPPoolClaimReconciler(
		mgr.GetClient(),
		mgr.GetAPIReader(),
	).SetupWithManager(mgr); err != nil {
		return err
	}

	// Set up workload controllers.
	if err := workload.NewWorkloadReconciler(
		mgr.GetClient(),
	).SetupWithManager(mgr); err != nil {
		return err
	}

	// Add liveness and readiness probe endpoint.
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return err
	}
	logger.Info("Start controller manager")

	return mgr.Start(ctx)
}
