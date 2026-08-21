# TypeConverter

HTTP-сервис и веб-интерфейс для согласованного преобразования **XML ⇄ JSON** и схем **XSD ⇄ JSON template**. Документ хранится как одна модель и отдаётся в обоих форматах.

## Возможности

- Каталог: XML и JSON одной и той же модели `catalog` / `books`
- Схемы: разбор XSD в JSON template и обратно в XSD
- Очередь задач с воркерами (`queued` → `running` → `done` / `failed`)
- Результаты в SQLite (`typeconverter.db`): история задач, последний документ и загруженная схема
- Веб-UI на http://localhost:8080/

UI не конвертирует формат сам в себя: XML уходит в JSON, JSON — в XML, XSD — в JSON template и наоборот.

## Запуск

Нужен Go 1.25+.

```bash
go run ./cmd/typeconverter
```

Сервер слушает `:8080`. Конфиг по умолчанию — `config.json` в рабочей директории.

```bash
go run ./cmd/typeconverter -config config.json
```

Либо переменная окружения `CONFIG`. Если файл не задан явно и его нет на диске, берутся значения по умолчанию.

Сборка (Windows):

```bash
go build -o TypeConverter.exe ./cmd/typeconverter
```

Запускайте exe из папки с `config.json`. Рядом появится `typeconverter.db`.

## Конфигурация

`config.json`:

```json
{
  "http": {
    "addr": ":8080",
    "max_body_bytes": 10485760
  },
  "queue": {
    "workers": 0,
    "queue_size": 256,
    "max_jobs": 10000
  },
  "store": {
    "path": "typeconverter.db"
  }
}
```

`workers: 0` означает «число CPU, но не меньше 2». Путь SQLite относительно рабочей директории процесса. `:memory:` держит базу только в RAM.

Срок хранения в SQLite не ограничен по времени. История задач удаляется кнопкой «Очистить» (`DELETE /jobs`) или при превышении `max_jobs` (самые старые завершённые). Последний документ и схема от очистки истории не зависят.

Переменные окружения перекрывают файл: `ADDR`, `WORKERS`, `QUEUE_SIZE`, `MAX_JOBS`, `MAX_BODY_BYTES`, `STORE`.

## Веб-интерфейс

Откройте `/`. Два окна:

| Окно | Вход | Выход |
| --- | --- | --- |
| XML ⇄ JSON | XML или JSON каталога | противоположный формат |
| XSD / шаблон | XSD или JSON template | противоположный формат |

Переключение XML / JSON / XSD / JSON template подставляет пример, но на сервер ничего не уходит, пока не нажата кнопка **В JSON** / **В XML** или **В JSON template** / **В XSD**. Крупные тела в DOM показываются превью, полное содержимое уходит на сервер и в скачивание. Копирование и скачивание включаются после успешного результата; скачанный файл форматируется. Кнопка **Скачать** отдаёт файл браузеру (обычно в «Загрузки») и не сохраняет копию в проекте.

## HTTP API

### Документы

`POST /xml` и `POST /json` ставят задачу в очередь и отвечают **202** с билетом и заголовком `Location: /jobs/{id}`.

```bash
curl -s -X POST http://localhost:8080/xml?catalog=1 \
  -H "Content-Type: application/xml" \
  --data-binary @xmlconv/testdata/catalog.xml
```

```bash
curl -s http://localhost:8080/jobs/{id}
curl -s http://localhost:8080/jobs/{id}/json
curl -s http://localhost:8080/jobs/{id}/xml
```

`?catalog=1` всегда разбирает тело как каталог, даже если на сервере загружена схема.

Без `catalog=1` после `POST /xsd` документы разбираются по загруженной схеме (xsi:type, choice, наследование).

Последний успешный документ (живёт в SQLite и переживает перезапуск):

- `GET /xml`
- `GET /json`

Очередь (история тоже в SQLite):

- `GET /jobs` — в каждом элементе есть `kind`: `document` или `schema`
- `GET /jobs/{id}`
- `DELETE /jobs` — убрать все завершённые задачи
- `DELETE /jobs?kind=document` / `DELETE /jobs?kind=schema` — только документы или схемы

### Схемы

`POST /xsd` и `POST /jsontemplate` тоже ставят задачу в очередь и отвечают **202**. Результат: `GET /jobs/{id}/json` (JSON template) и `GET /jobs/{id}/xml` (XSD).

```bash
curl -s -X POST http://localhost:8080/xsd \
  -H "Content-Type: application/xml" \
  --data-binary @schema/testdata/messages.xsd
```

После успешной задачи схема загружается на сервер и тоже пишется в SQLite (переживает перезапуск):

- `GET /jsontemplate`
- `GET /xsd`
- `GET /schema` — `{ "status": "loaded", "root": "..." }`
- `DELETE /schema`

`POST /jsontemplate` принимает JSON template и кодирует схему обратно в XSD.

## Пакеты

| Пакет | Назначение |
| --- | --- |
| `cmd/typeconverter` | точка входа |
| `api` | HTTP, очередь, встроенный UI |
| `queue` | воркеры и статусы задач |
| `model` | DTO каталога |
| `xmlconv` / `jsonconv` | разбор и кодирование произвольного XML/JSON |
| `schema` | XSD, JSON template, инстансы |
| `converter` | приведение скаляров |
| `store` | SQLite: задачи и закодированные XML/JSON |
| `config` | JSON-конфиг и env |

## Тесты

```bash
go test ./...
```
