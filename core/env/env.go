// SPDX-License-Identifier: Apache-2.0

// Package env resolves configuration environment variables shared by
// muto-operator and muto-mcp.
package env

import "os"

// OrDefault returns the value of the environment variable key, or fallback
// if it is unset or empty. It returns the value verbatim otherwise,
// including values like "0" that look falsy but are meaningful (e.g.
// MUTO_METRICS_BIND_ADDRESS=0 disables the metrics server).
func OrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
