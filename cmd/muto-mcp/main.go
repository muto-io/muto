// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/go-logr/stdr"
	"github.com/muto-io/muto/core/scheduler"
	"github.com/muto-io/muto/core/tracing"
	"github.com/muto-io/muto/mcp/server"
	k8sadapter "github.com/muto-io/muto/platform/k8s"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	ctrl.SetLogger(stdr.New(log.Default()))
	log := ctrl.Log.WithName("muto-mcp")

	shutdownTracing, err := tracing.Init(context.Background(), "muto-mcp")
	if err != nil {
		log.Error(err, "unable to initialize tracing")
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			log.Error(err, "tracing shutdown failed")
		}
	}()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cfg := ctrl.GetConfigOrDie()
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "unable to create k8s client")
		os.Exit(1)
	}

	namespace := os.Getenv("MUTO_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	adapter := tracing.WrapPlatformAdapter(k8sadapter.NewK8sAdapter(c, namespace))
	sched := tracing.WrapScheduler(scheduler.NewDefaultScheduler(adapter))
	srv := server.New(sched)

	log.Info("starting muto-mcp server (stdio)")
	if err := srv.ServeStdio(); err != nil {
		log.Error(err, "mcp server exited")
		os.Exit(1)
	}
}
