# Serverkit

Inbound server configurations and network listeners for HTTP/gRPC services.

## Language

**HTTPServer**:
A component that manages the lifecycle of a TCP network listener and its associated HTTP server.
_Avoid_: WebServer, HTTPListener, APIHost

## Architecture

- `HTTPServer` embeds `core.Lifecycle` for robust start/stop handling, manages the graceful shutdown context for the underlying HTTP server, and delegates its health checks to `healthkit.StandardChecks`.
- `MountHealthRoutes(mux, aggregator)` registers standard health probe endpoints (`/health`, `/health/liveness`, `/health/readiness`, `/health/startup`) on any `chi.Router`. Consumers who build their own router call this directly instead of using `NewDefaultHandler`.
- `NewDefaultHandler` assembles a chi router with middleware and calls `MountHealthRoutes` internally for health endpoint wiring.
