# SETUP — руководство по запуску

> Написано так, чтобы выполнялось **вами, вручную**. Каждый шаг печатает что-то, что можно проверить.
> Ожидаемое время: 20–30 минут, большая часть — ожидание на страницах создания аккаунтов.
>
> **Перепроверено на живой системе 2026-10-02 (сессия 4).** Названия учётных данных (credential),
> их типы и список узлов ниже прочитаны из `workflow/book-rag-system.json`; ширина эмбеддинга
> измерена на живом API Gemini. Если вы меняете узел, меняйте и этот файл — см. [PROGRESS.md](PROGRESS.md).

---

## Шаг 0 — Контрольный список предварительных требований

| Проверка | Команда | Ожидается |
|---|---|---|
| Docker Desktop **запущен** | `docker info --format '{{.ServerVersion}}'` | номер версии, а не ошибка |
| n8n установлен | `& "$env:APPDATA\npm\n8n.cmd" --version` | `2.40.7` |
| Порт 5433 свободен | `Test-NetConnection localhost -Port 5433` | `TcpTestSucceeded: False` до запуска |
| Порт 5432 не тронут | `Test-NetConnection localhost -Port 5432` | ваш существующий Postgres, без изменений |

Если `docker info` выдаёт ошибку, запустите Docker Desktop и дождитесь, пока значок кита перестанет анимироваться.

---

## Шаг 1 — Настройка окружения

```powershell
cd C:\Users\ThinkPro\Documents\deepseek-harness\n8n-rag-system
Copy-Item .env.example .env
notepad .env
```

Задайте настоящий `POSTGRES_PASSWORD`. У всего остального есть рабочие значения по умолчанию.

> `.env` хранит секреты и игнорируется git. Никогда не вставляйте его содержимое в документацию или чат.

---

## Шаг 2 — Запуск базы данных

```powershell
.\scripts\start-db.ps1
```

Команда подтягивает `pgvector/pgvector:pg17` (первый запуск занимает несколько минут), запускает контейнер
`n8n-book-rag-db` на порту **5433** и ждёт, пока healthcheck сообщит `healthy`.

Затем убедитесь, что схема действительно создана:

```powershell
.\scripts\verify-db.ps1
```

**Ожидается:** расширение `vector`, таблицы `books` и `book_chunks`, а также функции
`match_book_chunks`, `get_book`, `list_books`. Если таблиц нет, скрипты инициализации не
выполнились — см. Диагностика проблем.

---

## Шаг 3 — Получение бесплатного ключа API

**Один ключ покрывает всё, что нужно собранному workflow.** Перейдите на
[Google AI Studio](https://aistudio.google.com/apikey), войдите под аккаунтом Google, создайте ключ API.
**Без банковской карты.** Он обеспечивает работу эмбеддингов **и** чат-модели агента.

> Workflow в собранном виде запускает агента на `@n8n/n8n-nodes-langchain.lmChatGoogleGemini`
> (`models/gemini-3-flash-preview`), разделяя этот один ключ и, следовательно, эту одну квоту. И
> `models/gemini-3-flash-preview`, и `models/gemini-embedding-001` были подтверждены как доступные для этого ключа
> 2026-10-02.

**Следите за квотой.** Агент с вызовом инструментов тратит несколько запросов на один ответ, поэтому небольшой суточный лимит ощущается примерно как один разговор. Если агент начинает возвращать `429`, дождитесь сброса в полночь по тихоокеанскому времени или
переключите чат-модель (ниже).

**Необязательно — замена чат-модели агента на бесплатный тариф Groq.** Groq совместим с OpenAI и снова доступен
из этой сети (`openai/gpt-oss-120b` подтверждён как `active` 2026-10-02). Это **изменение
workflow, а не просто учётных данных (credential)**: замените узел `Chat Model (swap me)` на узел чат-модели Groq и
привяжите учётные данные (credential) типа **Groq** (n8n 2.40.7 поставляется с нативным — вам не нужен
обходной путь через совместимость с OpenAI). Не создавайте учётные данные (credential) Groq в остальных случаях; ничто в поставляемом
workflow на них не ссылается.

Полное обоснование и альтернативы (включая DeepSeek как первое платное обновление и локальную Ollama)
находятся в [DECISIONS.md](DECISIONS.md) ADR-001, ADR-005a и ADR-006.

---

## Шаг 4 — Импорт workflow

```powershell
.\scripts\import-workflow.ps1
```

Или вручную в интерфейсе n8n: **Workflows → ⋯ → Import from File →**
`workflow\book-rag-system.json`.

Импорт приносит структуру узлов и **только заполнители учётных данных (credential)** — n8n никогда не импортирует секреты.
Ожидайте, что каждый узел с учётными данными (credential) будет показывать красный предупреждающий треугольник до Шага 5.

---

## Шаг 5 — Создание учётных данных (credential) в n8n

Workflow привязывает учётные данные (credential) **по имени**, поэтому имена должны совпадать символ в символ. Эти три —
полный набор, прочитанный из `workflow/book-rag-system.json` 2026-10-02:

| Имя для использования | Тип | Поля |
|---|---|---|
| `Postgres (Book RAG)` | Postgres | Host `localhost`, Port `5433`, Database `bookrag`, User `bookrag`, Password = `POSTGRES_PASSWORD`, SSL `disable` |
| `Google Gemini (Book RAG)` | Google Gemini(PaLM) Api | ключ API из Шага 3 |
| `Book RAG Webhook Auth` | Header Auth | Name = `x-book-rag-key` (`WEBHOOK_HEADER_NAME`), Value = `WEBHOOK_HEADER_VALUE` из `.env` |

---

## Шаг 6 — Привязка учётных данных (credential) к узлам

Откройте workflow и привяжите, по имени — **12 привязок по 12 узлам**:

| Узел | Учётные данные (credential) |
|---|---|
| `Delete Previous Chunks` | Postgres (Book RAG) |
| `Postgres PGVector Store (Insert)` | Postgres (Book RAG) |
| `Upsert Book Metadata` | Postgres (Book RAG) |
| `Postgres Chat Memory` | Postgres (Book RAG) |
| `Search Book Library` | Postgres (Book RAG) |
| `Get Book Details` | Postgres (Book RAG) |
| `List Library` | Postgres (Book RAG) |
| `Remove Book` | Postgres (Book RAG) |
| `Embeddings Google Gemini` | Google Gemini (Book RAG) |
| `Chat Model (swap me)` | Google Gemini (Book RAG) |
| `Ingest Webhook` | Book RAG Webhook Auth |
| `Recommend Webhook` | Book RAG Webhook Auth |

**Один узел эмбеддингов обслуживает и загрузку, и поиск (по векторам)** (ADR-002). Не добавляйте второй.

Ширина эмбеддинга — **3072** — измерена на живом API Gemini 2026-10-02 — и она совпадает с
`halfvec(3072)` в `db/01-schema.sql`. Узел эмбеддингов Gemini в n8n не предоставляет управления размерностью, и никаких
изменений не требуется. `halfvec` обязателен, а не косметичен: pgvector не может построить HNSW-индекс по обычному `vector` выше
2000 измерений (ADR-005b).

---

## Шаг 7 — Индексация библиотеки

> **Используйте скрипт — массовый путь workflow в собранном виде сломан.** Нажатие *Execute workflow* на
> `Index Book Library` падает на `Read Book Catalogue`. Этот узел — узел `extractFromFile`: он
> преобразует **входящие двоичные данные** в JSON, а ничто выше него по потоку их не создаёт. Ни один узел в
> workflow не читает файл с диска, поэтому примечание на нём ("Reads samples/books.csv") описывает намерение,
> которое так и не было подключено.
>
> Исправление — узел **Read/Write Files from Disk** между `Index Book Library` и
> `Read Book Catalogue`, **плюс** `N8N_BLOCK_FILE_ACCESS_TO_N8N_FILES=false` в окружении n8n (n8n
> по умолчанию блокирует доступ к файлам вне `~/.n8n`) **плюс** перезапуск n8n. Эта переменная расширяет доступ к файлам
> для *каждого* workflow на инстансе, так что это осознанный компромисс, а не значение по умолчанию. Она
> **не** была применена.

Поддерживаемый путь — это webhook загрузки, который запускает **ту же** цепочку индексации
(`Prepare Book Record` → чанк → эмбеддинг → PGVector → upsert):

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\ingest-catalogue.ps1
# options: -Path .\samples\books.csv   -DelayMs 500   -StopOnError
```

Workflow должен быть **Active**, чтобы production-URL webhook отвечал.

Затем проверьте результат в базе данных:

```powershell
docker exec -it n8n-book-rag-db psql -U bookrag -d bookrag -c "select (select count(*) from books) as books, (select count(*) from book_chunks) as chunks;"
```

**Ожидается:** `books` = число строк в вашем CSV, `chunks` > 0. Повторный запуск загрузки **не должен**
увеличивать эти счётчики (идемпотентность — ADR-004): `book_id` является ключом, поэтому повторный запуск обновляет, а не
дублирует. `samples/books.csv` содержит 20 книг, поэтому ожидайте `books = 20`.

Если любая строка возвращает `500`, это почти всегда квота эмбеддингов Gemini — подождите и запустите снова; скрипт
безопасно повторять.

---

## Шаг 8 — Дымовой тест (smoke test) обеих точек входа

**Чат:**
Откройте workflow, нажмите кнопку **Chat** (внизу слева на холсте) и спросите:
> I loved Dune — recommend something similar.

Ожидайте 2–3 названия, которые **действительно есть в вашей библиотеке**, каждое с причиной в одну строку. Задайте уточняющий вопрос
("something shorter?"), чтобы убедиться, что память работает.

**Webhook (PowerShell):**

> **Workflow должен быть Active, чтобы этот URL отвечал.** Production-путь — `/webhook/...`. Если
> workflow неактивен, либо переключите его в Active, либо нажмите **Listen for test event** на узле webhook
> и используйте URL `/webhook-test/...`, который покажет n8n.

```powershell
$headers = @{ 'x-book-rag-key' = '<WEBHOOK_HEADER_VALUE from .env>' }
$body = @{ chatInput = 'Recommend a mystery novel with a strong female lead'; sessionId = 'test-1' } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri 'http://localhost:5678/webhook/book-rag/recommend' -Headers $headers -ContentType 'application/json' -Body $body
```

Ожидайте `{ "output": "…", "sessionId": "test-1" }`.

**Webhook загрузки:**

```powershell
$book = @{
  title = 'Test Book'; author = 'A. Author'; genre = 'Science Fiction'
  description = 'A test book used to verify the ingest webhook end to end.'
  content = 'This is the indexed content of the test book, long enough to be chunked.'
} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri 'http://localhost:5678/webhook/book-rag/ingest' -Headers $headers -ContentType 'application/json' -Body $book
```

---

## Диагностика проблем

| Симптом | Причина | Исправление |
|---|---|---|
| `start-db.ps1` сразу падает на `docker info` | Docker Desktop не запущен | Запустите Docker Desktop, дождитесь его стабилизации, повторите |
| Порт 5433 уже используется | Что-то другое заняло порт | Измените `POSTGRES_PORT` в `.env` **и** порт в учётных данных (credential) Postgres в n8n |
| `verify-db.ps1` сообщает об отсутствии таблиц | Скрипты инициализации выполнились до появления `01-schema.sql`, или том старше этого файла | Примените вручную: `docker exec -i n8n-book-rag-db psql -U bookrag -d bookrag < db/01-schema.sql` |
| `expected 3072 dimensions, not N` при вставке | Ширина модели эмбеддингов ≠ ширина схемы | Согласуйте их: измените `halfvec(3072)` в `db/01-schema.sql` на реальную ширину **и** переиндексируйте. Никогда не оставляйте их несогласованными. Ширина составляет **3072** по состоянию на 2026-10-02 |
| Агент отвечает книгами, которых нет в библиотеке | Модель игнорирует инструкцию о привязке к источникам, или поиск (по векторам) ничего не вернул | Сначала проверьте поиск (по векторам): запустите `search_book_library` вручную. Затем проверьте системный промпт на узле агента (он запрещает выдумывать названия) |
| Агент не возвращает ничего полезного для целого жанра | Нет проиндексированных подходящих чанков | Убедитесь, что строки `books` существуют для этого жанра; `list_library` должен их показать |
| Ошибки `429` во время загрузки | Достигнут лимит запросов бесплатного тарифа | Уменьшите размер пакета (обработки) `Loop Over Books` или запустите позже. Квота действует на проект и сбрасывается в полночь по тихоокеанскому времени |
| Gemini embeddings 404 | Модель выведена из эксплуатации/переименована | Проверьте текущее имя модели эмбеддингов в AI Studio и обновите узел |

---

## Команды на каждый день

```powershell
.\scripts\start-db.ps1        # bring the database up
.\scripts\verify-db.ps1       # confirm schema + row counts
.\scripts\stop-db.ps1         # stop the container (data kept)
& "$env:APPDATA\npm\n8n.cmd" start    # start n8n → http://localhost:5678

docker logs n8n-book-rag-db --tail 50            # database logs
docker exec -it n8n-book-rag-db psql -U bookrag -d bookrag   # open a SQL shell
```
