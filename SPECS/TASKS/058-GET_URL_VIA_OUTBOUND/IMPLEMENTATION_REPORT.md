# IMPLEMENTATION REPORT 058 — GetURLViaOutbound

**Статус:** C (complete) — код, тесты и сборки зелёные; отгружается в
`v1.14.0-lx.25-rc.1`. Открыт один пункт приёмки: полевая проверка с устройства
после попадания в AAR.

## Что сделано

| Слой | Файл | Содержание |
|------|------|------------|
| Провод | `daemon/started_service.proto` | `rpc GetURLViaOutbound` + `GetURLViaOutboundRequest`/`Response` + `HttpHeaderPair` внутри `lx:begin/end lx_command` |
| Провод | `daemon/started_service*.pb.go` | Регенерация пиненым тулчейном |
| Ядро | `daemon/started_service_command_lx.go` | Handler + константы клампа |
| Ядро | `daemon/started_service_command_lx_stub.go` | `Unimplemented` без тега |
| Клиент | `experimental/libbox/command_client_command_lx.go` | `HTTPHeaders`, `GetURLResult`, `GetURLViaOutbound` |
| Тесты | `daemon/started_service_geturl_lx_test.go` | 8 юнитов контракта |
| Тесты | `daemon/started_service_geturl_lx_stub_test.go` | Stub-эквивалентность |

Контракт реализован ровно как в SPEC: unary, Variant B, не-2xx = результат,
кламп 256 KiB / потолок 1 MiB, `Truncated`, `RemoteAddr`, `ElapsedMs`,
резолв тега в обоих менеджерах, отмена per-call ctx.

## Что выяснилось по ходу

**Встраивание `adapter.Outbound` + `*probeDialer` в тестовый двойник даёт
неоднозначный `DialContext`.** Оба интерфейса несут метод на одной глубине
встраивания, и Go отказывается выбирать. Дайлер в двойниках держится полем,
методы написаны явно — на продовый код не влияет, но следующий, кто будет
писать фейк узла, наступит туда же.

**`go run ./cmd/internal/protogen` без последующего `gofumpt` даёт ложный шум.**
Генератор перестраивает импорты во ВСЕХ `.pb.go` репозитория (включая
`v2rayapi`, `v2raygrpc`, `boxdd`, `managed_service`) и в сабмодуле `gvisor`;
`gofumpt` возвращает их к исходному виду. Полный `make -f Makefile.lx lx-proto`
делает оба шага, но его `gofumpt -w .` лезет в сабмодули — известные грабли.
Рабочий порядок: `protogen`, затем `gofumpt -w` точечно по сгенерированным
`.pb.go`, затем `git checkout` в затронутых сабмодулях. После этого дифф
содержит только целевые файлы.

**`lx-proto-install` требует сети даже при уже установленных плагинах** —
`go install` ходит в sumdb за проверкой. При флапающей сети шаг падает, хотя
`protoc-gen-go v1.36.11` / `protoc-gen-go-grpc 1.5.1` уже стоят и совпадают
с пином Makefile. Обход — запустить `protogen` напрямую, предварительно сверив
версии плагинов с `LX_PROTOC_GEN_*_VERSION`.

## Проверки

- `go test -tags with_lx_command ./daemon/` — зелёный, в том числе под `-race`.
- `go test ./daemon/` (без тега) — зелёный, stub отвечает `Unimplemented`.
- `go build` обеих сборок для `daemon` и `libbox` — зелёный.
- `make -f Makefile.lx lx-build` — полный бинарь с LX-тегами собран;
  `./sing-box check -c lx-test/config/minimal.json` — код 0.
- `gofmt -l` / `gofumpt -l` по lx-owned файлам — пусто.
- Дрейф upstream по merge-base: единственный расходящийся subject
  `Fix oomkiller service stub build` (`9fa673d10`) уже поглощён более поздней
  лентой — наш `service/oomkiller/service_stub.go` несёт и `s.network`,
  и более новую сигнатуру `newAdaptiveTimer(…, s.writeOOMReport)`. Не берётся.
- Сабмодули на своих пинах, рабочие деревья чисты.

## Границы реализации

- gomobile-биндинг проверен по правилам SPEC 038 (возврат — объект, строка
  только геттером), но сам AAR локально не собирался: нужен Android SDK,
  это делает CI.
- Полевая проверка (`cdn-cgi/trace` через WG-endpoint и vless-outbound,
  HTTPS на Android без кастомных корней) остаётся за устройством.
## Дополнение — строгая REST-проба HTTP 204 для MagicNet

**Статус дополнения:** реализация и локальная проверка завершены; приёмка на
Android ожидается после сборки MagicNet. Этот отчёт не подтверждает выпуск
или проверку нового core на устройстве.

### Реализованный контракт

`with_lx_probe` добавляет опциональный `expected_status=204` к существующему
Clash delay endpoint. Helper выполняет HTTPS GET через заданный outbound,
проверяет TLS, не следует redirect и признаёт успехом только HTTP 204.
Входящий контекст отмены сохраняется; используются сертификаты и часы ядра.
URL ограничен 4096 байтами, заголовки ответа — 16 KiB, тело не читается.

У завершённого измерения API HTTP 200 содержит `strict_probe:true`,
`expected_status:204`, `ok` и фактический `http_status` либо `null`.
Успех включает `delay`, в том числе 0; отказ — нормализованный `reason`.
Ошибочные параметры получают 400. Стандартный HEAD delay, selector и
история измерений не меняются.

### Зона изменений

Новый код — `common/urltest/http204_lx.go` и пара
`experimental/clashapi/proxy_probe_lx{,_stub}.go`; рядом добавлены три
тестовых файла. Единственная новая зона касания upstream —
`experimental/clashapi/proxies.go:getProxyDelay`: пять строк под
`lx:begin/end strict_http_probe`. Зависимости и сабмодули не менялись.

### Полученные свидетельства

Все команды ниже завершились успешно с `GOMAXPROCS=2`:

- `go test -p 2 -tags with_lx_probe ./common/urltest ./experimental/clashapi`.
- `go test -p 2 ./common/urltest ./experimental/clashapi`.
- `go vet -p 2 -tags with_lx_probe ./common/urltest ./experimental/clashapi`.

Проверки используют реальные локальные TLS fixtures: GET/204, отрицательные
200/403/500/302, отсутствие обращения к redirect-цели, отказ HEAD при
успешном GET, выбранный dialer, доверие сертификату, лимит заголовков,
зависшее ошибочное тело, timeout и cancellation. REST-тесты проверяют
маркеры поддержки на успехе и отказе; stub-тест проверяет прежнюю
диспетчеризацию без тега. `gofmt -l` по новым Go-файлам вернул пустой
вывод; `git diff --check` завершился с кодом 0.

### Открытая приёмка

- [ ] Собрать поставляемый Android core с `with_lx_probe` и проверить
      строгий endpoint на реальном устройстве.
- [ ] Проверить совместно с CLI MagicNet выбор кандидата, подтверждение
      результата и откат при неуспешном восстановлении.

Локальные проверки доказывают поведение HTTP-пробы и её API-контракт.
Они не доказывают доступность внешних узлов с устройства или итоговую
работоспособность всей цепочки восстановления MagicNet.
