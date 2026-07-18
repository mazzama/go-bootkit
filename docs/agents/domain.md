# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT-MAP.md`** at the repo root — it points at one `CONTEXT.md` per module context. Read each one relevant to the topic you are working on.
- **`docs/adr/`** — read ADRs that touch the area you're about to work in.
- **`<module>/docs/adr/`** (e.g. `core/docs/adr/`, `serverkit/docs/adr/`, etc.) — check for module-scoped architectural decisions.

If any of these files don't exist, **proceed silently**. Don't flag their absence; don't suggest creating them upfront. The `/domain-modeling` skill (reached via `/grill-with-docs` and `/improve-codebase-architecture`) creates them lazily when terms or decisions actually get resolved.

## File structure

This is a multi-context repo structured around Go modules:

```
/
├── CONTEXT-MAP.md                     ← points to all module contexts
├── docs/agents/                       ← agent configuration
├── docs/adr/                          ← repository-wide ADRs
├── cachekit/
│   ├── CONTEXT.md
│   └── docs/adr/                      ← cachekit decisions
├── configkit/
│   ├── CONTEXT.md
│   └── docs/adr/                      ← configkit decisions
├── core/
│   ├── CONTEXT.md
│   └── docs/adr/                      ← core decisions
├── databasekit/
│   ├── CONTEXT.md
│   └── docs/adr/                      ← databasekit decisions
└── serverkit/
    ├── CONTEXT.md
    └── docs/adr/                      ← serverkit decisions
```

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in the context's `CONTEXT.md`. Don't drift to synonyms the glossary explicitly avoids.

If the concept you need isn't in the glossary yet, that's a signal — either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0007 (event-sourced orders) — but worth reopening because…_
