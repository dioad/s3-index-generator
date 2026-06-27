---
description: 'Required pre-completion checks for eventbroker'
applyTo: "**"
---

# Pre-completion Checks

Before a task is marked as complete, the following must execute successfully:

1. `go build .` — must complete without compilation errors
2. `go fix ./...` — apply any fixable issues
3. `go fmt ./...` — all files must be formatted
4. `go vet ./...` — must report no issues
5. `staticcheck ./...` — must report no issues (install: `go install honnef.co/go/tools/cmd/staticcheck@latest`)
6. `go test -race ./...` — all tests must pass with no data races
7. `shellcheck -o all <script.sh>` — all shell scripts must pass shellcheck

# PR Review Workflow

## Finding the PR for the Current Branch

```bash
gh pr view --json number --jq .number
gh pr view
```

## Workflow for Addressing Review Comments

1. Fetch unresolved comments for the current branch's PR
2. Analyze each comment
3. Make code changes to address the issues
4. One commit per comment or related group of issues
5. Run pre-completion checks
6. Push when all checks pass

## Commit Message Format for Review Fixes

```
fix: address PR {PR_NUMBER} review comments on {topic}

This commit addresses {N} unresolved review comments:

1. {Comment title} (line {X} of {file}.go)
   - {Description of fix}

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```
