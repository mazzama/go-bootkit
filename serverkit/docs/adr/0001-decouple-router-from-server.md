# Decouple Router from Server

We decided to decouple HTTP routing, middleware, and health aggregator registration from the `HTTPServer` component, accepting a generic `http.Handler` instead. This keeps the server focused entirely on network listener lifecycle management, ready-state signaling, and graceful shutdown, allowing callers to use any router framework if they choose.

`NewDefaultHandler` returns `http.Handler` — chi is the internal implementation, not the declared interface. Callers that need chi-specific features (route groups, middleware via `chi.Mux`) type-assert at wiring time:

```go
handler := serverkit.NewDefaultHandler(healthAgg, logger)
router := handler.(*chi.Mux)
router.Mount("/api", apiHandler)
```

This keeps the architectural seam consistent: the server accepts `http.Handler`, the handler factory returns `http.Handler`, and chi stays behind the seam.
