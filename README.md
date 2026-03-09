# Go Boot Kit

A production-ready Go application framework that integrates multiple components for building robust web services.

## Features

- **Modular Architecture**: Separate modules for core functionality, caching, database, and web server
- **Configuration Management**: Environment-based configuration with sensible defaults
- **Health Checks**: Built-in health check endpoints for monitoring application status
- **Middleware**: Pre-configured middleware for logging, CORS, panic recovery, and more
- **API Structure**: Well-organized API structure with handlers and routes
- **Docker Support**: Ready-to-use Dockerfile and docker-compose configuration

## Components

- **Core**: Base interfaces and application runner for managing component lifecycle
- **CacheKit**: Redis cache integration
- **DatabaseKit**: PostgreSQL database integration
- **ServerKit**: HTTP server with Chi router and middleware

## Getting Started

### Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose (for containerized deployment)
- PostgreSQL (for local development)
- Redis (for local development)

### Installation

1. Clone the repository:

```bash
git clone https://github.com/mazzama/go-bootkit.git
cd go-bootkit
```

2. Install dependencies:

```bash
cd app
go mod tidy
```

### Running Locally

1. Start the required services (Redis and PostgreSQL):

```bash
docker-compose up -d redis postgres
```

2. Run the application:

```bash
cd app
go run main.go
```

### Running with Docker

Build and run the entire stack using Docker Compose:

```bash
docker-compose up --build
```

## Configuration

The application is configured using environment variables. You can set these in the `.env` file for local development or in the Docker Compose file for containerized deployment.

Key configuration options:

- `SERVER_ADDR`: HTTP server address (default: `:8080`)
- `REDIS_ADDR`: Redis server address (default: `localhost:6379`)
- `DB_CONN_STR`: PostgreSQL connection string
- `ENVIRONMENT`: Application environment (`development`, `staging`, `production`)
- `LOG_LEVEL`: Logging level (`debug`, `info`, `warn`, `error`)

See the `.env` file for a complete list of configuration options.

## API Endpoints

- `GET /api/status`: Returns the API status
- `GET /api/items`: Returns a list of items
- `POST /api/items`: Creates a new item

## Development

### Project Structure

```
├── app/              # Main application
│   ├── api/          # API handlers
│   ├── config/       # Configuration
│   ├── middleware/   # Custom middleware
│   └── main.go       # Application entry point
├── cachekit/         # Redis cache integration
├── core/             # Core interfaces and runner
│   └── healthkit/    # Health check utilities
├── databasekit/      # PostgreSQL database integration
└── serverkit/        # HTTP server with Chi router
```

### Adding New Components

To add a new component:

1. Implement the `core.Component` interface
2. Add the component to the application runner in `main.go`

## License

This project is licensed under the MIT License – see the [LICENSE](./LICENSE) file for details.
