//go:build integration

package k8s_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tck3s "github.com/testcontainers/testcontainers-go/modules/k3s"
	"github.com/go-logr/logr"

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
	Expect(os.Setenv("MUTO_A2A_GATEWAY_IMAGE", "ghcr.io/a2aprotocol/a2a-gateway:v0.1.0")).To(Succeed())

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
		GinkgoLogr.Info("Using existing cluster", "kubeconfig", kubeconfigPath)
	} else {
		// Start k3s cluster via testcontainers k3s module
		GinkgoLogr.Info("Starting k3s cluster via testcontainers")
		k3sContainer, err = tck3s.Run(ctx, "rancher/k3s:v1.27.1-k3s1")
		Expect(err).NotTo(HaveOccurred())

		// Get kubeconfig, write to temp file, set KUBECONFIG env var
		kubeConfigBytes, err := k3sContainer.GetKubeConfig(ctx)
		Expect(err).NotTo(HaveOccurred())

		tmpFile, err := os.CreateTemp("", "k3s-kubeconfig-*.yaml")
		Expect(err).NotTo(HaveOccurred())
		_, err = tmpFile.Write(kubeConfigBytes)
		Expect(err).NotTo(HaveOccurred())
		Expect(tmpFile.Close()).To(Succeed())

		kubeconfigPath = tmpFile.Name()
		Expect(os.Setenv("KUBECONFIG", kubeconfigPath)).To(Succeed())
	}

	// Build rest.Config from kubeconfig
	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	Expect(err).NotTo(HaveOccurred())

	// Connect via envtest.Environment with UseExistingCluster and CRD paths
	crdPath, err := filepath.Abs("../../../deploy/crds")
	Expect(err).NotTo(HaveOccurred())

	testEnv = &envtest.Environment{
		UseExistingCluster:    boolPtr(true),
		CRDDirectoryPaths:     []string{crdPath},
		Config:                cfg,
		ErrorIfCRDPathMissing: false,
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// 4. Build runtime scheme with corev1 + appsv1 + v1alpha1
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())

	// 5. Create k8sClient
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	// 6. Start controller-runtime manager with all three reconcilers registered
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	Expect(err).NotTo(HaveOccurred())

	Expect((&reconcilers.TenantReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)).To(Succeed())

	Expect((&reconcilers.AgentJobReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)).To(Succeed())

	Expect((&reconcilers.AgentFleetReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)).To(Succeed())

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
