// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestBuildLogger(t *testing.T) {
	t.Run("valid combinations", func(t *testing.T) {
		for _, level := range []string{"debug", "info", "warn", "error"} {
			for _, format := range []string{"json", "console"} {
				t.Run(level+"/"+format, func(t *testing.T) {
					var buf bytes.Buffer
					if _, err := buildLogger(format, level, &buf); err != nil {
						t.Fatalf("buildLogger(%q, %q): %v", format, level, err)
					}
				})
			}
		}
	})

	t.Run("invalid level", func(t *testing.T) {
		var buf bytes.Buffer
		if _, err := buildLogger("json", "trace", &buf); err == nil {
			t.Fatal("expected error for invalid MUTO_LOG_LEVEL, got nil")
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		var buf bytes.Buffer
		if _, err := buildLogger("yaml", "info", &buf); err == nil {
			t.Fatal("expected error for invalid MUTO_LOG_FORMAT, got nil")
		}
	})

	t.Run("json format emits parseable JSON", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := buildLogger("json", "info", &buf)
		if err != nil {
			t.Fatalf("buildLogger: %v", err)
		}
		logger.Info("test message", "key", "value")

		line := buf.Bytes()
		if len(line) == 0 {
			t.Fatal("expected log output, got none")
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(line, &decoded); err != nil {
			t.Fatalf("log line is not valid JSON: %v\nline: %s", err, line)
		}
		if decoded["msg"] != "test message" {
			t.Errorf("msg = %v, want %q", decoded["msg"], "test message")
		}
		if decoded["key"] != "value" {
			t.Errorf("key = %v, want %q", decoded["key"], "value")
		}
	})

	t.Run("error level filters info messages", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := buildLogger("json", "error", &buf)
		if err != nil {
			t.Fatalf("buildLogger: %v", err)
		}
		logger.Info("should be filtered")
		if buf.Len() != 0 {
			t.Errorf("expected no output at error level for an Info call, got: %s", buf.Bytes())
		}
	})
}
