package main

import (
	"fmt"
	"log"
	"os"

	"github.com/go-logr/stdr"
	cfplatform "github.com/muto-io/muto/platform/cf"
	k8sadapter "github.com/muto-io/muto/platform/k8s"
	"github.com/muto-io/muto/platform/k8s/reconcilers"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	"github.com/muto-io/muto/core/scheduler"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var scheme = runtime.NewScheme()

func init() {
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
}

func main() {
	ctrl.SetLogger(stdr.New(log.Default()))
	log := ctrl.Log.WithName("muto-operator")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: ":8080",
		},
		HealthProbeBindAddress: ":8081",
	})
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
