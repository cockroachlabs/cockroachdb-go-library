# Running the Demo

## Prerequisites
- Docker and Docker Compose

## Quick Start
```sh
docker compose up --build
```

## Services
- Demo App: http://localhost:8080
- CockroachDB SQL: localhost:26257
- CockroachDB Admin UI: http://localhost:8081

## Available Endpoints
- GET  /health              - Health check
- GET  /metrics             - Metrics snapshot
- POST /tx/insert           - Insert account (body: {"name":"Alice","balance":100})
- POST /tx/transfer         - Transfer funds (body: {"from":"id1","to":"id2","amount":50})
- GET  /follower-read       - Follower read demo
- GET  /accounts            - List accounts
- POST /webhook/cdc         - CDC webhook endpoint
- GET  /circuit-breaker/status  - Circuit breaker state
- POST /circuit-breaker/test   - Test circuit breaker

## Cleanup
```sh
docker compose down -v
```
