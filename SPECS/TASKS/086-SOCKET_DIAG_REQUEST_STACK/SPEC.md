# SPEC: 086 — SOCKET_DIAG_REQUEST_STACK

**Фича:** [HOTFIXES](../../FEATURES/004-HOTFIXES/FEATURE.md)

| Поле | Значение |
|------|----------|
| Тип | R (refactor) — аллокации существующего owner-query |
| Статус | C (complete) — host-tests, race/vet, обычная и Android ARM64 release-сборки; без полевой RSS/энергопроверки |
| Base | `8d51dc993c4862fbd61e259a1d852aa6e04f40e9` |

## Контракт

Netlink-request фиксированного размера возвращается по значению и живёт
в вызывающем стеке до завершения синхронной отправки. Формат сообщения,
UDP-обмен местами endpoints, IPv4/IPv6, dump-флаги, locks и retries прежние.
Новых настроек или публичных интерфейсов нет.

Кодировка запроса ранее выделяла 80 B и один объект на heap. Изменение
устраняет эту временную аллокацию, не ограничивает heap или DNS-cache и
не меняет GOGC/GOMEMLIMIT. Оно не обещает снижение RSS, сетевой задержки
или расхода батареи на конкретном устройстве.

## Приёмка

- IPv4/IPv6 TCP/UDP encoding — ноль heap-аллокаций.
- Существующие kernel lookup, UDP wildcard/dump, concurrent identity,
  buffer isolation и blocked-lane проверки проходят с `-race`.
- Кодирование netlink-полей не меняется; не сохраняется ссылка на
  caller-owned stack после syscall.
- Обычная и полная Android ARM64 release-сборки проходят.
- Снятие: заменить патч, когда базовая query-реализация сама уберёт аллокацию.
