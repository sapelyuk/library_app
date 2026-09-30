# KODA.md — контекст проекта

## Обзор проекта

**library_app** — бэкенд для библиотечного приложения, реализованный как набор микросервисов на **Go (Golang)**. Реализовано: **Book Service полностью рабочий** (gRPC-API, REST и Swagger через grpc-gateway, домен, бизнес-логика, in-memory хранилище, тесты), **User Service рабочий** (gRPC-API, REST и Swagger, регистрация/вход, хеширование паролей, сессии, ролевая модель, PostgreSQL-хранилище, миграции; тестов пока нет), плюс общий модуль `pkg` (logger, config). Остальные сервисы существуют только как строки в архитектурной таблице.

- **Назначение:** каталог книг, учёт читателей и библиотекарей, выдача/возврат книг, уведомления о сроках возврата, единая точка входа для клиентов.
- **Язык и стек:** Go (локально установлен `go1.26.6 windows/amd64`), gRPC, protobuf, `log/slog`; NATS/Kafka, Consul/Kubernetes DNS и PostgreSQL — в планах.
- **Архитектурный стиль:** микросервисы с изолированным хранилищем на каждый сервис (паттерн *database-per-service*).
- **Организация кода:** монорепо с Go-воркспейсом (`go.work`) для локальной разработки нескольких модулей одновременно.

## Топология сервисов

| Сервис | Зона ответственности | Порт |
| --- | --- | --- |
| API Gateway | Единая точка входа, маршрутизация запросов | 8080 |
| Book Service | Каталог книг, ISBN, экземпляры | 8081 |
| User Service | Читатели, библиотекари, аутентификация | 8082 |
| Loan Service | Выдача/возврат, сроки возврата | 8083 |
| Notification Service | Email/SMS-уведомления о сроках | 8084 |

## Паттерны взаимодействия

- **Синхронная коммуникация:** gRPC между сервисами (быстро, типизированно).
- **Асинхронная коммуникация:** NATS или Kafka для доменных событий (`book.borrowed`, `loan.overdue`).
- **Обнаружение сервисов:** Consul либо Kubernetes DNS.
- **Хранилище:** отдельная база PostgreSQL на каждый сервис.

## Структура каталога (фактическое состояние)

```
library_app/
├── go.work                      # воркспейс: ./book-service, ./pkg, ./user-service
├── README.md                    # архитектурная спецификация проекта
├── KODA.md                      # этот файл
├── scripts/
│   └── gen_proto.ps1            # перегенерация кода (protoc + 4 плагина)
├── third_party/                 # внешние .proto: google/api, protoc-gen-openapiv2/options
├── tools/
│   └── protoc/                  # локальный protoc 36.2 (bin/protoc.exe)
├── pkg/                         # модуль library_app/pkg
│   ├── config/config.go         # String/Int/Duration/RequireString из env
│   ├── logger/logger.go         # slog: json|text, debug..error, MustNew
│   └── migrate/migrate.go       # SQL-миграции: таблица schema_migrations, checksum, Apply
├── book-service/                # модуль library_app/book-service (go.mod, go.sum)
│   ├── README.md                # документация сервиса: API, env, grpcurl/curl-примеры
│   ├── cmd/server/main.go       # конфиг, gRPC :8081 + HTTP :8091, health, reflection, shutdown
│   ├── proto/book/v1/book.proto # контракт BookService (9 RPC) + аннотации google.api.http
│   ├── gen/go/book/v1/          # book.pb.go, book_grpc.pb.go, book.pb.gw.go — генерация, не править
│   ├── docs/                    # book/v1/book.swagger.json (генерация) + docs.go (embed)
│   ├── internal/
│   │   ├── domain/              # Book, Copy, CopyStatus, ISBN, ошибки + тесты
│   │   ├── service/             # BookService (use-case'ы) + тесты
│   │   ├── repository/          # контракты + memory/ (in-memory Store) + тесты
│   │   └── handler/             # grpc.go: прото <-> домен; http.go: gateway + Swagger UI
│   └── migrations/001_init.sql  # схема PostgreSQL (ещё не применяется)
└── user-service/                # модуль library_app/user-service (go.mod, go.sum)
    ├── README.md                # документация сервиса: API, env, RBAC, provisioning БД
    ├── cmd/server/main.go       # конфиг, миграции, сид библиотекаря, purge, gRPC :8082 + HTTP :8092
    ├── proto/user/v1/user.proto # контракт UserService (12 RPC) + аннотации google.api.http
    ├── gen/go/user/v1/          # user.pb.go, user_grpc.pb.go, user.pb.gw.go — генерация, не править
    ├── docs/                    # user/v1/user.swagger.json (генерация) + docs.go (embed)
    ├── internal/
    │   ├── domain/              # User, Role, UserStatus, Email, PasswordPolicy, Session, Principal (RBAC)
    │   ├── security/            # argon2id (PHC), токены (base64url + SHA-256)
    │   ├── service/             # use-case'ы: регистрация, вход, RBAC, смена пароля
    │   ├── repository/          # контракты + postgres/ (database/sql, lib/pq)
    │   └── handler/             # gRPC-адаптер, auth-интерцептор, gateway + Swagger UI
    └── migrations/              # 001_init.sql (users, sessions) + migrations.go (embed FS)
```

`loan-service`, `notification-service` и `api-gateway` на диске **отсутствуют** — это следующая работа.

## Статус реализации

| Компонент | Состояние |
| --- | --- |
| `pkg/logger`, `pkg/config`, `pkg/migrate` | готово, используется обоими сервисами |
| Book Service: контракт, домен, сервис, handler, запуск | готов, собирается и работает |
| REST-слой и Swagger Book Service | готов: grpc-gateway на `:8091`, Swagger UI на `/swagger/` |
| Хранилище Book Service | только in-memory (`internal/repository/memory`), данные живут до рестарта |
| PostgreSQL-реализация репозитория Book Service | нет; миграция `001_init.sql` написана заранее |
| User Service: контракт, домен, security, сервис, handler, запуск | готов, собирается и работает |
| REST-слой и Swagger User Service | готов: grpc-gateway на `:8092`, Swagger UI на `/swagger/` |
| Аутентификация и RBAC | готово: argon2id-пароли, bearer-токены (в БД только SHA-256-хеш), сессии с TTL, роли READER/LIBRARIAN, интерцептор + проверки в домене |
| Хранилище User Service | PostgreSQL (`database/sql` + `lib/pq`), миграции применяются при старте |
| Loan / Notification Service, API Gateway | нет |
| gRPC-клиенты между сервисами, события (NATS/Kafka), discovery | нет (`AuthenticateToken` User Service — подготовленная точка входа для gateway) |
| CI, контейнеризация | нет |

## Ключевые файлы

- `README.md` — архитектурная спецификация: таблица сервисов с портами, паттерны коммуникации, целевая структура. Источник истины по архитектурным решениям.
- `book-service/README.md` — документация сервиса: методы API, env-переменные, примеры `grpcurl`, команда генерации протобуфа.
- `book-service/proto/book/v1/book.proto` — контракт `book.v1.BookService`: `CreateBook`, `GetBook`, `ListBooks`, `UpdateBook`, `DeleteBook`, `AddBookCopy`, `ListBookCopies`, `BorrowBookCopy`, `ReturnBookCopy`. Каждый RPC аннотирован `google.api.http` — это источник и REST-маршрутов, и Swagger-спецификации. После правки — перегенерировать код.
- `book-service/internal/handler/http.go` — HTTP-слой: grpc-gateway монтируется на `/v1/`, Swagger UI на `/swagger/`, спецификация на `/swagger/swagger.json`. Вызывает gRPC-сервер этого же процесса через локальный клиент.
- `book-service/docs/docs.go` — `//go:embed` сгенерированной `book/v1/book.swagger.json`; сам JSON правится только перегенерацией.
- `book-service/internal/repository/repository.go` — контракты `BookRepository` и `CopyRepository`. Важно: `AcquireAvailable` — атомарная выдача первого доступного экземпляра; в описании прямо указано, что для PostgreSQL это `SELECT ... FOR UPDATE SKIP LOCKED`.
- `book-service/internal/domain/errors.go` — доменные ошибки; handler переводит их в коды gRPC, наружу `Internal` не течёт.
- `scripts/gen_proto.ps1` — ждёт `protoc` в `tools/protoc/bin` и `$GOPATH/bin` в `PATH`; подключает `-I third_party` и плагины `protoc-gen-grpc-gateway`, `protoc-gen-openapiv2`.
- `third_party/` — `.proto` зависимости (`google/api/annotations.proto`, `google/api/http.proto`, `protoc-gen-openapiv2/options/*`): только include-путь, код из них не генерируется.
- `user-service/README.md` — документация сервиса: методы API, env-переменные, RBAC, provisioning локальной БД.
- `user-service/proto/user/v1/user.proto` — контракт `user.v1.UserService`: `Register`, `Login`, `Logout`, `GetCurrentUser`, `AuthenticateToken` (внутренний), `CreateUser`, `GetUser`, `ListUsers`, `UpdateUser`, `ChangePassword`, `DeactivateUser`, `RestoreUser`.
- `user-service/internal/domain/principal.go` — RBAC: `RequireLibrarian`, `AccountAccess`, `UpdateAccess`, `PasswordChange`, `StatusChange` (включая запрет самоблокировки библиотекаря).
- `user-service/internal/security/` — `password.go` (argon2id в PHC-формате, `DummyPasswordHash` для выравнивания времени входа) и `token.go` (случайный токен, в БД — SHA-256-хеш).
- `user-service/internal/handler/interceptor.go` — извлечение Bearer-токена из metadata, публичные методы (`Register`, `Login`), кладо principal в контекст.
- `user-service/migrations/` — `001_init.sql` в goose-формате + `migrations.go` с `//go:embed`; применяется `pkg/migrate` при старте.

## Сборка и запуск

Воркспейс уже инициализирован (`go.work`: `./book-service`, `./pkg`, `./user-service`). Команды выполняются из директории соответствующего модуля (из корня `go build ./...` не работает — корень не является модулем):

| Задача | Команда | Где выполнять |
| --- | --- | --- |
| Сборка | `go build ./...` | `book-service/`, `pkg/`, `user-service/` |
| Статический анализ | `go vet ./...` | `book-service/`, `pkg/`, `user-service/` |
| Форматирование | `gofmt -l .` (список), `gofmt -w .` (править) | `book-service/`, `pkg/`, `user-service/` |
| Тесты | `go test ./...` | `book-service/`, `pkg/`, `user-service/` |
| Запуск Book Service | `go run ./cmd/server` (gRPC на `:8081`, REST + Swagger на `:8091`) | `book-service/` |
| Запуск User Service | `go run ./cmd/server` (gRPC на `:8082`, REST + Swagger на `:8092`), нужна PostgreSQL | `user-service/` |
| Генерация gRPC-кода | `./scripts/gen_proto.ps1` | корень (PowerShell) |

Текущее состояние проверок (последний запуск): build/vet — чисто во всех трёх модулях; тесты есть только в `book-service` (23 теста + 27 подтестов), в `user-service` тестов пока нет.

Нюанс с `gofmt -l`: в рабочем дереве файлы `book-service/*` и `pkg/config`, `pkg/logger` идут с **CRLF** (включён `core.autocrlf=true`), поэтому `gofmt -l` помечает их все, хотя содержимое отформатировано корректно (`gofmt -d` показывает различие только в концах строк). Файлы, созданные с LF (`user-service/*`, `pkg/migrate`), помечены не быть. Гонять `gofmt -w .` ради этого не нужно — это перепишет конца строк во всех файлах модуля; форматировать стоит точечно, в файлах где реально менялся код.

Окружение: `go1.26.6 windows/amd64`; `protoc 36.2` лежит локально в `tools/protoc/bin/protoc.exe` (в системном PATH его нет); плагины `protoc-gen-go`, `protoc-gen-go-grpc`, `protoc-gen-grpc-gateway`, `protoc-gen-openapiv2` установлены в `C:\Users\yaros\go\bin`. GOPROXY доступен, Docker CLI есть, но демон обычно не запущен.

## Правила разработки

**Структура сервиса (проверяемое правило).** Каждый сервис — самостоятельный Go-модуль с фиксированным слоем:

- `cmd/server/main.go` — только сборка зависимостей и запуск; бизнес-логика здесь запрещена.
- `internal/domain/` — сущности и доменные ошибки, без внешних зависимостей.
- `internal/repository/` — доступ к PostgreSQL, единственное место с SQL/ORM.
- `internal/service/` — бизнес-правила, оркестрация репозиториев и клиентов других сервисов.
- `internal/handler/` — транспортный слой (HTTP и/или gRPC), маппинг между внешними контрактами и доменом.
- `proto/` — `.proto`-контракты сервиса.
- `migrations/` — SQL-миграции базы этого сервиса.

**Границы модулей.** Общая логика (logger, config) выносится в `pkg/`. Чужой `internal/` импортировать нельзя — межсервисные вызовы идут только через gRPC-контракты из `proto/` или через события.

**Изоляция данных.** Каждый сервис владеет своей PostgreSQL-схемой. Прямые запросы к чужой базе запрещены; согласованность между сервисами достигается событиями (`book.borrowed`, `loan.overdue`).

**Порты.** Зафиксированы таблицей сервисов и не должны меняться без обновления документации: 8080 gateway, 8081 book, 8082 user, 8083 loan, 8084 notification. Исключение — вспомогательный HTTP у Book Service (`8091` = gRPC-порт + 10): REST и Swagger для разработки и ручной проверки.

**Стиль кода.** Стандартные инструменты экосистемы Go: `gofmt`/`goimports` для форматирования, `go vet` для анализа, стандартная структура имён и ошибок Go. Имена пакетов — строчные, без подчёркиваний; имена директорий — kebab-case (`book-service`). **Комментарии в коде — только на английском** (в `.go`, `.sql`, `.proto`, `.ps1`); документация (`README.md`, `KODA.md`, `*/README.md`) — на русском.

**Тестирование.** Используется стандартный `testing` (без testify). Unit-тесты живут рядом с кодом (`*_test.go`, пакет `*_test`). Тесты сервиса и репозитория гоняются поверх in-memory `Store`, HTTP-слой проверяется через `httptest` (`internal/handler/http_test.go`). TODO: тесты репозитория против PostgreSQL (тестовые контейнеры), интеграционный тест gRPC-handler'а через `bufconn`, моки для gRPC-клиентов.

**Конфигурация.** Переменные окружения, читаются через `pkg/config` (значения по умолчанию задаются в `cmd/server/main.go`). Префикс Book Service — `BOOK_SERVICE_*` (см. таблицу в `book-service/README.md`).

## Открытые вопросы для уточнения

- Выбрать брокер событий: NATS или Kafka.
- Выбрать механизм discovery: Consul или Kubernetes DNS.
- Выбрать драйвер/ORM для PostgreSQL и инструмент миграций.
- Определить, какие эндпоинты gateway'я публичные (REST) и какие внутренние (gRPC).
- Настроить CI и контейнеризацию.
- Покрыть тестами `user-service` (домен, security, сервис на фейковых репозиториях, HTTP-слой через `httptest`, интеграционные проверки репозитория против локального PostgreSQL).
