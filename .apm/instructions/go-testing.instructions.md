---
description: 'Cross-project Go testing instructions: testify assertions, executable examples, and Test Desiderata 2.0'
applyTo: '**/*.go'
---

# Go Testing Instructions

## Assertions: Use testify

Use `github.com/stretchr/testify` for all test assertions. Do not use bare `t.Error`, `t.Fatal`, or manual comparisons when testify covers the case.

- Use `github.com/stretchr/testify/assert` for non-fatal assertions (test continues after failure).
- Use `github.com/stretchr/testify/require` for fatal assertions (test stops immediately on failure). Prefer `require` for setup and preconditions.
- Use `github.com/stretchr/testify/mock` for mock objects when interface substitution is needed.

```go
import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestFoo(t *testing.T) {
    result, err := Foo()
    require.NoError(t, err)
    assert.Equal(t, "expected", result)
}
```

Prefer specific assertions over generic ones (`assert.Equal` over `assert.True(t, a == b)`), as they produce clearer failure messages.

## Executable Examples

All Go code examples must be executable and written using the `func Example...` convention from the `testing` package. Place example functions in `*_test.go` files.

- Name examples `ExampleFunctionName`, `ExampleType_MethodName`, or `ExamplePackage` following Go conventions.
- Include an `// Output:` comment so `go test` verifies the output automatically.
- Keep examples self-contained and minimal — they serve as documentation.

```go
func ExampleGreet() {
    fmt.Println(Greet("world"))
    // Output:
    // hello, world
}
```

Do not write example code in comments, README snippets, or prose without a corresponding runnable `func Example...` counterpart in a `_test.go` file.

## Test Desiderata 2.0

Write tests that satisfy the properties from [Test Desiderata 2.0](https://coding-is-like-cooking.info/2025/12/test-desiderata-2-0/). These are grouped by the outcome they support.

### Predict Success in Production

- **Sensitive to Behaviour** — tests must detect functional regressions. Test observable outputs and side effects, not internal implementation details.
- **Sensitive to Execution Qualities** — where relevant, cover non-functional concerns such as latency bounds, concurrency safety (`-race`), and resource cleanup.

### Fast Feedback

- **Minimal Data** — use the smallest dataset that exercises the behaviour. Avoid large fixtures when a single representative value will do.
- **Run in Any Order** — tests must be fully independent. Never rely on global state left by another test; use `t.Cleanup` or reset state explicitly.
- **Run in Parallel** — call `t.Parallel()` in tests that are safe to run concurrently. Design tests to be parallelisable by default.

### Low Total Cost of Ownership

- **Automated** — every test must run via `go test ./...` with no manual steps.
- **Deterministic** — tests must produce the same result on every run. Eliminate time dependencies, random seeds, and environment-specific assumptions.
- **Diagnosable** — a failing test must clearly communicate what went wrong. Use testify's descriptive messages and `t.Run` subtests with meaningful names so failures are easy to locate and understand.
- **Easy to Read** — test code is documentation. Prefer readability over brevity. Use table-driven tests and `t.Run` to express intent clearly.
- **Easy to Update** — write tests against the public API and observable behaviour, not private functions or implementation internals, so refactoring does not require rewriting tests.
- **Easy to Write** — reduce friction: shared helpers go in `testdata` or helper functions marked with `t.Helper()`. Reuse setup with `t.Cleanup` rather than duplicating teardown.
- **Insensitive to Code Structure** — tests must not break when the code is refactored without changing behaviour. Avoid testing private functions directly.

### Support Ongoing Code Design Change

- **Composable** — structure tests so subtests and helpers can be combined without duplication.
- **Documents Intent** — test names and assertions should describe what the code is supposed to do, not how it does it. A reader should understand expected behaviour from the test alone.
- **Durable** — write tests that remain valid as the product evolves. Avoid asserting on incidental details that are likely to change.
- **Necessary** — every test must guide a development decision or prevent a real regression. Delete tests that duplicate coverage without adding signal.
- **Organized** — place test files alongside the code they test. Use `_test.go` suffix. One test file per source file is a reasonable default.
- **Positive Design Pressure** — if a piece of code is hard to test, treat that as a design signal. Refactor toward testable, decoupled interfaces rather than reaching for mocks of concrete types.
