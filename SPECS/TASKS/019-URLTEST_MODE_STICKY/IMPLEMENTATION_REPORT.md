# IMPLEMENTATION REPORT 019 — lazy_start

**Фича:** [URLTEST_BALANCE](../../FEATURES/007-URLTEST_BALANCE/FEATURE.md)

Дата: 2026-10-08. База: `231ae3c0`. Статус расширения: I, host-verified.

## Изменение

`lazy_start:true` откладывает собственные startup/reset-пробы неиспользованной
группы. First Touch/Attach немедленно запускает non-force цикл; activity-ticker
определяет участие в reset. Default false оставляет старый lifecycle. Manual
force и проверка дочерних зависимостей активным родителем сохранены.

Новый lifecycle изолирован в `protocol/group/urltest_lazy_start_lx.go`.
Опции/проводка — lx-швы `option/group.go`, `protocol/group/urltest.go`.
Существующие выбирающий алгоритм, batch concurrency, HTTP/TLS проверка,
таймауты проб и passive/history правила не изменены.

## Измерение и границы

Host sentinel отвечает HTTP HEAD через net.Pipe, без DNS/реальной сети.
5 независимых групп × 27 членов реально выполняют 135 startup и 135 дополнительных
network-reset-проб при false; при true до трафика обе величины равны 0.
Активная группа всё ещё проверяется; first Touch работает при interval=1m,
не ожидая этого интервала. Ручной тест дважды повторяет все свежие пробы.

Это измерение исключённых фоновых запросов, не процента батареи и не RTT узла.
Новый флаг на телефоне не включался, полевого energy/throughput замера нет.
С nested URLtest активный родитель всё ещё пробует неиспользованную зависимость.
Wake без следующего traffic Touch не обязан немедленно проверять unused группу.

## Проверки

Новые tests: counts, cold fallback/round-robin pool, repeated force, Attach,
paused Touch, idle restart, pending first cycle, missed wake registration,
real pause × Touch race, Touch × Close race, first/reset cancellation, queued
reset recheck, nested dependency. Финальная версия (atomic pause callback и final Stop после Unregister):
`go test -race -ldflags=-checklinkname=0 ./protocol/group -run '^TestURLTestLazyStart' -count=5 -v`
— PASS, 4.138s package time.
`go test -race -ldflags=-checklinkname=0 ./protocol/group ./common/urltest ./option`
— PASS; `go vet -ldflags=-checklinkname=0` для тех же пакетов — PASS.
`GOMAXPROCS=2 go build -p=2 -ldflags=-checklinkname=0 ./...` — PASS.
Full release-tags host build — PASS; `sing-box version` подтверждает полный
набор `release/DEFAULT_BUILD_TAGS_OTHERS`. SHA256 host кандидата:
`4f226c9558158efcf9cc781956a63b56820575ebb57a9ecf46a96e43e18cdc27`.
Android ARM64 — отдельный финальный release gate, ещё не был завершён
на момент фиксации этого host-verified отчёта.

Host raw evidence: `/tmp/magicnet-singbox-perf-evidence-20261008/lazy-start-race-count5.txt`.
Документация не содержит конфигурацию, credentials или идентификаторы устройства.
