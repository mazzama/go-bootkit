# Decouple Router from Server

> **Status: Superseded by [ADR-0002](0002-return-chi-router.md).**

We decided to decouple HTTP routing, middleware, and health aggregator registration from the `HTTPServer` component, accepting a generic `http.Handler` instead. This keeps the server focused entirely on network listener lifecycle management, ready-state signaling, and graceful shutdown, allowing callers to use any router framework if they choose.

`NewDefaultHandler` returned `http.Handler` — chi was the internal implementation, not the declared interface. Callers that needed chi-specific features (route groups, middleware via `chi.Mux`) type-asserted at wiring time:

```go
handler := serverkit.NewDefaultHandler(healthAgg, logger)
router := handler.(*chi.Mux)
router.Mount("/api", apiHandler)
```

This kept the architectural seam consistent: the server accepts `http.Handler`, the handler factory returns `http.Handler`, and chi stays behind the seam. However, `MountHealthRoutes` (added in the same commit) already exposed `chi.Router` in the public API, breaking the premise. See ADR-0002 for the resolution.
