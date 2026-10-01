# Smart Library

Полнофункциональная платформа управления библиотекой: **монорепозиторий** с
бэкендом на Go (gRPC-микросервисы), планируемыми React-приложением и
AI-рекомендациями книг на LLM/RAG.

- **Бэкенд:** Go, gRPC + protobuf, grpc-gateway (REST/Swagger), PostgreSQL, RabbitMQ.
- **Фронтенд (в плане):** React SPA, потребляющая REST-контракты сервисов.
- **AI (в плане):** рекомендации книг через LLM/RAG как часть архитектуры, а не
  надстройка — см. раздел «AI-first подход» и `docs/ai-first-principles.md`.

Проект в активной разработке: бэкенд-сервисы реализуются, фронтенд и AI-модуль —
следующие этапы.

## Архитектура

| Сервис               | Ответственность                    | Порт |
|----------------------|------------------------------------|------|
| API Gateway          | Единая точка входа, маршрутизация  | 8080 |
| Book Service         | Каталог книг, ISBN, экземпляры     | 8081 |
| User Service         | Читатели, библиотекари, доступ     | 8082 |
| Loan Service         | Выдача/возврат, сроки возврата     | 8083 |
| Notification Service | Email/SMS-уведомления о сроках     | 8084 |

Book Service дополнительно поднимает HTTP-слой для разработки на **8091**: REST-эндпоинты,
сгенерированные из аннотаций `google.api.http` его proto-контракта (grpc-gateway),
плюс Swagger UI по адресу `http://localhost:8091/swagger/`. User Service повторяет
это на **8082** (gRPC) / **8092** (REST + Swagger).

## Паттерны взаимодействия:
- Синхронно: gRPC между сервисами (быстро, типизированно)
- Асинхронно: **RabbitMQ** для событий (`book.borrowed`, `loan.overdue`) —
  выбор зафиксирован в `docs/adr/0001-message-broker.md`: topic exchange
  `library.events`, routing key = тип события, publisher confirms + ручной ack,
  отложенные доставки через TTL + dead-letter exchange
- Обнаружение сервисов: Consul или Kubernetes DNS
- Хранилище: отдельный PostgreSQL на каждый сервис (паттерн database-per-service)

## AI-first подход

В отличие от подхода «добавить AI в конец» (chatbot поверх готового продукта), в
Smart Library AI является **первоклассным компонентом архитектуры** с первого дня:

1. **Рекомендации — часть домена.** Каталог книг (Book Service) содержит
   метаданные, которые AI-сервис использует для формирования рекомендаций.
   Каждый сервис предоставляет gRPC-API — фронтенд и AI-модуль вызывают их
   независимо и параллельно.
2. **LLM/RAG через отдельный сервис.** AI-логика инкапсулирована в
   `ai-assistant/` (см. `docs/ai-first-principles.md`):
   - **RAG** для точности: семантический поиск по каталогу книг через векторную
     БД, результаты конкатенируются в контекст для LLM.
   - **Промпты версионируются** в `ai/prompts/` — каждый шаблон хранится как
     отдельный файл с номером версии.
   - **Оценка качества** в `ai/eval/` — тестовые сценарии, метрики
     релевантности, регрессия при обновлении модели.
3. **Фронтенд потребляет AI-контент как данные.** React-приложение получает
   рекомендации через REST-эндпоинт AI-сервиса и рендерит их в том же UI, что
   и обычные данные из каталога.
4. **Безопасность.** AI-модуль не имеет прямого доступа к БД сервисов — все
   данные поступают через gRPC-API с авторизацией. Промпты не содержат
   пользовательских данных без явного согласия.

Полные принципы описаны в `docs/ai-first-principles.md`.

## Состояние (на 2026-09)

| Компонент                                             | Состояние                                              |
|-------------------------------------------------------|--------------------------------------------------------|
| Общий `pkg/` (logger, config, migrate)                | готово                                                 |
| Book Service (proto, domain, service, repository, handler, server) | готово, хранилище in-memory         |
| Book Service REST + Swagger UI (grpc-gateway, `:8091`) | готово                                                |
| Book Service PostgreSQL repository                     | не начато; миграция `001_init.sql` готова              |
| User Service (proto, domain, security, service, handler, server) | готово, хранилище **PostgreSQL** (argon2id + bearer-токены) |
| User Service REST + Swagger UI (grpc-gateway, `:8092`) | готово                                                |
| Миграции User Service (`pkg/migrate`, embed FS)        | готовы, применяются при старте                         |
| Loan / Notification Service, API Gateway               | не начато                                              |
| Межсервисные gRPC-клиенты, события, discovery          | не начато (User Service отдаёт `AuthenticateToken` для будущего gateway) |
| CI, контейнеризация                                    | не начато                                              |
| Брокер сообщений: выбор и локальная инфраструктура    | готово: ADR-0001 (RabbitMQ), `docker-compose.yml`; реализация — #14 |

Локальный запуск Book Service:

```bash
cd book-service
go run ./cmd/server     # gRPC на :8081, REST + Swagger на :8091
```

Локальный запуск User Service (нужна база PostgreSQL; одноразовое создание роли и
базы и переменные сида `USER_SERVICE_SEED_LIBRARIAN_*` описаны в `user-service/README.md`):

```bash
cd user-service
go run ./cmd/server     # gRPC на :8082, REST + Swagger на :8092
```

Сборка / проверка / тесты (из директории модуля, не из корня репозитория):

```bash
go build ./... && go vet ./... && go test ./...
```

Перегенерация кода gRPC после правки `.proto`-контракта:

```powershell
./scripts/gen_proto.ps1
```

Локальная инфраструктура (брокер из ADR-0001):

```bash
docker compose up -d      # RabbitMQ: AMQP :5672, management UI http://localhost:15672
docker compose down -v    # остановить и удалить volume
```

Учётные данные брокера берутся из `RABBITMQ_USER`/`RABBITMQ_PASS` (по умолчанию `guest`).

## Структура проекта (фактическая)

```
library_app/
├── go.work              # воркспейс: ./book-service, ./user-service, ./pkg
├── README.md            # этот файл
├── KODA.md              # контекст репозитория для AI-сессий
├── docker-compose.yml   # локальная инфраструктура: RabbitMQ (:5672, UI :15672)
├── docs/
│   ├── adr/
│   │   └── 0001-message-broker.md  # решение по брокеру сообщений
│   └── ai-first-principles.md  # принципы AI-first подхода
├── scripts/
│   └── gen_proto.ps1    # кодогенерация protoc + go/go-grpc/grpc-gateway/openapiv2
├── third_party/         # vendored .proto includes (google/api, openapiv2 options)
├── tools/
│   └── protoc/          # локальный protoc 36.2
├── pkg/                 # общие библиотеки (config, logger, migrate)
├── book-service/        # реализован, см. book-service/README.md
│   ├── cmd/server/
│   ├── proto/book/v1/
│   ├── gen/go/          # генерация, не править руками
│   ├── docs/            # swagger.json (генерация) + обёртка go:embed
│   ├── internal/
│   │   ├── domain/
│   │   ├── repository/  # контракты + in-memory реализация
│   │   ├── service/
│   │   └── handler/
│   └── migrations/
├── user-service/        # реализован, см. user-service/README.md
│   ├── cmd/server/
│   ├── proto/user/v1/
│   ├── gen/go/          # генерация, не править руками
│   ├── docs/            # swagger.json (генерация) + обёртка go:embed
│   ├── internal/
│   │   ├── domain/      # User, Role, Status, Email, Session, RBAC
│   │   ├── security/    # argon2id, токены
│   │   ├── repository/  # контракты + postgres/
│   │   ├── service/
│   │   └── handler/     # gRPC, auth-интерцептор, REST + Swagger
│   └── migrations/
├── api-gateway/         # не начато
├── loan-service/        # не начато
└── notification-service/ # не начато
```

Для локальной разработки нескольких модулей одновременно используется Go-воркспейс
(`go.work`).
