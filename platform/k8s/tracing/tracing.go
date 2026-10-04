// SPDX-License-Identifier: Apache-2.0
package tracing

import (
	"context"

	"github.com/muto-io/muto/core/tracing"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// WrapReconciler returns a reconcile.Reconciler that records a span named
// name around every call to inner.Reconcile.
func WrapReconciler(name string, inner reconcile.Reconciler) reconcile.Reconciler {
	return reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
		return tracing.Wrap(ctx, name, func(ctx context.Context) (reconcile.Result, error) {
			return inner.Reconcile(ctx, req)
		})
	})
}
