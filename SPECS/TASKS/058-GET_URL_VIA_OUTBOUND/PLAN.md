# PLAN 058 — GetURLViaOutbound

Класс §3.6 «мост» по образцу SPEC 015/037: шов в `.proto` под `// lx:`-маркером,
handler за `with_lx_command`, stub-двойник, обёртка в CommandClient. Донор
механики — `URLTestOutbound` (сосед по файлу): резолв тега в двух менеджерах,
отмена per-call ctx, Variant B.

## Файлы

| Файл | Изменение | Зона |
|------|-----------|------|
| `daemon/started_service.proto` | `rpc GetURLViaOutbound` + `message GetURLViaOutboundRequest/Response` + `HttpHeaderPair` внутри `lx:begin/end lx_command` | lx-шов в upstream-файле |
| `daemon/started_service.pb.go`, `daemon/started_service_grpc.pb.go` | регенерация (`make -f Makefile.lx lx-proto` + `gofumpt`) | машинный вывод |
| `daemon/started_service_command_lx.go` | handler `GetURLViaOutbound` рядом с `URLTestOutbound` | lx-only |
| `daemon/started_service_command_lx_stub.go` | stub `Unimplemented` | lx-only |
| `daemon/started_service_geturl_lx_test.go` | **новый** — юниты handler'а (httptest-сервер + фиктивный dialer) | lx-only |
| `experimental/libbox/command_client_command_lx.go` | `HTTPHeaders`, `GetURLResult`, `CommandClient.GetURLViaOutbound(...)` | lx-only |

## Решения

- **Отдельный RPC, не флаг в `URLTestOutbound`.** Донор пишет результат
  в `urlTestHistoryStorage` и меряет только время; фетч времени тела
  в историю писать не должен (испортит показания задержек в UI). Общий
  handler с `if wantBody` разошёлся бы по всем веткам — дешевле сосед.
- **`body` = `bytes`, не `string`.** proto3-`string` обязан быть валидным
  UTF-8; тело произвольного эндпоинта этого не гарантирует. На libbox-границе
  конверсия в `string` — осознанная потеря (§5 SPEC).
- **Клиент строится на вызов, не кешируется.** `DisableKeepAlives` +
  `CloseIdleConnections` в defer: держать сокет через узел после ответа
  диагностика не должна (на WG это ещё и удерживает туннель от засыпания —
  ср. SPEC 020).
- **certificate-store как у `libbox.NewHTTPClient`.** Системный пул x509
  в mobile-процессе пуст → HTTPS без него не поднимется на Android.
  Store создаётся на вызов и закрывается в defer: живёт миллисекунды,
  инстанс-кеш не оправдан для ручного действия пользователя.
  Ветка — `C.IsAndroid`, как в доноре `http.go`.
- **Редиректы следуются штатным `http.Client`** (лимит 10). Каждый хоп идёт
  через тот же `DialContext`, то есть через тот же узел — специальной
  политики не нужно. Возвращается статус конечной точки.
- **`remoteAddr` через `httptrace.GotConn`**, а не через резолвер: нужен
  адрес фактического соединения изнутри туннеля, включая случай, когда
  узел сам резолвил домен.
- **Кламп в ядре, не в клиенте.** `0 → 256 KiB`, потолок `1 MiB`: клиент
  не должен иметь возможности заказать неограниченное чтение в память
  gomobile-процесса.
- **`headers` — `repeated HttpHeaderPair`,** не `map<string,string>`:
  на проводе map даёт недетерминированный порядок, а на libbox-границе
  всё равно нужен билдер (мост не умеет коллекций кроме `[]byte`).
- **`Host` из пары заголовков перекладывается в `request.Host`** — в Go
  это отдельное поле, установка через `Header.Set` игнорируется.
- Возврат libbox — объект с геттерами (SPEC 038), не голая строка.

## Зона касания upstream

Единственный upstream-файл — `daemon/started_service.proto`, правка внутри
существующего `lx:begin/end lx_command`-блока. Go-код целиком в lx-only
файлах. Конфликтность при синке — нулевая сверх уже существующей.

## Дополнение — строгая REST-проба для MagicNet

Для контракта §7 используется новый helper `URLTestHTTP204` за
`with_lx_probe`. Он создаёт одноразовый GET-клиент с dialer выбранного
узла, штатной проверкой TLS, запретом redirect и ограничением заголовков.
Старый `URLTest` не меняется: у него иной HEAD-контракт.

REST handler сохраняет отмену входящего запроса и добавляет service
registry ядра для сертификатов и часов. Неуспешное измерение возвращается
как данные HTTP 200 с маркером поддержки; только неверные параметры дают
400. Парный stub возвращает управление прежнему handler. Дополнительная
зона касания upstream — пять строк ранней диспетчеризации в
`experimental/clashapi/proxies.go` под `lx:begin/end strict_http_probe`.
Новые lx-файлы и этот шов коммитятся отдельно.
