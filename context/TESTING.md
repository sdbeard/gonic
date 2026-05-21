# Testing

This document defines the testing strategy for this codebase. It is intended as
grounding context for AI coding tools and human contributors alike. When generating
or reviewing tests, follow the guidance here.

---

## Testing Philosophy

This codebase uses a **behavior-driven testing strategy**. The goal of a test is to
verify that the system does the right thing — not to verify that individual functions
exist and execute. Tests capture logic errors, especially as the system grows, and
provide confidence that observable behavior is correct and stable.

The pipeline architecture directly supports this strategy. Because functions are short
and single-purpose, the complexity of the system lives in its composition — in the
pipeline — not inside individual steps. This means:

- **The pipeline is the unit of behavior.** Testing a pipeline end-to-end exercises
  all of its steps in the correct order, with the correct error propagation, in a
  single test. This is more valuable than testing each step in isolation.
- **Unit tests on simple steps have diminishing returns.** A five-line function with
  one guard clause and one transformation does not need its own test. The pipeline
  behavior test already covers it. Writing a unit test for it tests the Go compiler
  more than it tests the system.
- **Tests should catch logic errors at scale.** As the system grows, behavior tests
  at the pipeline boundary catch regressions that unit tests on individual steps
  cannot — because most logic errors emerge from how steps interact, not from what
  any single step does in isolation.

The overall goal is to reduce the testing burden on the system by reducing the
surface area that needs testing. Short, single-purpose functions with no branching
are not the testing risk. The risk is in composition, external dependencies, and
the behavior the system exposes to its callers.

---

## The Testing Hierarchy

Tests in this codebase are organized into two scopes. Behavior tests and integration
tests are the same concept described at different levels — both verify observable
outcomes rather than implementation details. The distinction is whether real
infrastructure is involved.

### 1. Behavior Tests (primary)

Behavior tests verify what a pipeline does from the outside — its inputs, its
outputs, and its error handling — without reference to how any individual step is
implemented internally. They are in-process, fast, and use test doubles for external
dependencies.

This is the primary testing scope. Most tests in the codebase should be behavior
tests at the pipeline level.

```go
// Behavior test — verifies what the order processing pipeline does,
// not how any individual step works internally
func TestOrderPipeline(t *testing.T) {
    tests := []struct {
        name    string
        input   Order
        want    Order
        wantErr bool
    }{
        {
            name:  "valid order is processed with tax applied and marked complete",
            input: Order{ID: 1, Total: 100.00},
            want:  Order{ID: 1, Total: 108.00, Tax: 8.00, Processed: true},
        },
        {
            name:    "order with missing ID is rejected before any processing occurs",
            input:   Order{ID: 0, Total: 100.00},
            wantErr: true,
        },
        {
            name:    "order with negative total is rejected before tax is applied",
            input:   Order{ID: 1, Total: -50.00},
            wantErr: true,
        },
    }

    store := &stubOrderStore{saveFn: func(_ context.Context, o Order) error {
        return nil
    }}

    p := newOrderPipeline(store)

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := p.Run(context.Background(), tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("pipeline error = %v, wantErr %v", err, tt.wantErr)
            }
            if !tt.wantErr && got != tt.want {
                t.Errorf("pipeline result = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### 2. Integration Tests (infrastructure boundary)

Integration tests verify the same behaviors as behavior tests but against real
infrastructure — a real database, a real HTTP service, a real message queue. They
confirm that the system's external dependency wiring is correct and that the
infrastructure behaves as expected under real conditions.

Integration tests are behavior tests at a wider scope. They do not test new logic —
they test that the logic verified by behavior tests holds when the stubs are replaced
with real systems.

Integration tests use the `//go:build integration` build tag and live in files
suffixed with `_integration_test.go`. They are never run as part of the default
`go test ./...` invocation:

```go
//go:build integration

package order_test

// TestOrderPipelineIntegration verifies the full order processing pipeline
// against a real Postgres instance. Requires DATABASE_URL in the environment.
func TestOrderPipelineIntegration(t *testing.T) {
    db := connectTestDatabase(t)
    store := newPostgresOrderStore(db)
    p := newOrderPipeline(store)

    got, err := p.Run(context.Background(), Order{ID: 1, Total: 100.00})
    if err != nil {
        t.Fatalf("pipeline failed against real database: %v", err)
    }
    if !got.Processed {
        t.Error("expected order to be marked processed")
    }
}
```

Run integration tests explicitly:

```bash
go test -tags integration ./...
```

---

## When to Write a Unit Test on a Step

Unit tests on individual pipeline steps are not the default. They are written only
in two specific situations:

### 1. The step encodes a non-trivial business rule

If a step contains logic that is genuinely complex — a calculation with edge cases,
a transformation that could be wrong in subtle ways — a focused unit test on that
step clarifies the rule and catches regressions precisely.

```go
// Worth a unit test — the tax calculation has edge cases worth specifying explicitly
func TestApplyTax(t *testing.T) {
    tests := []struct {
        name  string
        input Order
        want  Order
    }{
        {
            name:  "standard rate applied to positive total",
            input: Order{Total: 100.00},
            want:  Order{Total: 108.00, Tax: 8.00},
        },
        {
            name:  "zero total produces zero tax without error",
            input: Order{Total: 0},
            want:  Order{Total: 0, Tax: 0},
        },
    }
    ...
}
```

### 2. The step interacts with an external dependency

Any step that calls a database, an HTTP API, a file system, or any other external
system must have unit tests that verify graceful degradation. These tests use an
interface and a test double to simulate failure modes that the pipeline behavior
test cannot easily exercise.

Failure modes to test at the external dependency boundary:

- The dependency is unavailable (connection refused, timeout).
- The dependency returns an error response.
- The dependency responds slowly — use a stub that blocks on a context and verify
  that cancellation propagates correctly.
- The dependency returns malformed or unexpected data.

```go
func TestSaveOrder_DependencyFailures(t *testing.T) {
    tests := []struct {
        name    string
        storeFn func(context.Context, Order) error
        wantErr bool
    }{
        {
            name: "database unavailable returns error without panic",
            storeFn: func(_ context.Context, _ Order) error {
                return errors.New("connection refused")
            },
            wantErr: true,
        },
        {
            name: "context cancellation is respected and propagated",
            storeFn: func(ctx context.Context, _ Order) error {
                <-ctx.Done()
                return ctx.Err()
            },
            wantErr: true,
        },
        {
            name: "slow dependency times out when context deadline is exceeded",
            storeFn: func(ctx context.Context, _ Order) error {
                select {
                case <-time.After(10 * time.Second):
                    return nil
                case <-ctx.Done():
                    return ctx.Err()
                }
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
            defer cancel()

            store := &stubOrderStore{saveFn: tt.storeFn}
            _, err := saveOrder(ctx, store, Order{ID: 1})
            if (err != nil) != tt.wantErr {
                t.Errorf("saveOrder() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

---

## Test File Organization

Tests are organized into two categories based on what they can see:

### White-box tests — same package

White-box tests have access to unexported identifiers and are used for the step-level
unit tests described above — cases where internal behavior of a specific step is
being verified directly.

```
internal/order/
  pipeline.go         // package order
  pipeline_test.go    // package order  ← same package, access to unexported identifiers
```

### Black-box tests — `_test` package

Black-box tests exercise only the exported API. All behavior tests at the pipeline
level are black-box tests — they verify the public contract exactly as a caller would.

```
internal/order/
  pipeline.go                  // package order
  pipeline_behavior_test.go    // package order_test  ← exported API only
```

---

## Handling External Dependencies in Tests

External dependencies must not appear in behavior tests or step unit tests. Use a
minimal interface defined at the point of consumption and a test double that
satisfies it.

```go
// Interface defined where it is consumed
type OrderStore interface {
    Save(ctx context.Context, order Order) error
}

// Test double defined in the test file
type stubOrderStore struct {
    saveFn func(ctx context.Context, order Order) error
}

func (s *stubOrderStore) Save(ctx context.Context, order Order) error {
    return s.saveFn(ctx, order)
}
```

The function field pattern (`saveFn func(...)`) allows each test case to define
exactly the behavior it needs without requiring a separate stub type per scenario.

---

## Table-Driven Tests

Table-driven tests are the standard structure for behavior tests and for step unit
tests with multiple cases. They make the set of covered scenarios explicit and
readable, and make it straightforward to add new cases as the system evolves.

Test case names are plain English descriptions of observable behavior — what the
system does, not what function is being called:

```go
// Bad — describes implementation, not behavior
name: "validateOrder called with zero ID"

// Good — describes what the system does
name: "order with missing ID is rejected before processing begins"
```

Single-case tests are permitted for pipeline invariants and property-style tests
where a table adds ceremony without clarity.

---

## Error Checking

Tests must check errors explicitly. Use `t.Fatalf` when continuing after a failure
would produce misleading results. Use `t.Errorf` when the test should continue to
collect additional failures.

```go
// Bad — error ignored
got, _ := p.Run(ctx, input)

// Bad — test continues in a broken state after unexpected error
if err != nil {
    t.Errorf("unexpected error: %v", err)
}
// got may be a zero value here — subsequent assertions are meaningless

// Good — stop immediately when an unexpected error occurs
got, err := p.Run(ctx, input)
if err != nil {
    t.Fatalf("pipeline.Run() unexpected error: %v", err)
}
```

---

## What Not to Do

- **Do not write unit tests for trivially simple steps.** A guard clause plus a
  single transformation does not need its own test. The pipeline behavior test
  covers it. Writing a unit test for it produces noise without signal.
- **Do not use third-party test libraries.** No `testify`, no `gomock`, no `godog`.
  The standard `testing` package is sufficient and keeps the dependency tree clean.
- **Do not test implementation details.** If a refactor that preserves behavior
  breaks a test, the test is wrong.
- **Do not share mutable state between test cases.** Each test case must be fully
  independent. Shared state causes order-dependent failures that are difficult
  to diagnose.
- **Do not leave skipped tests without explanation.** `t.Skip()` must include a
  reason and a linked issue: `t.Skip("#42: flaky against CI database")`.
- **Do not use `init()` in test files.** Use `TestMain` if suite-level setup is
  genuinely required.

---

## Quick Reference

| Concern | Rule |
|---|---|
| Primary test scope | Pipeline level — test observable behavior, not individual steps |
| Behavior vs integration tests | Same concept at different scopes — in-process vs real infrastructure |
| Unit tests on steps | Only for non-trivial business rules or external dependency interactions |
| Graceful degradation | Tested at the external dependency boundary using stubs that simulate failure |
| External dependencies in tests | Minimal interface + function-field stub — no real infrastructure in behavior tests |
| Integration tests | `//go:build integration` tag; never in default `go test ./...` run |
| Test structure | Table-driven preferred; single-case permitted for invariants |
| Test case names | Plain English, behavior-focused — what the system does, not what function is called |
| Test library | Standard library `testing` package only |
| Error checking | Always explicit — `t.Fatalf` when continuing after failure is misleading |
| White-box tests | Same package — for step unit tests accessing unexported identifiers |
| Black-box tests | `_test` package — for all pipeline behavior tests against exported API |
