package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dop251/goja"
)

// defaultJSTimeout bounds a single condition/function evaluation.
const defaultJSTimeout = 5 * time.Second

// evalJS runs a JavaScript expression or script body against a context object
// and returns the exported Go value. The `ctx` object is injected as a global
// `context` variable, matching sim's condition/function evaluation where the
// upstream output is available as `context`.
//
// It mirrors the safety model used by internal/hooks/handlers/script.go: a
// watchdog interrupts the runtime on context cancellation, and panics are
// recovered into errors.
func evalJS(ctx context.Context, body string, contextObj map[string]any, extraGlobals map[string]any) (any, error) {
	rt := goja.New()
	if err := rt.Set("context", contextObj); err != nil {
		return nil, err
	}
	for k, v := range extraGlobals {
		if err := rt.Set(k, v); err != nil {
			return nil, err
		}
	}

	// Watchdog: interrupt on cancellation; done closes on normal exit.
	runCtx, cancel := context.WithTimeout(ctx, defaultJSTimeout)
	defer cancel()
	done := make(chan struct{})
	go func() {
		select {
		case <-runCtx.Done():
			rt.Interrupt("context cancelled")
		case <-done:
		}
	}()

	var (
		result  goja.Value
		execErr error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				execErr = fmt.Errorf("js panic: %v", r)
			}
		}()
		result, execErr = rt.RunString(body)
	}()
	close(done)

	if execErr != nil {
		if runCtx.Err() != nil {
			return nil, fmt.Errorf("js timeout: %w", runCtx.Err())
		}
		return nil, execErr
	}
	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return nil, nil
	}
	return result.Export(), nil
}

// evalBool wraps an expression so its result is coerced to a boolean, used by
// condition edges. Mirrors sim's `return Boolean(<expr>)`.
func evalBool(ctx context.Context, expr string, contextObj map[string]any) (bool, error) {
	if expr == "" {
		return false, errors.New("empty condition expression")
	}
	v, err := evalJS(ctx, "Boolean("+expr+")", contextObj, nil)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("condition did not evaluate to bool: %T", v)
	}
	return b, nil
}

// EvalBoolPublic exposes boolean JS evaluation to the dag package for loop
// while/doWhile conditions. It mirrors evalBool.
func EvalBoolPublic(ctx context.Context, expr string, contextObj map[string]any) (bool, error) {
	return evalBool(ctx, expr, contextObj)
}
