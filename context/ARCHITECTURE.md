# Architecture

This document describes the architectural patterns governing this codebase. It is
intended as grounding context for AI coding tools and human contributors alike. When
generating or reviewing code that involves chained operations, error propagation, or
multi-step data transformation, follow the guidance here.

---

## Core Concept

Go's `(value, error)` multiple return convention is a native expression of the
**Result monad** — a pattern where each operation either produces a value for the
next step or short-circuits with an error. This codebase formalizes that pattern
into a reusable, generic pipeline type.

The monadic pipeline has three properties:

1. **Each step has one job.** A step function takes a value, does one thing, and
   returns a transformed value or an error. It knows nothing about what came before
   or after it.
2. **The pipeline owns control flow.** Short-circuit logic (`if err != nil`) lives
   in the pipeline runner, not in individual steps.
3. **Composition is external.** The order and selection of steps is declared at
   the call site, not hardcoded inside the steps themselves.

---

## The Generic Pipeline Type

The canonical pipeline implementation lives in `internal/pipeline/pipeline.go`:

```go
package pipeline

// Step is a single transform in a pipeline.
// It takes a value of type T, performs an operation, and returns
// a transformed value or an error. Steps must be pure and focused
// on a single responsibility.
type Step[T any] func(T) (T, error)

// Pipeline is an ordered sequence of Steps.
type Pipeline[T any] struct {
    steps []Step[T]
}

// New constructs a Pipeline from an ordered list of Steps.
func New[T any](steps ...Step[T]) *Pipeline[T] {
    return &Pipeline[T]{steps: steps}
}

// Run executes each Step in order, passing the output of one as the
// input to the next. It short-circuits and returns on the first error.
func (p *Pipeline[T]) Run(input T) (T, error) {
    var err error
    for _, step := range p.steps {
        input, err = step(input)
        if err != nil {
            return input, err
        }
    }
    return input, nil
}

// Add returns a new Pipeline with the given Step appended.
// The original Pipeline is not modified (value semantics).
func (p *Pipeline[T]) Add(step Step[T]) *Pipeline[T] {
    newSteps := make([]Step[T], len(p.steps)+1)
    copy(newSteps, p.steps)
    newSteps[len(p.steps)] = step
    return &Pipeline[T]{steps: newSteps}
}
```

---

## How to Write a Step

A step is any function matching `func(T) (T, error)`. Steps must:

- **Do one thing.** Fetch, validate, enrich, transform, or persist — not combinations.
- **Be independently testable.** A step should be callable in a test without
  needing to run the rest of the pipeline.
- **Not call other steps.** A step must not internally invoke the next step in the
  chain. That coupling is the responsibility of the pipeline, not the step.
- **Wrap errors with context.** Use `fmt.Errorf("stepName: %w", err)` so error
  chains are traceable.

```go
// Good — single responsibility, no knowledge of pipeline neighbors
func validateUser(user User) (User, error) {
    if user.Email == "" {
        return user, fmt.Errorf("validateUser: missing email")
    }
    user.Valid = true
    return user, nil
}

// Bad — step is calling the next step, creating tight coupling
func validateUser(user User) (User, error) {
    if user.Email == "" {
        return user, fmt.Errorf("validateUser: missing email")
    }
    user.Valid = true
    return saveUser(user) // wrong — not this function's responsibility
}
```

---

## How to Compose a Pipeline

Pipelines are composed at the call site — in a factory function or application
bootstrap — not inside domain logic:

```go
func newUserPipeline() *pipeline.Pipeline[User] {
    return pipeline.New(
        fetchUser,
        validateUser,
        enrichUser,
        saveUser,
    )
}
```

To extend a pipeline for a specific context without modifying the base:

```go
auditedPipeline := newUserPipeline().Add(auditUser)
```

---

## Value vs Pointer Semantics

Steps should use **value types** by default:

```go
// Preferred — each step receives a copy, no shared mutation
func validateUser(user User) (User, error) { ... }
```

Use pointer types only when:
- The struct is large enough that copying is a measurable performance concern.
- The step must communicate mutations back through an interface boundary.

When using pointers, ensure nil is checked at the top of each step:

```go
func validateUser(user *User) (*User, error) {
    if user == nil {
        return nil, errors.New("validateUser: nil user")
    }
    ...
}
```

---

## When to Use a Pipeline vs Plain Sequential Calls

Use the pipeline type when:

- Three or more sequential steps operate on the same type.
- Steps are reused across multiple pipelines.
- Steps need to be swapped, mocked, or reordered (e.g., in tests).
- The pipeline itself is a configurable or injectable dependency.

Use plain sequential calls (`result, err := f(x); if err != nil { ... }`) when:

- There are only two steps.
- The sequence is a one-off and will never change.
- The functions are unexported and local to a single file.

---

## When Tight Coupling Is Acceptable

There are cases where a step calling the next step directly is intentional and correct:

- **Invariant enforcement.** If `saveUser` must never be called without `validateUser`
  having run first as a hard business rule, encoding that sequence in the call chain
  makes the invariant impossible to violate by construction.
- **Safety-critical or defensive code.** In auth flows, financial transactions, or
  compliance-sensitive paths, prescriptive coupling is a deliberate guardrail, not
  a design smell.
- **Internal-only, single-use sequences.** If the functions are unexported and used
  in exactly one place, the loose coupling overhead is not worth it.

When tight coupling is intentional, document it explicitly with a comment:

```go
// fetchUser calls validateUser directly — this sequence is a business invariant.
// validateUser must always run before any user record is returned to callers.
func fetchUser(id int) (User, error) {
    ...
    return validateUser(user)
}
```

---

## Error Handling Convention

All steps wrap errors with their function name:

```go
return user, fmt.Errorf("validateUser: %w", err)
```

Pipeline runners wrap errors with the pipeline or operation name:

```go
user, err := p.Run(input)
if err != nil {
    return fmt.Errorf("processUser: %w", err)
}
```

This produces readable error chains:

```
processUser: validateUser: missing email
```

---

## Testing Steps in Isolation

Because steps are decoupled, each is independently unit-testable:

```go
func TestValidateUser(t *testing.T) {
    tests := []struct {
        name    string
        input   User
        wantErr bool
    }{
        {"valid user", User{Email: "a@b.com"}, false},
        {"missing email", User{Email: ""}, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := validateUser(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("got err=%v, wantErr=%v", err, tt.wantErr)
            }
        })
    }
}
```

Testing the pipeline itself focuses on composition and ordering, not step logic:

```go
func TestPipelineShortCircuits(t *testing.T) {
    failStep := func(u User) (User, error) {
        return u, errors.New("intentional failure")
    }
    neverReached := func(u User) (User, error) {
        t.Fatal("this step should not have been called")
        return u, nil
    }

    p := pipeline.New(failStep, neverReached)
    _, err := p.Run(User{})
    if err == nil {
        t.Error("expected error, got nil")
    }
}
```

---

## Summary

| Concern | Approach |
|---|---|
| Sequential transforms | `pipeline.New(step1, step2, ...)` |
| Single responsibility | One function, one job, no neighbor knowledge |
| Error propagation | `fmt.Errorf("stepName: %w", err)` |
| Extensibility | `p.Add(newStep)` without modifying existing steps |
| Tight coupling | Acceptable for invariants; document the intent |
| Generics scope | Pipeline type lives in a shared package; business logic stays plain |
