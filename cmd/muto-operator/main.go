// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/go-logr/logr"
	cfplatform "github.com/muto-io/muto/platform/cf"
	k8sadapter "github.com/muto-io/muto/platform/k8s"
	"github.com/muto-io/muto/platform/k8s/reconcilers"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	"github.com/muto-io/muto/core/scheduler"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var scheme = runtime.NewScheme()

func init() {
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
}

// buildLogger builds a zap-backed logr.Logger at the given level, writing
// out in the given format. format must be "json" or "console"; level must
// be one of "debug", "info", "warn", "error". The two are independent
// knobs (not zap.UseDevMode, which conflates encoder choice with level and
// stacktrace defaults).
func buildLogger(format, level string, out io.Writer) (logr.Logger, error) {
	var levelOpt zapcore.Level
	switch level {
	case "debug":
		levelOpt = zapcore.DebugLevel
	case "info":
		levelOpt = zapcore.InfoLevel
	case "warn":
		levelOpt = zapcore.WarnLevel
	case "error":
		levelOpt = zapcore.ErrorLevel
	default:
		return logr.Logger{}, fmt.Errorf("invalid MUTO_LOG_LEVEL %q: must be one of debug, info, warn, error", level)
	}

	var encoderOpt ctrlzap.Opts
	switch format {
	case "json":
		encoderOpt = ctrlzap.JSONEncoder()
	case "console":
		encoderOpt = ctrlzap.ConsoleEncoder()
	default:
		return logr.Logger{}, fmt.Errorf("invalid MUTO_LOG_FORMAT %q: must be one of json, console", format)
	}

	return ctrlzap.New(ctrlzap.Level(levelOpt), encoderOpt, ctrlzap.WriteTo(out)), nil
}

// newManager creates the controller manager serving Prometheus metrics on
// metricsAddr and the /healthz and /readyz probes on probeAddr.
// controller-runtime only mounts the probe endpoints once a check is
// registered, so both checks must be added here.
func newManager(cfg *rest.Config, metricsAddr, probeAddr string) (ctrl.Manager, error) {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
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
	logFormat := os.Getenv("MUTO_LOG_FORMAT")
	if logFormat == "" {
		logFormat = "json"
	}
	logLevel := os.Getenv("MUTO_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	logger, err := buildLogger(logFormat, logLevel, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid logging configuration: %v\n", err)
		os.Exit(1)
	}
	ctrl.SetLogger(logger)
	log := ctrl.Log.WithName("muto-operator")

	metricsAddr := os.Getenv("MUTO_METRICS_BIND_ADDRESS")
	if metricsAddr == "" {
		metricsAddr = ":8080"
	}
	probeAddr := os.Getenv("MUTO_HEALTH_PROBE_BIND_ADDRESS")
	if probeAddr == "" {
		probeAddr = ":8081"
	}

	mgr, err := newManager(ctrl.GetConfigOrDie(), metricsAddr, probeAddr)
	if err != nil {
		log.Error(err, "unable to start manager")
		os.Exit(1)
	}

	platform := os.Getenv("MUTO_PLATFORM")
	if platform == "" {
		platform = "k8s"
	}

	var platformAdapter scheduler.PlatformAdapter
	switch platform {
	case "k8s":
		namespace := os.Getenv("MUTO_NAMESPACE")
		if namespace == "" {
			namespace = "default"
		}
		c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
		if err != nil {
			log.Error(err, "unable to create k8s client for adapter")
			os.Exit(1)
		}
		platformAdapter = k8sadapter.NewK8sAdapter(c, namespace)
	case "cf":
		cfClient, err := cfplatform.NewRealCFClient(
			os.Getenv("CF_API_URL"),
			os.Getenv("CF_USERNAME"),
			os.Getenv("CF_PASSWORD"),
		)
		if err != nil {
			log.Error(err, "unable to create CF client")
			os.Exit(1)
		}
		platformAdapter = cfplatform.NewCFAdapter(cfClient, cfplatform.CFAdapterConfig{
			IsolationTier: os.Getenv("CF_ISOLATION_TIER"),
			SharedOrgName: os.Getenv("CF_SHARED_ORG"),
		})
	default:
		log.Error(nil, "unknown MUTO_PLATFORM value", "platform", platform)
		os.Exit(1)
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
		os.Exit(1)
	}

	if err := (&reconcilers.AgentJobReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create AgentJobReconciler")
		os.Exit(1)
	}

	if err := (&reconcilers.AgentFleetReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create AgentFleetReconciler")
		os.Exit(1)
	}

	log.Info("starting muto-operator", "platform", platform)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "operator exited with error")
		os.Exit(1)
	}
}
