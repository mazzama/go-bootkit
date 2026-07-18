# Auto-Wire Health Checks

We decided to support automatic discovery and registration of service health checks inside the `ApplicationRunner` by introducing a `HealthCheckProvider` interface in `core`. Any component that implements `HealthChecks()` (such as `PostgresDB`, `RedisCache`, or `HTTPServer`) will have its readiness and liveness checks registered automatically at startup if a `healthkit.Aggregator` is provided to the runner. This significantly reduces setup boilerplate for developers starting a new service.
