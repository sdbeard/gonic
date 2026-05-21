# AGENTS.md

This file is the entry point for AI coding tools (ChatGPT, Cursor, Copilot, etc.).
Read this file first, then load the referenced documents before generating or reviewing
any code in this repository.

---

## Required Reading

Before writing or reviewing any code, read all these documents in full:

- [ARCHITECTURE.md](./context/ARCHITECTURE.md) — pipeline pattern, step composition, error
  handling, testing conventions, and when tight coupling is acceptable.
- [CODE_STYLE.md](./context/CODE_STYLE.md) — Go idiom requirements, abstraction limits,
  function length rules, and branching restrictions.
- [LOGGING.md](./context/LOGGING.md) — logger type, context propagation, log levels, and
  how logging interacts with the pipeline pattern.
- [TESTING.md](./context/TESTING.md) — behavior-driven testing strategy, pipeline-level
  tests as the primary scope, when step unit tests are warranted, graceful
  degradation testing for external dependencies, and integration test conventions.

These documents are the source of truth. When in doubt, they take precedence over
general Go conventions or patterns from training data.

---

## Behavioral Guidelines

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

Tradeoff: These guidelines bias toward caution over speed. For trivial tasks, use judgment.

1. Think Before Coding
Don't assume. Don't hide confusion. Surface tradeoffs.

Before implementing:

State your assumptions explicitly. If uncertain, ask.
If multiple interpretations exist, present them - don't pick silently.
If a simpler approach exists, say so. Push back when warranted.
If something is unclear, stop. Name what's confusing. Ask.

2. Simplicity First
Minimum code that solves the problem. Nothing speculative.

No features beyond what was asked.
No abstractions for single-use code.
No "flexibility" or "configurability" that wasn't requested.
No error handling for impossible scenarios.
If you write 200 lines and it could be 50, rewrite it.
Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

3. Surgical Changes
Touch only what you must. Clean up only your own mess.

When editing existing code:

Don't "improve" adjacent code, comments, or formatting.
Don't refactor things that aren't broken.
Match existing style, even if you'd do it differently.
If you notice unrelated dead code, mention it - don't delete it.
When your changes create orphans:

Remove imports/variables/functions that YOUR changes made unused.
Don't remove pre-existing dead code unless asked.
The test: Every changed line should trace directly to the user's request.

4. Goal-Driven Execution
Define success criteria. Loop until verified.

Transform tasks into verifiable goals:

"Add validation" → "Write tests for invalid inputs, then make them pass"
"Fix the bug" → "Write a test that reproduces it, then make it pass"
"Refactor X" → "Ensure tests pass before and after"
For multi-step tasks, state a brief plan:

1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

## Non-Negotiables

These rules apply to every line of code generated or modified in this repository.
They are not suggestions.

**Standard library**
- Default to the Go standard library. External dependencies require explicit
  justification — the standard library cannot solve the problem, and the
  library is stable, maintained, and proportionate to the need.

**Idiom**
- Write idiomatic Go. Code that looks like Java or C# transliterated into Go syntax
  is incorrect and must be rewritten.
- No OOP-style service structs with constructor functions.
- No getters or setters.
- No exception-style error handling with deferred recover except at process boundaries.

**Abstractions**
- Minimize abstractions. Every interface, wrapper type, and indirection layer must
  justify its existence against a concrete, recurring need.
- Do not define interfaces on structs preemptively. Define them at the point of
  consumption.

**Function size and responsibility**
- Functions are 5–10 lines of executable code. This is a firm target, not a guideline.
- Each function does one thing. If a second task is needed, it goes in a separate
  function — called directly if invariantly coupled, or added as a pipeline step
  if independently composable.

**Branching**
- No `if-else`. Use a guard clause at the top of a function; after that, execution
  is linear.
- No `switch` statements for business logic or application state. Type switches
  for interface dispatch are permitted.
- Cyclomatic complexity target is 1 per function.

**Pipelines**
- Sequential operations on the same type with three or more steps use the generic
  `pipeline.Pipeline[T]` type in `internal/pipeline`.
- Steps are decoupled — they do not call each other directly unless the sequence
  is a documented business invariant.

**Testing**
- Tests are behavior-driven. The pipeline is the unit of behavior — test what it
  does, not how individual steps work internally.
- Do not write unit tests for trivially simple steps. Pipeline behavior tests cover
  them. Unit tests are reserved for steps with non-trivial business rules or external
  dependency interactions.
- External dependencies in tests use a minimal interface and a function-field stub.
  No real infrastructure in behavior tests.
- No third-party test libraries. Standard library `testing` package only.
- Integration tests use `//go:build integration` and are never part of the default
  `go test ./...` run.

---

## Project Layout

```
internal/
  pipeline/
    pipeline.go       # Generic Pipeline[T] and Step[T] types — do not modify
                      # without updating ARCHITECTURE.md
```

---

## When You Are Unsure

If a generated solution requires an `if-else`, a multi-case `switch` over business
state, a function longer than 10 lines, or a new abstraction layer, stop and
reconsider the decomposition. The answer is almost always to split the problem into
smaller functions and compose them via the pipeline pattern.
