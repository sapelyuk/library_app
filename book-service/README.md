# Book Service

Микросервис каталога книг и учёта физических экземпляров. Транспорт — gRPC
(`book.v1.BookService`), порт по умолчанию **:8081**.

## Архитектура

```
book-service/
├── cmd/server/            # main: конфигурация, запуск gRPC-сервера, graceful shutdown
├── proto/book/v1/         # контракт API (protobuf)
├── gen/go/                # сгенерированный код (protoc-gen-go, protoc-gen-go-grpc)
├── internal/
│   ├── domain/            # сущности Book/Copy, валидация ISBN, доменные ошибки
│   ├── service/           # бизнес-логика (use-case'ы)
│   ├── repository/        # порт хранилища + in-memory реализация
│   └── handler/           # gRPC-адаптер: маппинг прото <-> домен, ошибки -> codes.*
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
| `BOOK_SERVICE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `BOOK_SERVICE_LOG_FORMAT` | `json` | `json` или `text` |
| `BOOK_SERVICE_SHUTDOWN_TIMEOUT` | `15s` | таймаут graceful shutdown |

Останов по `Ctrl+C` — graceful shutdown: сначала снимается health-статус
`SERVING`, затем `GracefulStop` с таймаутом.

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

Контракт — `proto/book/v1/book.proto`. После его изменения:

```powershell
./scripts/gen_proto.ps1   # protoc + protoc-gen-go + protoc-gen-go-grpc
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
