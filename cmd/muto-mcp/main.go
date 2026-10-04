// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/muto-io/muto/core/env"
	"github.com/muto-io/muto/core/logging"
	"github.com/muto-io/muto/core/scheduler"
	"github.com/muto-io/muto/core/shutdown"
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
	os.Exit(run())
}

// run contains the entirety of the server's startup/serve logic and
// returns a process exit code rather than calling os.Exit itself. os.Exit
// terminates the process immediately without running any deferred
// functions, which would otherwise skip the tracing shutdown deferred
// below on every error path; returning normally from run lets main's
// single os.Exit(run()) call happen only after those defers (including the
// bounded-timeout trace flush) have run.
func run() int {
	logFormat := env.OrDefault("MUTO_LOG_FORMAT", "json")
	logLevel := env.OrDefault("MUTO_LOG_LEVEL", "info")
	logger, err := logging.BuildLogger(logFormat, logLevel, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid logging configuration: %v\n", err)
		return 1
	}
	ctrl.SetLogger(logger)
	log := ctrl.Log.WithName("muto-mcp")

	shutdownTracing, err := tracing.Init(context.Background(), "muto-mcp")
	if err != nil {
		log.Error(err, "unable to initialize tracing")
		return 1
	}
	defer func() {
		shutdown.All(context.Background(), log, 5*time.Second, shutdown.Func{Name: "tracing", Run: shutdownTracing})
	}()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	// ctrl.GetConfigOrDie calls os.Exit(1) on failure, which would skip the
	// tracing shutdown defer above; ctrl.GetConfig lets that error flow
	// through run()'s normal return path instead.
	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Error(err, "unable to load kubeconfig")
		return 1
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "unable to create k8s client")
		return 1
	}

	namespace := env.OrDefault("MUTO_NAMESPACE", "default")

	adapter := tracing.WrapPlatformAdapter(k8sadapter.NewK8sAdapter(c, namespace))
	sched := tracing.WrapScheduler(scheduler.NewDefaultScheduler(adapter))
	srv := server.New(sched)

	log.Info("starting muto-mcp server (stdio)")
	if err := srv.ServeStdio(); err != nil {
		log.Error(err, "mcp server exited")
		return 1
	}
	return 0
}
