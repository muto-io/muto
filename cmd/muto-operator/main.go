// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/muto-io/muto/core/env"
	"github.com/muto-io/muto/core/logging"
	coremetrics "github.com/muto-io/muto/core/metrics"
	"github.com/muto-io/muto/core/scheduler"
	"github.com/muto-io/muto/core/shutdown"
	"github.com/muto-io/muto/core/tracing"
	cfplatform "github.com/muto-io/muto/platform/cf"
	k8sadapter "github.com/muto-io/muto/platform/k8s"
	"github.com/muto-io/muto/platform/k8s/reconcilers"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var scheme = runtime.NewScheme()

func init() {
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
}

// newManager creates the controller manager serving Prometheus metrics on
// metricsAddr and the /healthz and /readyz probes on probeAddr.
// controller-runtime only mounts the probe endpoints once a check is
// registered, so both checks must be added here.
func newManager(cfg *rest.Config, metricsAddr, probeAddr string) (ctrl.Manager, error) {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			// "0" disables the metrics server entirely, per
			// controller-runtime's BindAddress contract.
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		return nil, err
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("adding healthz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("adding readyz check: %w", err)
	}
	return mgr, nil
}

func main() {
	os.Exit(run())
}

// run contains the entirety of the operator's startup/serve logic and
// returns a process exit code rather than calling os.Exit itself. os.Exit
// terminates the process immediately without running any deferred
// functions, which would otherwise skip the tracing shutdown deferred
// below on every error path; returning normally from run lets main's
// single os.Exit(run()) call happen only after those defers (including the
// bounded-timeout trace flush) have run.
func run() int {
	logFormat := env.OrDefault("MUTO_LOG_FORMAT", "json")
	logLevel := env.OrDefault("MUTO_LOG_LEVEL", "info")
	logger, err := logging.BuildLogger(logFormat, logLevel, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid logging configuration: %v\n", err)
		return 1
	}
	ctrl.SetLogger(logger)
	log := ctrl.Log.WithName("muto-operator")

	// Registered once, up front: whichever shutdown funcs have been
	// appended by the time run() returns all flush concurrently under one
	// shared 5s deadline, instead of each Init's shutdown getting its own
	// deadline and running sequentially in LIFO defer order.
	var shutdowns []shutdown.Func
	defer func() { shutdown.All(context.Background(), log, 5*time.Second, shutdowns...) }()

	shutdownTracing, err := tracing.Init(context.Background(), "muto-operator")
	if err != nil {
		log.Error(err, "unable to initialize tracing")
		return 1
	}
	shutdowns = append(shutdowns, shutdown.Func{Name: "tracing", Run: shutdownTracing})

	shutdownOTLPMetrics, err := coremetrics.Init(context.Background(), "muto-operator")
	if err != nil {
		log.Error(err, "unable to initialize OTLP metrics")
		return 1
	}
	shutdowns = append(shutdowns, shutdown.Func{Name: "OTLP metrics", Run: shutdownOTLPMetrics})

	metricsAddr := env.OrDefault("MUTO_METRICS_BIND_ADDRESS", ":8080")
	probeAddr := env.OrDefault("MUTO_HEALTH_PROBE_BIND_ADDRESS", ":8081")

	// ctrl.GetConfigOrDie calls os.Exit(1) on failure, which would skip the
	// tracing/metrics shutdown defers above; ctrl.GetConfig lets that error
	// flow through run()'s normal return path instead.
	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Error(err, "unable to load kubeconfig")
		return 1
	}

	mgr, err := newManager(cfg, metricsAddr, probeAddr)
	if err != nil {
		log.Error(err, "unable to start manager")
		return 1
	}

	platform := env.OrDefault("MUTO_PLATFORM", "k8s")

	var platformAdapter scheduler.PlatformAdapter
	switch platform {
	case "k8s":
		namespace := env.OrDefault("MUTO_NAMESPACE", "default")
		c, err := client.New(cfg, client.Options{Scheme: scheme})
		if err != nil {
			log.Error(err, "unable to create k8s client for adapter")
			return 1
		}
		platformAdapter = tracing.WrapPlatformAdapter(k8sadapter.NewK8sAdapter(c, namespace))
	case "cf":
		cfClient, err := cfplatform.NewRealCFClient(
			os.Getenv("CF_API_URL"),
			os.Getenv("CF_USERNAME"),
			os.Getenv("CF_PASSWORD"),
		)
		if err != nil {
			log.Error(err, "unable to create CF client")
			return 1
		}
		platformAdapter = tracing.WrapPlatformAdapter(cfplatform.NewCFAdapter(cfClient, cfplatform.CFAdapterConfig{
			IsolationTier: os.Getenv("CF_ISOLATION_TIER"),
			SharedOrgName: os.Getenv("CF_SHARED_ORG"),
		}))
	default:
		log.Error(nil, "unknown MUTO_PLATFORM value", "platform", platform)
		return 1
	}

	// platformAdapter is used by the DefaultScheduler for direct job scheduling
	// (e.g. via the MCP server). K8s reconcilers use mgr.GetClient() directly
	// and are platform-independent. Future work: pass platformAdapter into
	// reconcilers that need to spawn agents on non-K8s platforms.
	log.Info("platform adapter initialized", "platform", platform, "type", fmt.Sprintf("%T", platformAdapter))

	if err := (&reconcilers.TenantReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create TenantReconciler")
		return 1
	}

	if err := (&reconcilers.AgentJobReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create AgentJobReconciler")
		return 1
	}

	if err := (&reconcilers.AgentFleetReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create AgentFleetReconciler")
		return 1
	}

	log.Info("starting muto-operator", "platform", platform)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "operator exited with error")
		return 1
	}
	return 0
}
