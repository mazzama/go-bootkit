# Serverkit

Inbound server configurations and network listeners for HTTP/gRPC services.

## Language

**HTTPServer**:
A component that manages the lifecycle of a TCP network listener and its associated HTTP server.
_Avoid_: WebServer, HTTPListener, APIHost

## Architecture

- `HTTPServer` embeds `core.Lifecycle` for robust start/stop handling, manages the graceful shutdown context for the underlying HTTP server, and delegates its health checks to `healthkit.StandardChecks`.
