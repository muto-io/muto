//go:build integration

package k8s_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tck3s "github.com/testcontainers/testcontainers-go/modules/k3s"

	"github.com/muto-io/muto/platform/k8s/reconcilers"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	cfg          *rest.Config
	k8sClient    client.Client
	testEnv      *envtest.Environment
	cancelMgr    context.CancelFunc
	k3sContainer *tck3s.K3sContainer
)

func TestK8sIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "K8s Integration Suite")
}

func boolPtr(b bool) *bool { return &b }

var _ = BeforeSuite(func() {
	ctx := context.Background()

	// Initialize logger to suppress controller-runtime warnings
	ctrl.SetLogger(logr.Discard())

	// Set required env vars for reconcilers
	if err := os.Setenv("MUTO_A2A_GATEWAY_IMAGE", "ghcr.io/a2aprotocol/a2a-gateway:v0.1.0"); err != nil {
		Skip("Kubernetes cluster not available: failed to set required environment variables: " + err.Error())
	}

	// Opt the a2a-gateway Deployment into a 5s pod termination grace period
	// (instead of the Kubernetes default of 30s). This test tears down a real
	// pod on a real cluster for every spec's AfterEach, so shaving 25s off
	// each pod's graceful shutdown meaningfully speeds up and stabilizes
	// cleanup. See TenantReconciler.a2aGatewayTerminationGracePeriodSeconds
	// for why this is safe only in a test context.
	if err := os.Setenv("MUTO_A2A_GATEWAY_TEST_GRACE_PERIOD", "true"); err != nil {
		Skip("Kubernetes cluster not available: failed to set required environment variables: " + err.Error())
	}

	var err error
	var kubeconfigPath string

	// Check if using existing cluster (e.g., kind cluster from CI)
	useExistingCluster := os.Getenv("MUTO_USE_EXISTING_CLUSTER") == "true"

	if useExistingCluster {
		// Use existing cluster (kind or other)
		kubeconfigPath = os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			kubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
		// Verify kubeconfig exists
		if _, err := os.Stat(kubeconfigPath); err != nil {
			Skip("Kubernetes cluster not available: kubeconfig not found at " + kubeconfigPath)
		}
		GinkgoLogr.Info("Using existing cluster", "kubeconfig", kubeconfigPath)
	} else {
		// Start k3s cluster via testcontainers k3s module
		GinkgoLogr.Info("Starting k3s cluster via testcontainers")
		k3sContainer, err = tck3s.Run(ctx, "rancher/k3s:v1.27.1-k3s1")
		if err != nil {
			Skip("Kubernetes cluster not available: failed to start k3s container: " + err.Error())
		}

		// Get kubeconfig, write to temp file, set KUBECONFIG env var
		kubeConfigBytes, err := k3sContainer.GetKubeConfig(ctx)
		if err != nil {
			Skip("Kubernetes cluster not available: failed to get kubeconfig: " + err.Error())
		}

		tmpFile, err := os.CreateTemp("", "k3s-kubeconfig-*.yaml")
		if err != nil {
			Skip("Kubernetes cluster not available: failed to create temp kubeconfig file: " + err.Error())
		}
		_, err = tmpFile.Write(kubeConfigBytes)
		if err != nil {
			tmpFile.Close()
			Skip("Kubernetes cluster not available: failed to write kubeconfig: " + err.Error())
		}
		if err := tmpFile.Close(); err != nil {
			Skip("Kubernetes cluster not available: failed to close kubeconfig file: " + err.Error())
		}

		kubeconfigPath = tmpFile.Name()
		if err := os.Setenv("KUBECONFIG", kubeconfigPath); err != nil {
			Skip("Kubernetes cluster not available: failed to set KUBECONFIG env var: " + err.Error())
		}
	}

	// Build rest.Config from kubeconfig
	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		Skip("Kubernetes cluster not available: failed to build kubeconfig: " + err.Error())
	}

	// Connect via envtest.Environment with UseExistingCluster and CRD paths
	crdPath, err := filepath.Abs("../../../deploy/crds")
	if err != nil {
		Skip("Kubernetes cluster not available: failed to resolve CRD path: " + err.Error())
	}

	testEnv = &envtest.Environment{
		UseExistingCluster:    boolPtr(true),
		CRDDirectoryPaths:     []string{crdPath},
		Config:                cfg,
		ErrorIfCRDPathMissing: false,
	}

	cfg, err = testEnv.Start()
	if err != nil {
		Skip("Kubernetes cluster not available: failed to start envtest environment: " + err.Error())
	}
	if cfg == nil {
		Skip("Kubernetes cluster not available: envtest config is nil")
	}

	// 4. Build runtime scheme with corev1 + appsv1 + v1alpha1
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		Skip("Kubernetes cluster not available: failed to add corev1 to scheme: " + err.Error())
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		Skip("Kubernetes cluster not available: failed to add appsv1 to scheme: " + err.Error())
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		Skip("Kubernetes cluster not available: failed to add v1alpha1 to scheme: " + err.Error())
	}

	// 5. Create k8sClient
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		Skip("Kubernetes cluster not available: failed to create k8s client: " + err.Error())
	}

	// 6. Start controller-runtime manager with all three reconcilers registered
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	if err != nil {
		Skip("Kubernetes cluster not available: failed to create controller manager: " + err.Error())
	}

	if err := (&reconcilers.TenantReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		Skip("Kubernetes cluster not available: failed to setup TenantReconciler: " + err.Error())
	}

	if err := (&reconcilers.AgentJobReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		Skip("Kubernetes cluster not available: failed to setup AgentJobReconciler: " + err.Error())
	}

	if err := (&reconcilers.AgentFleetReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		Skip("Kubernetes cluster not available: failed to setup AgentFleetReconciler: " + err.Error())
	}

	// 7. Start manager in goroutine, save cancel func
	var mgrCtx context.Context
	mgrCtx, cancelMgr = context.WithCancel(context.Background())
	go func() {
		Expect(mgr.Start(mgrCtx)).To(Succeed())
	}()
})

var _ = AfterSuite(func() {
	ctx := context.Background()

	// Stop the manager
	if cancelMgr != nil {
		cancelMgr()
	}

	// Allow manager goroutine to exit
	time.Sleep(200 * time.Millisecond)

	// Stop envtest
	if testEnv != nil {
		Expect(testEnv.Stop()).To(Succeed())
	}

	// Terminate k3s cluster
	if k3sContainer != nil {
		Expect(k3sContainer.Terminate(ctx)).To(Succeed())
	}
})

// waitForPhase is a helper retained from the original suite for use by sub-tests.
func waitForPhase(t GinkgoTInterface, ctx context.Context, name, namespace, phase string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job := &v1alpha1.AgentJob{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, job); err == nil {
			if job.Status.Phase == phase {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("job %s/%s did not reach phase %s within %s", namespace, name, phase, timeout)
}
