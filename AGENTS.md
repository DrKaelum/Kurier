# Kurier repository context

Kurier is an agent-ready API testing and debugging platform. Its core value is
creating immutable, sanitized API-execution evidence that developers and
coding agents can inspect.

Security-sensitive request information must never be logged, persisted,
committed, or exposed to MCP without deliberate redaction. Authorization
headers, cookies, credential headers, and user-designated secrets must be
treated as sensitive.

The proposal and architecture are allowed to change when evidence supports a
better design. Prefer completing a reliable presentation-ready vertical slice
over maximizing feature count. Avoid implementing speculative abstractions or
stretch features before the MVP requires them.

Every completed task must be formatted, built, linted, and tested using the
relevant available commands before being declared complete.

# Repository workflow

- After fully completing and verifying a task, if this working directory is in
  a Git repository with a GitHub remote, commit only the changes made for that
  task and push them to GitHub.
- Always make task commits on a branch named `codex`. If `codex` already
  exists, switch to it before committing; otherwise, create it and commit
  there.
- Use a lightweight Conventional Commits format for commit subjects:
  `type: concise description` or, when useful,
  `type(scope): concise description`. Prefer lowercase types such as `feat`,
  `fix`, `refactor`, `build`, `chore`, `docs`, `test`, `style`, `perf`, `ci`,
  and `revert`.
- Preserve unrelated user changes in the working tree; do not include them in
  the task's commit.
- Never force-push, rewrite published history, or delete branches unless
  explicitly requested.
- If committing or pushing is unsafe, blocked, or would require a consequential
  choice, explain the issue instead of silently skipping submission.
