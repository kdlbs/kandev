package gitlab

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// noopClientNonErroringMethods are the only NoopClient methods that must NOT
// report ErrNoClient. They answer questions that have a meaningful answer even
// with no GitLab configured, and callers branch on those answers rather than on
// an error.
//
// GetProtectedBranch is the subtle one: (nil, nil) is not "no data", it is the
// affirmative answer "this branch is not protected". PATClient returns exactly
// that for a 404 (see pat_client_actions.go), so NoopClient reporting
// ErrNoClient here would make the unconfigured path *more* restrictive than a
// real GitLab that simply has no protection rule.
var noopClientNonErroringMethods = map[string]struct{}{
	"Host":                 {},
	"IsAuthenticated":      {},
	"GetAuthenticatedUser": {},
	"GetProtectedBranch":   {},
}

// TestNoopClientDegradesUniformly is the contract for the unconfigured-GitLab
// path: every data method must return ErrNoClient rather than a nil error with
// a nil result, because callers that see (nil, nil) dereference it. Driving the
// method set by reflection rather than a hand-written list means a method added
// to the Client interface cannot quietly skip this guarantee, and it also
// proves no method panics on zero-valued arguments.
func TestNoopClientDegradesUniformly(t *testing.T) {
	client := NewNoopClient("")
	clientType := reflect.TypeOf(client)
	ctxValue := reflect.ValueOf(context.Background())

	exercised := 0
	for i := range clientType.NumMethod() {
		method := clientType.Method(i)
		if !method.IsExported() {
			continue
		}
		t.Run(method.Name, func(t *testing.T) {
			results := method.Func.Call(noopCallArgs(reflect.ValueOf(client), method.Type, ctxValue))
			err := trailingError(results)

			if _, exempt := noopClientNonErroringMethods[method.Name]; exempt {
				if err != nil {
					t.Fatalf("%s returned %v, want no error", method.Name, err)
				}
				return
			}
			if !errors.Is(err, ErrNoClient) {
				t.Fatalf("%s returned err = %v, want ErrNoClient", method.Name, err)
			}
			// A method that reports ErrNoClient must not also hand back a
			// half-built value the caller might use.
			for j := range len(results) - 1 {
				if result := results[j]; result.IsValid() && !result.IsZero() {
					t.Errorf("%s returned a non-zero value %v alongside ErrNoClient",
						method.Name, result.Interface())
				}
			}
		})
		exercised++
	}
	if exercised == 0 {
		t.Fatal("no exported methods were exercised; the reflection walk is broken")
	}
}

// noopCallArgs builds the receiver plus zero-valued arguments for one method,
// substituting the real context for the first context.Context parameter.
func noopCallArgs(receiver reflect.Value, methodType reflect.Type, ctxValue reflect.Value) []reflect.Value {
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	args := make([]reflect.Value, 0, methodType.NumIn())
	args = append(args, receiver)
	for i := 1; i < methodType.NumIn(); i++ {
		paramType := methodType.In(i)
		if paramType == ctxType {
			args = append(args, ctxValue)
			continue
		}
		args = append(args, reflect.Zero(paramType))
	}
	return args
}

// trailingError extracts the error from a call's results, or nil when the
// method does not return one.
func trailingError(results []reflect.Value) error {
	if len(results) == 0 {
		return nil
	}
	last := results[len(results)-1]
	if last.Type() != reflect.TypeOf((*error)(nil)).Elem() {
		return nil
	}
	if last.IsNil() {
		return nil
	}
	return last.Interface().(error)
}

// TestNoopClientAuthAnswersAreExplicitlyNegative pins the values behind the
// three exemptions above: "not authenticated, no username" is the answer the
// status endpoint renders, and it must be a definite negative rather than an
// error the caller has to interpret.
func TestNoopClientAuthAnswersAreExplicitlyNegative(t *testing.T) {
	client := NewNoopClient("https://gitlab.acme.test")
	ctx := context.Background()

	authenticated, err := client.IsAuthenticated(ctx)
	if err != nil || authenticated {
		t.Errorf("IsAuthenticated = (%v, %v), want (false, nil)", authenticated, err)
	}
	username, err := client.GetAuthenticatedUser(ctx)
	if err != nil || username != "" {
		t.Errorf("GetAuthenticatedUser = (%q, %v), want (\"\", nil)", username, err)
	}
	if host := client.Host(); host != "https://gitlab.acme.test" {
		t.Errorf("Host = %q, want the configured host reported even while unauthenticated", host)
	}
}

// TestNoopClientReportsBranchesAsUnprotected pins the exemption above: the
// unconfigured client must give the same "not protected" answer a real GitLab
// gives for a branch with no protection rule, not an error.
func TestNoopClientReportsBranchesAsUnprotected(t *testing.T) {
	branch, err := NewNoopClient("").GetProtectedBranch(context.Background(), "group/p", "main")

	if err != nil {
		t.Fatalf("GetProtectedBranch err = %v, want nil", err)
	}
	if branch != nil {
		t.Errorf("protected branch = %+v, want nil (not protected)", branch)
	}
}
