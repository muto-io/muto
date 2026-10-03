// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"fmt"
	"io"
	"strings"

	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// BuildLogger builds a zap-backed logr.Logger at the given level and format
func BuildLogger(format, level string, out io.Writer) (logr.Logger, error) {
	var levelOpt zapcore.Level
	switch strings.ToLower(level) {
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
	switch strings.ToLower(format) {
	case "json":
		encoderOpt = ctrlzap.JSONEncoder()
	case "console":
		encoderOpt = ctrlzap.ConsoleEncoder()
	default:
		return logr.Logger{}, fmt.Errorf("invalid MUTO_LOG_FORMAT %q: must be one of json, console", format)
	}

	return ctrlzap.New(ctrlzap.Level(levelOpt), encoderOpt, ctrlzap.WriteTo(out)), nil
}
