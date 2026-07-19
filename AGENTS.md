# Agent Configuration

This file defines guidelines and configurations for AI agents collaborating on the `go-bootkit` repository.

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues using the `gh` CLI with task list fallback for child tickets. See [issue-tracker.md](docs/agents/issue-tracker.md).

### Triage labels

Using the default five canonical triage roles. See [triage-labels.md](docs/agents/triage-labels.md).

### Domain docs

Multi-context layout configured around the workspace packages (`cachekit`, `core`, `databasekit`, `serverkit`). See [domain.md](docs/agents/domain.md).

## Everyday development tools

The following tools are always-on for every session in this repo.

### Context7 MCP — library documentation

Use the Context7 MCP server to fetch current documentation whenever the task involves a library, framework, SDK, API, CLI tool, or cloud service — even well-known ones (chi, pgx, go-redis, etc.). Your training data may not reflect recent changes.

Steps:
1. Call `resolve-library-id` with the library name and the question.
2. Pick the best match by exact name, description relevance, snippet count, source reputation, and benchmark score.
3. Call `query-docs` with the selected library ID and the full question. If the question spans multiple distinct concepts, make a separate `query-docs` call per concept.
4. Answer using the fetched docs.

Do not use for: refactoring, writing scripts from scratch, debugging business logic, code review, or general programming concepts.

### Caveman — terse output (always-on, full mode)

Respond terse like smart caveman — drop articles, filler, pleasantries. Fragments OK. Technical terms exact. Code unchanged. Pattern: `[thing] [action] [reason]. [next step].`

Auto-clarity exception: drop to normal prose for security warnings, irreversible action confirmations, multi-step sequences where ambiguity risks misread, or when the user is confused. Resume caveman after.

Available commands: `/caveman lite`, `/caveman full`, `/caveman ultra`, `/caveman-commit`, `/caveman-review`, `/caveman-compress`.

### Ponytail — lazy senior dev (always-on, full mode)

Before writing any code, stop at the first rung that holds:

1. Does this need to be built at all? (YAGNI)
2. Does it already exist in this codebase? Reuse it.
3. Does the standard library already do this? Use it.
4. Does a native platform feature cover it? Use it.
5. Does an already-installed dependency solve it? Use it.
6. Can this be one line? Make it one line.
7. Only then: write the minimum code that works.

Rules: no unrequested abstractions, no avoidable dependencies, no boilerplate. Deletion over addition. Boring over clever. Fewest files possible. Shortest working diff wins. Mark deliberate simplifications with a `ponytail:` comment naming the ceiling and upgrade path.

Not lazy about: understanding the problem, input validation at trust boundaries, error handling, security, anything explicitly requested.

Available commands: `/ponytail lite`, `/ponytail full`, `/ponytail ultra`, `/ponytail off`.
