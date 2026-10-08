# IMPLEMENTATION_REPORT: 086 — SOCKET_DIAG_REQUEST_STACK

Linux amd64, Intel Core i9-12900HX, Go 1.27.1, `GOMAXPROCS=4`, три прогона
encoding-бенчмарка по 300 ms. Baseline `8d51dc99`, одинаковый benchmark.

| Request | До ns/op (медиана) | После ns/op | До B/op / allocs | После |
|---------|--------------------|-------------|------------------|-------|
| IPv4 TCP | 44.28 | 22.50 | 80 / 1 | 0 / 0 |
| IPv4 UDP | 46.54 | 21.06 | 80 / 1 | 0 / 0 |
| IPv6 TCP | 44.79 | 20.99 | 80 / 1 | 0 / 0 |
| IPv6 UDP | 45.82 | 21.03 | 80 / 1 | 0 / 0 |

Существующий `BenchmarkSocketDiagPool` с реальными локальными datagram
syscall и контролируемыми ответами подтверждает **2 → 1 alloc/op** на
запрос. Общие ns/op и B/op этого короткого прогона шумят из-за планирования
и повторного наполнения `sync.Pool` при GC, поэтому улучшение скорости
syscall/пропускной способности по ним не заявляется.

Компилятор подтверждает: `querySocketDiag` не удерживает `request`.
`packSocketDiagRequest` не inline, но fixed-array return избегает heap.
Обычная сборка `go build -mod=readonly -ldflags=-checklinkname=0 ./...`
и Android ARM64 с полным `release/DEFAULT_BUILD_TAGS_OTHERS` прошли.
Независимое byte-level сравнение 160 комбинаций exact/dump, IPv4/IPv6,
TCP/UDP и наличия destination подтвердило полное совпадение 72 байт с базой.
Существующие tests пакета с `-race` и `go vet` прошли, включая live host
TCP/UDP ownership и параллельную изоляцию запросов.

```sh
GOMAXPROCS=4 go test -mod=readonly -ldflags=-checklinkname=0 -run '^$' \
  -bench '^BenchmarkSocketDiagRequestEncoding$' -benchmem -count=3 \
  -benchtime=300ms ./common/process
GOMAXPROCS=4 go test -mod=readonly -ldflags=-checklinkname=0 -race \
  -count=1 ./common/process
go vet -mod=readonly ./common/process
```

Снижение heap-аллокаций не является доказательством снижения RSS или
энергопотребления. Настройки DNS-cache, таймеров и GC не менялись;
Android-девайс в этой задаче не трогался.
