// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/go-logr/logr"
	k8sadapter "github.com/muto-io/muto/platform/k8s"
	"github.com/muto-io/muto/core/scheduler"
	"github.com/muto-io/muto/mcp/server"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

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
