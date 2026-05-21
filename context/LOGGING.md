# Logging

This document defines the logging standards for this codebase. It is intended as
grounding context for AI coding tools and human contributors alike. When generating
or reviewing code that produces log output, follow the guidance here.

---

## Default Logger

The standard logging mechanism is `*slog.Logger` from the Go standard library
(`log/slog`, available since Go 1.21). No third-party logging libraries are to be
introduced unless there is a documented, compelling reason that `slog` cannot satisfy.

`slog` is the correct choice because:
- It is part of the standard library — no dependency, no version drift.
- It is structured by design — key-value attributes are first-class.
- It is level-aware — production log level configuration is handled at initialization,
  not scattered through the code.
- It satisfies Go idiom — it does not require a service struct or constructor pattern.

---

## How the Logger Is Passed

The logger is stored in and retrieved from `context.Context`. It is never passed as
a direct function parameter, stored in a package-level variable, or attached to a
struct.

### Storing the logger in context

At the application entry point or request boundary, initialize the logger and store
it in the context before passing it into the call chain:

```go
func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo, // production: Info and above only
    }))

    ctx := context.WithValue(context.Background(), loggerKey, logger)
    run(ctx)
}
```

### Retrieving the logger from context

Define a single canonical helper for retrieving the logger. This is the only place
a fallback to the default logger is permitted:

```go
type contextKey string

const loggerKey contextKey = "logger"

// LoggerFrom retrieves the slog.Logger from ctx.
// If no logger is present, it returns the default slog logger as a safe fallback.
func LoggerFrom(ctx context.Context) *slog.Logger {
    if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
        return logger
    }
    return slog.Default()
}
```

All code that needs to log retrieves the logger this way — no other retrieval
mechanism is permitted:

```go
func validateUser(ctx context.Context, user User) (User, error) {
    logger := LoggerFrom(ctx)
    logger.Debug("validateUser: entry", "userID", user.ID)
    if user.Email == "" {
        return user, fmt.Errorf("validateUser: missing email")
    }
    user.Valid = true
    return user, nil
}
```

---

## Log Levels

Four levels are available. Their usage is strict:

| Level | When to use |
|---|---|
| `Debug` | Development and diagnostic detail. Controlled by runtime configuration — never visible in production unless explicitly enabled. |
| `Info` | Significant, expected events in normal operation. Pipeline entry and exit, service start/stop, meaningful state transitions. |
| `Warn` | Unexpected but recoverable conditions. Something is off but the system can continue. |
| `Error` | A failure that prevented an operation from completing. Always accompanied by the error value as a structured attribute. |

### Production log level

Production initializes the logger at `slog.LevelInfo`. Debug output is suppressed
at the handler level — it is not stripped from the code. This means:

- Debug log calls are valid in any context, including steps.
- They have zero output cost in production because the handler discards them.
- They are available when the environment is configured to surface them (development,
  staging, incident investigation).

```go
// Production handler — Debug is silently discarded
slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})

// Development handler — all levels visible
slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
})
```

---

## Logging and the Pipeline Pattern

Steps are permitted to log, subject to the following rules:

- **Steps log at `Debug` level only.** A step's internal activity is diagnostic
  detail, not an operational event. Use Debug for entry, exit, and intermediate
  state within a step.
- **Pipeline boundaries log at `Info` level.** The start and successful completion
  of a pipeline is an operational event worth recording in production.
- **Errors are logged at the boundary, not inside the step.** A step returns an
  error — it does not log it. The caller (pipeline runner or boundary function)
  logs the error with context. This avoids duplicate log entries for the same failure.

```go
// Good — step logs diagnostic detail at Debug only
func validateUser(ctx context.Context, user User) (User, error) {
    logger := LoggerFrom(ctx)
    logger.Debug("validateUser: checking email", "userID", user.ID)
    if user.Email == "" {
        return user, fmt.Errorf("validateUser: missing email")
    }
    user.Valid = true
    logger.Debug("validateUser: passed", "userID", user.ID)
    return user, nil
}

// Good — pipeline boundary logs Info and Error
func processUser(ctx context.Context, id int) (User, error) {
    logger := LoggerFrom(ctx)
    logger.Info("processUser: starting", "userID", id)

    user, err := p.Run(ctx, User{ID: id})
    if err != nil {
        logger.Error("processUser: failed", "userID", id, "error", err)
        return user, fmt.Errorf("processUser: %w", err)
    }

    logger.Info("processUser: complete", "userID", id)
    return user, nil
}

// Bad — step logs an error it should be returning
func validateUser(ctx context.Context, user User) (User, error) {
    logger := LoggerFrom(ctx)
    if user.Email == "" {
        logger.Error("validateUser: missing email") // wrong — log at the boundary
        return user, fmt.Errorf("validateUser: missing email")
    }
    return user, nil
}
```

---

## Error Wrapping and Failure Tracing

Wrapped errors provide the failure path through the call chain. Each step that
returns an error must wrap it with the function name and any local values needed to
understand the failure:

```go
func openReadFile(state readState) (readState, error) {
    file, err := os.Open(state.Request.Path)
    if err != nil {
        return state, fmt.Errorf("openReadFile: path=%q: %w", state.Request.Path, err)
    }
    state.File = file
    return state, nil
}
```

The boundary logs the final wrapped error once, together with the operation state:

```go
func readFile(ctx context.Context, state readState) (readState, error) {
    logger := LoggerFrom(ctx)
    state, err := readPipeline.Run(state)
    if err != nil {
        logger.Error("readFile: failed",
            "path", state.Request.Path,
            "element", state.Result.ElementName,
            "count", state.Result.Count,
            "error", err,
        )
        return state, fmt.Errorf("readFile: %w", err)
    }
    return state, nil
}
```

This means:

- **Wrapped errors trace where the failure happened.** The final error message
  should read like a path through the failing operation.
- **Boundary logs capture runtime state.** The log entry records counts, IDs,
  paths, and other state that is useful at the moment the operation failed.
- **Steps do not log errors.** A step returns the wrapped error and lets the
  boundary create one authoritative error log entry.

Wrapped errors are not a replacement for distributed tracing. They do not provide
spans, timings, cross-service correlation, or sampling. They are the local failure
trace for a single in-process operation.

---

## Structured Attributes

`slog` supports structured key-value attributes on every log call. Their use is
encouraged where they add clarity, but not mandated on every log line.

Use structured attributes when:
- Logging an error — always include `"error", err`.
- Logging an entity operation — include the entity ID (e.g. `"userID", user.ID`).
- Logging a state transition — include before/after values where relevant.
- The message alone would be ambiguous without additional context.

```go
// Encouraged — attributes make the log entry actionable
logger.Error("saveUser: database write failed",
    "userID", user.ID,
    "error", err,
)

logger.Info("processOrder: complete",
    "orderID", order.ID,
    "total", order.Total,
    "duration", time.Since(start),
)

// Acceptable — message is self-contained and unambiguous
logger.Info("server: listening on :8080")
logger.Warn("cache: miss, falling back to database")
```

Do not include sensitive data in log attributes — no passwords, tokens, full credit
card numbers, or personally identifiable information beyond an opaque identifier
(e.g. a user ID is acceptable; an email address or name is not).

---

## Logger Initialization

The logger is initialized once at the application entry point and stored in the
root context. It is never re-initialized mid-application except to create a child
logger with additional attributes scoped to a specific operation:

```go
// Child logger — adds fixed attributes for the duration of a request
requestLogger := logger.With("requestID", req.ID, "userID", req.UserID)
ctx = context.WithValue(ctx, loggerKey, requestLogger)
```

Child loggers inherit the parent's handler and level configuration. Use them to
attach request-scoped or operation-scoped context that would otherwise be repeated
on every log call.

---

## What Not to Do

- **Do not use `log.Printf` or `fmt.Println` for logging.** All log output goes
  through `*slog.Logger`.
- **Do not create a package-level `var logger`.**  The logger lives in context.
- **Do not log and return an error.** Pick one. Steps return errors; boundaries log them.
- **Do not swallow errors silently with only a log call.** If an error is logged,
  it must also be returned or explicitly handled — a log line is not error handling.
- **Do not include sensitive data in log output** under any circumstances.

---

## Quick Reference

| Concern | Rule |
|---|---|
| Default logger type | `*slog.Logger` — standard library only |
| Logger availability | Retrieved from `context.Context` via `LoggerFrom(ctx)` |
| Production log level | `slog.LevelInfo` — Debug suppressed at handler, not in code |
| Debug in steps | Permitted — zero cost in production due to handler-level filtering |
| Error logging | At pipeline boundaries only — steps return errors, never log them |
| Failure tracing | Wrapped errors provide the local failure path; boundary logs provide operation state |
| Structured attributes | Encouraged on errors and entity operations; not mandated everywhere |
| Sensitive data | Never included in log output |
| Logger initialization | Once at entry point; child loggers via `logger.With(...)` for scoped context |
