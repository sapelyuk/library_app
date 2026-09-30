# Library app
This is backend microservices for library app. Services are written in Golang.
Work in progress...

## Architecture

| Service              | Responsibility                 | Port |
|----------------------|--------------------------------|------|
| Book Service         | Books catalog, ISBN, copies    | 8081 |
| User Service         | Members, librarians, auth      | 8082 |
| Loan Service         | Borrow/return, due dates       | 8083 |
| Notification Service | Email/SMS on due dates         | 8084 |
| API Gateway          | Single entry point, routing    | 8080 |

Book Service additionally serves a development HTTP surface on **8091**: REST endpoints
generated from the `google.api.http` annotations of its proto contract (grpc-gateway)
plus a Swagger UI at `http://localhost:8091/swagger/`. User Service mirrors this on
**8082** (gRPC) / **8092** (REST + Swagger).

## Communication patterns:
- Sync: gRPC between services (fast, typed)
- Async: NATS/Kafka for events (book.borrowed, loan.overdue)
- Discovery: Consul or Kubernetes DNS
- Storage: Postgres per service (database-per-service pattern)

## Status (as of 2026-09)

| Component                                | State                                                    |
|------------------------------------------|----------------------------------------------------------|
| Shared `pkg/` (logger, config, migrate)  | done                                                     |
| Book Service (proto, domain, service, repository, handler, server) | done, in-memory storage |
| Book Service REST + Swagger UI (grpc-gateway, `:8091`) | done |
| Book Service PostgreSQL repository       | not started; migration `001_init.sql` is ready           |
| User Service (proto, domain, security, service, handler, server) | done, **PostgreSQL** storage (argon2id + bearer tokens) |
| User Service REST + Swagger UI (grpc-gateway, `:8092`) | done |
| User Service migrations runner (`pkg/migrate`, embed FS) | done, applied at startup |
| Loan / Notification Service, API Gateway | not started                                              |
| Inter-service gRPC clients, events, discovery | not started (User Service exposes `AuthenticateToken` for the future gateway) |
| CI, containerization                     | not started                                              |

Run Book Service locally:

```bash
cd book-service
go run ./cmd/server     # gRPC on :8081, REST + Swagger on :8091
```

Run User Service locally (needs a PostgreSQL database; see `user-service/README.md`
for the one-time role/database provisioning and the `USER_SERVICE_SEED_LIBRARIAN_*`
bootstrap variables):

```bash
cd user-service
go run ./cmd/server     # gRPC on :8082, REST + Swagger on :8092
```

Build / check / test (inside each module directory, not from the repo root):

```bash
go build ./... && go vet ./... && go test ./...
```

Regenerate gRPC code after editing a `.proto` contract:

```powershell
./scripts/gen_proto.ps1
```

## Project Structure (actual)

library_app/
├── go.work              # workspace: ./book-service, ./pkg
├── README.md            # this file
├── KODA.md              # repo context for AI sessions
├── scripts/
│   └── gen_proto.ps1    # protoc + go/go-grpc/grpc-gateway/openapiv2 codegen
├── third_party/         # vendored .proto includes (google/api, openapiv2 options)
├── tools/
│   └── protoc/          # local protoc 36.2
├── pkg/                 # shared libs (logger, config)
├── book-service/        # implemented, see book-service/README.md
│   ├── cmd/server/
│   ├── proto/book/v1/
│   ├── gen/go/          # generated code, do not edit
│   ├── docs/            # generated swagger.json + go:embed wrapper
│   ├── internal/
│   │   ├── domain/
│   │   ├── repository/  # contracts + in-memory implementation
│   │   ├── service/
│   │   └── handler/
│   └── migrations/
├── api-gateway/         # not started
├── user-service/        # not started
├── loan-service/        # not started
└── notification-service/ # not started

Use Go workspaces (go.work) for local multi-module dev.
