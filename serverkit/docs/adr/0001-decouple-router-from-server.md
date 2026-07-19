# Decouple Router from Server

We decided to decouple HTTP routing, middleware, and health aggregator registration from the `HTTPServer` component, accepting a generic `http.Handler` instead. This keeps the server focused entirely on network listener lifecycle management, ready-state signaling, and graceful shutdown, allowing callers to use any router framework if they choose.

However, as an opinionated framework, we designate `go-chi/chi` as our standard routing solution. To reflect this, `serverkit` provides a default `chi`-based router implementation (e.g., `NewDefaultHandler`) pre-configured with our standard middleware stack. This balances a clean, decoupled server architecture with the convenience of an opinionated default out of the box.
