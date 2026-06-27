---
description: "Comprehensive architecture review: generate a ranked findings document, then address findings systematically one commit at a time"
argument-hint: "[review|address [round]]"
allowed-tools: ["Bash", "Glob", "Grep", "Read", "Write", "Edit", "Task"]
---

# Architecture Review Workflow

Generate a comprehensive architecture review document, or systematically address findings from a prior review.

**Arguments:** "$ARGUMENTS"

---

## Phase 1: Generate Review (`architecture-review review`)

Perform a comprehensive architecture and engineering review of the current codebase and write the output to `claude-review-architecture.md`.

### Review Dimensions

Analyse the codebase across these dimensions:

- **Inconsistencies** — mismatched patterns, duplicate logic, divergent conventions
- **Correctness** — bugs, race conditions, error handling gaps, incorrect assumptions
- **Maintainability** — coupling, cohesion, complexity, testability
- **Modern practices** — alignment with current language idioms and community standards
- **Architecture** — apply hexagonal architecture thinking to identify areas of high coupling and candidates for restructuring

### Output Format

Write findings to `claude-review-architecture.md`:

1. Executive summary
2. Findings list, each with:
   - Title and affected file(s)
   - Dimension(s) it falls under
   - Description and recommended fix
   - Priority rating (High / Medium / Low) across all dimensions
3. Ranked priority table (highest-impact findings first)

---

## Phase 2: Address Findings (`architecture-review address [round]`)

Systematically work through findings in `claude-review-architecture.md`, one commit per finding.

**Round** (optional): limits work to a labelled subset of findings (e.g. `Round-2`). Defaults to all unresolved findings.

### Workflow per Finding

1. **Plan** — read the finding and identify the minimal correct fix
2. **Baseline complexity** — record cognitive complexity before touching the file:
   ```bash
   gocognit <file>
   ```
3. **Fix** — implement the change; if the correct fix lives in an upstream `github.com/dioad` repository, create a GitHub issue instead:
   ```bash
   gh issue create --repo github.com/dioad/<repo> --title "..." --body "..."
   ```
4. **Verify** — run the repository's pre-completion checks before committing (e.g. `make verify`)
5. **Post-fix complexity** — record cognitive complexity after the fix:
   ```bash
   gocognit <file>
   ```
6. **Commit** — one conventional commit per finding:
   ```
   fix: <short description of finding>
   ```
7. **Update document** — record outcome, commit SHA, and complexity delta in `claude-review-architecture.md`

### Constraints

- Keep complexity the same or lower after each fix — do not let it increase
- Do not batch multiple unrelated findings into one commit
- Do not skip the pre-completion checks step
