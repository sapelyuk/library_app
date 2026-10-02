# AI Service

Сервис рекомендаций книг через LLM/RAG. Архитектурное решение —
ADR-0002 (`docs/adr/0002-ai-recommendation-architecture.md`).

## Статус

| Часть | Состояние |
| --- | --- |
| Архитектура (ADR-0002) | готово |
| RAG-прототип (n8n + pgvector + Gemini) | **работает end-to-end**: 20 книг проиндексировано, chat UI и webhook прошли smoke-тест |
| Артефакты прототипа RAG (`rag/`) | перенесены (задача #22) |
| Go-модуль: `ai.v1.AiService` (proto, gRPC/REST, n8n-клиент) | не начато (задача #23) |
| Синхронизация каталога через события (`book.*`) | не начато (задача #14) |

## Плановый контракт

`ai.v1.AiService` (proto + grpc-gateway, порты 8085 gRPC / 8095 REST):

- `Recommend(query, user_context) → [Book]` — рекомендация по запросу.
- `IngestBook(book_id) → success` — принудительная переиндексация книги.

Внутренняя реализация — HTTP-клиент к n8n webhook'ам (`internal/adapters/n8n/`).
Каталог синхронизируется через RabbitMQ: Book Service публикует
`book.created/updated/deleted`, сервис потребляет и вызывает ingest/delete.
n8n можно заменить на нативный Go-цикл RAG без изменения контракта.

## Каталог `rag/`

**Работающий** прототип agentic RAG-системы (n8n workflow + pgvector + Gemini):

- `rag/README.md` — назначение, состав, быстрый старт, переменные окружения.
- `rag/docs/` — документация прототипа: `ARCHITECTURE.md`, `SETUP.md`, `USAGE.md`.
- `rag/db/01-schema.sql` — схема pgvector (расширение, таблицы, функции поиска).
- `rag/workflow/` — экспорт n8n workflow.
- `rag/scripts/` — скрипты развёртывания и проверки (start/verify/import/ingest).

Ключевые параметры RAG-конвейера: эмбеддинги Gemini `models/gemini-embedding-001`
(3072 dims), вектор `halfvec(3072)`, HNSW-индекс, функция `match_book_chunks`;
агент использует 4 инструмента (векторный поиск, точный lookup, просмотр каталога,
удаление). Точки входа — chat UI и `POST /webhook/book-rag/recommend`.

Векторное хранилище поднято в корневом `docker-compose.yml` (контейнер
`ai-rag-db`, порт `:5433`, init-скрипты монтируются из `rag/db/`):

```bash
docker compose up -d ai-rag-db   # только pgvector
```

Схема применима и к отдельному стеку `rag/docker-compose.yml` (для работы
с прототипом без монорепо-инфраструктуры).
