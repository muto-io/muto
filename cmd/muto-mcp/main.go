// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"os"

	"github.com/muto-io/muto/core/env"
	"github.com/muto-io/muto/core/logging"
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
	logFormat := env.OrDefault("MUTO_LOG_FORMAT", "json")
	logLevel := env.OrDefault("MUTO_LOG_LEVEL", "info")
	logger, err := logging.BuildLogger(logFormat, logLevel, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid logging configuration: %v\n", err)
		os.Exit(1)
	}
	ctrl.SetLogger(logger)
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

	namespace := env.OrDefault("MUTO_NAMESPACE", "default")

	adapter := k8sadapter.NewK8sAdapter(c, namespace)
	sched := scheduler.NewDefaultScheduler(adapter)
	srv := server.New(sched)

	log.Info("starting muto-mcp server (stdio)")
	if err := srv.ServeStdio(); err != nil {
		log.Error(err, "mcp server exited")
		os.Exit(1)
	}
}
