# Book Service

Микросервис каталога книг и учёта физических экземпляров. Основной транспорт —
gRPC (`book.v1.BookService`), порт по умолчанию **:8081**. Дополнительно сервис
поднимает HTTP-слой (**:8091**): REST-ручки из аннотаций `google.api.http`
(grpc-gateway) и Swagger UI для ручного теста этих же ручек в браузере.

## Архитектура

```
book-service/
├── cmd/server/            # main: конфигурация, запуск gRPC + HTTP, graceful shutdown
├── proto/book/v1/         # контракт API (protobuf) + аннотации google.api.http
├── gen/go/                # сгенерированный код (protoc-gen-go, -go-grpc, -grpc-gateway)
├── docs/                  # book/v1/book.swagger.json (генерация) + embed спецификации
├── internal/
│   ├── domain/            # сущности Book/Copy, валидация ISBN, доменные ошибки
│   ├── service/           # бизнес-логика (use-case'ы)
│   ├── repository/        # порт хранилища + in-memory реализация
│   └── handler/           # grpc.go: прото <-> домен; http.go: gateway + Swagger UI
└── migrations/            # SQL-схема для PostgreSQL
```

Зависимости направлены строго внутрь: `handler -> service -> repository -> domain`.
Доменные ошибки (`domain.ErrNotFound`, `domain.ErrISBNAlreadyExists`, ...)
переводятся в коды gRPC в `internal/handler` и не текут наружу как `Internal`.

## Хранилище

Сейчас — **in-memory** (`internal/repository/memory`): данные живут только пока
работает процесс. Репозиторий спроектирован под переезд на PostgreSQL:

- уникальность ISBN и штрихкодов обеспечена на уровне хранилища;
- `AcquireAvailable` выполняет выдачу под write-lock — у PostgreSQL-реализации
  это будет `SELECT ... FOR UPDATE SKIP LOCKED`;
- реляционная схема — в `migrations/001_init.sql`.

## Запуск

```bash
cd book-service
go run ./cmd/server
```

Переменные окружения:

| Переменная | По умолчанию | Назначение |
| --- | --- | --- |
| `BOOK_SERVICE_GRPC_ADDR` | `:8081` | адрес gRPC-сервера |
| `BOOK_SERVICE_HTTP_ADDR` | `:8091` | адрес REST/Swagger-сервера |
| `BOOK_SERVICE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `BOOK_SERVICE_LOG_FORMAT` | `json` | `json` или `text` |
| `BOOK_SERVICE_SHUTDOWN_TIMEOUT` | `15s` | таймаут graceful shutdown |

Останов по `Ctrl+C` — graceful shutdown: сначала снимается health-статус
`SERVING`, закрывается HTTP-сервер, затем `GracefulStop` с таймаутом.

## REST API и Swagger

HTTP-слой построен на grpc-gateway: маршруты берутся из аннотаций
`google.api.http` в `proto/book/v1/book.proto`, поэтому REST и gRPC — это
один и тот же контракт и одна и та же бизнес-логика. Запросы приходят на
`:8091` и проксируются в gRPC-сервер этого же процесса (`:8081`).

Документация: **http://localhost:8091/swagger/** (корень `/` редиректит туда же),
спецификация — http://localhost:8091/swagger/swagger.json. Ассеты Swagger UI
подключаются с CDN, поэтому для открытия страницы нужен доступ в интернет;
сама спека отдаётся из бинарника (`//go:embed`).

| Метод | Путь | RPC |
| --- | --- | --- |
| `POST` | `/v1/books` | `CreateBook` |
| `GET` | `/v1/books` | `ListBooks` (`query`, `limit`, `offset`) |
| `GET` | `/v1/books/{id}` | `GetBook` |
| `PATCH` | `/v1/books/{id}` | `UpdateBook` |
| `DELETE` | `/v1/books/{id}` | `DeleteBook` |
| `POST` | `/v1/books/{book_id}/copies` | `AddBookCopy` |
| `GET` | `/v1/books/{book_id}/copies` | `ListBookCopies` |
| `POST` | `/v1/books/{book_id}/borrow` | `BorrowBookCopy` (тела нет, `book_id` в URL) |
| `POST` | `/v1/copies/{copy_id}/return` | `ReturnBookCopy` (тела нет, `copy_id` в URL) |

Доменные ошибки переводятся в HTTP-коды так же, как в gRPC-коды:
`400` (невалидные данные, `FailedPrecondition`), `404`, `409` (дубликат
ISBN/штрихкода). Тело ошибки — `{"code": ..., "message": "...", "details": []}`.

```bash
# создать книгу
curl -X POST http://localhost:8091/v1/books -H "Content-Type: application/json" \
  -d '{"isbn":"978-0-13-419044-0","title":"The Go Programming Language","author":"Alan A. A. Donovan","publisher":"Addison-Wesley","published_year":2015}'

# поиск
curl "http://localhost:8091/v1/books?query=go&limit=20"

# экземпляр, выдача и возврат (book_id/copy_id — из ответов выше)
curl -X POST http://localhost:8091/v1/books/<book_id>/copies \
  -H "Content-Type: application/json" -d '{"barcode":"BC-000001"}'
curl -X POST http://localhost:8091/v1/books/<book_id>/borrow
curl -X POST http://localhost:8091/v1/copies/<copy_id>/return
```

## Методы API

| Метод | Назначение |
| --- | --- |
| `CreateBook` | завести издание в каталог |
| `GetBook` | карточка книги + счётчики экземпляров |
| `ListBooks` | поиск (`query` по названию/автору/ISBN) с `limit`/`offset` |
| `UpdateBook` | частичное обновление (обновляются только переданные поля) |
| `DeleteBook` | удаление книги, если нет выданных экземпляров |
| `AddBookCopy` | зарегистрировать физический экземпляр |
| `ListBookCopies` | экземпляры книги |
| `BorrowBookCopy` | выдать первый доступный экземпляр (`AVAILABLE -> ON_LOAN`) |
| `ReturnBookCopy` | принять экземпляр обратно (`ON_LOAN -> AVAILABLE`) |

## Примеры (grpcurl)

На сервере включены reflection и standard health check:

```bash
# health
grpcurl -plaintext localhost:8081 grpc.health.v1.Health/Check

# создать книгу
grpcurl -plaintext -d '{
  "isbn": "978-0-13-419044-0",
  "title": "The Go Programming Language",
  "author": "Alan A. A. Donovan",
  "publisher": "Addison-Wesley",
  "published_year": 2015
}' localhost:8081 book.v1.BookService/CreateBook

# список с поиском
grpcurl -plaintext -d '{"query": "go", "limit": 20}' \
  localhost:8081 book.v1.BookService/ListBooks

# добавить экземпляр (book_id — из ответа CreateBook)
grpcurl -plaintext -d '{"book_id": "<uuid>", "barcode": "BC-000001"}' \
  localhost:8081 book.v1.BookService/AddBookCopy

# выдать и вернуть
grpcurl -plaintext -d '{"book_id": "<uuid>"}' localhost:8081 book.v1.BookService/BorrowBookCopy
grpcurl -plaintext -d '{"copy_id": "<uuid>"}' localhost:8081 book.v1.BookService/ReturnBookCopy
```

## Генерация кода

Контракт — `proto/book/v1/book.proto` (он же источник REST-маршрутов и Swagger).
После его изменения:

```powershell
./scripts/gen_proto.ps1
```

Скрипт запускает `protoc` из `tools/protoc` и четыре плагина:

| Плагин | Что генерирует |
| --- | --- |
| `protoc-gen-go` | `gen/go/book/v1/book.pb.go` |
| `protoc-gen-go-grpc` | `gen/go/book/v1/book_grpc.pb.go` |
| `protoc-gen-grpc-gateway` | `gen/go/book/v1/book.pb.gw.go` (REST-прокси) |
| `protoc-gen-openapiv2` | `docs/book/v1/book.swagger.json` |

Общие `.proto` (`google/api/annotations.proto`, `protoc-gen-openapiv2/options/*`)
лежат в `third_party/` на уровне репозитория. Установка плагинов:

```powershell
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.30.0
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.30.0
```

## Разработка

```bash
cd book-service
go build ./...
go vet ./...
go test ./...
```

## Ограничения текущей версии

- Хранилище in-memory: рестарт процесса очищает данные; PostgreSQL-реализация
  репозитория — в планах (миграция уже написана).
- Нет аутентификации: сервис рассчитан на внутренние вызовы через gateway.
- Событий (NATS/Kafka) пока нет — другие сервисы используют только gRPC.
