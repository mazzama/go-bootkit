# Decouple Router from Server

We decided to decouple HTTP routing, middleware, and health aggregator registration from the `HTTPServer` component, accepting a generic `http.Handler` instead. This keeps the server focused entirely on network listener lifecycle management, ready-state signaling, and graceful shutdown, allowing callers to use any router framework (like `chi` or the standard library) without framework lock-in inside `serverkit`.
