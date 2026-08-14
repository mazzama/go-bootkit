# Return chi.Router from NewDefaultRouter

## Status

Accepted — supersedes [ADR-0001](0001-decouple-router-from-server.md).

## Context

`NewDefaultHandler` returned `http.Handler` (ADR-0001), but every consumer immediately type-asserted it to `*chi.Mux` to mount application routes — a runtime panic waiting to happen. The stated rationale was "keep chi behind the seam," but `MountHealthRoutes(mux chi.Router, ...)` — added in the same commit — already exposed `chi.Router` in the public API. One adapter exists (chi); the hypothetical seam provided no real leverage.

## Decision

`NewDefaultRouter` returns `chi.Router`. `chi.Router` satisfies `http.Handler`, so callers can still pass it to `NewHTTPServer` or `otelhttp.NewHandler` without a cast. Route mounting is now direct:

```go
router := serverkit.NewDefaultRouter(healthAgg, logger)
router.Mount("/api", apiHandler)

server, _ := serverkit.NewHTTPServer("api", ":8080", router)
```

The seam discipline for serverkit is:

- `HTTPServer` accepts `http.Handler` — this is the real seam (any router framework works here).
- `NewDefaultRouter` returns `chi.Router` — honest about the concrete framework; one adapter = hypothetical seam, two = real.
- `MountHealthRoutes` accepts `chi.Router` — consistent with `NewDefaultRouter`.

## Consequences

- Callers no longer need a type assertion to mount routes — compile-time safety.
- `chi` is an explicit public dependency of serverkit (it already was via `MountHealthRoutes`).
- If a second router framework is ever needed, the seam is at `HTTPServer` (accepts `http.Handler`), not at `NewDefaultRouter`.
- The function was renamed from `NewDefaultHandler` to `NewDefaultRouter` so the name matches the return type.
