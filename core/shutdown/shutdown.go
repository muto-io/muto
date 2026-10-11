// SPDX-License-Identifier: Apache-2.0

package shutdown

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

// Func is a named shutdown callback
type Func struct {
	Name string
	Run  func(context.Context) error
}

// All runs every fn concurrently under one shared timeout, so N shutdowns
// share a single deadline instead of each getting their own.
func All(ctx context.Context, log logr.Logger, timeout time.Duration, fns ...Func) {
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, fn := range fns {
		wg.Add(1)
		go func(fn Func) {
			defer wg.Done()
			if err := fn.Run(shutdownCtx); err != nil {
				log.Error(err, fn.Name+" shutdown failed")
			}
		}(fn)
	}
	wg.Wait()
}
