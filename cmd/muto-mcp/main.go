// SPDX-License-Identifier: Apache-2.0
package main

import (
	"log"
	"os"

	"github.com/go-logr/stdr"
	k8sadapter "github.com/muto-io/muto/platform/k8s"
	"github.com/muto-io/muto/core/scheduler"
	"github.com/muto-io/muto/mcp/server"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	ctrl.SetLogger(stdr.New(log.Default()))
	log := ctrl.Log.WithName("muto-mcp")

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

	adapter := k8sadapter.NewK8sAdapter(c, namespace)
	sched := scheduler.NewDefaultScheduler(adapter)
	srv := server.New(sched)

	log.Info("starting muto-mcp server (stdio)")
	if err := srv.ServeStdio(); err != nil {
		log.Error(err, "mcp server exited")
		os.Exit(1)
	}
}
