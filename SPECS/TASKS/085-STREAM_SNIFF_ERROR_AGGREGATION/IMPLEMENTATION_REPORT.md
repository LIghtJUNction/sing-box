# IMPLEMENTATION_REPORT: 085 — STREAM_SNIFF_ERROR_AGGREGATION

Изменены один upstream-файл (`common/sniff/sniff.go`, отмеченный lx-шов),
новый тестовый файл и документация существующей фичи SNIFF.

Пять прогонов на Linux amd64, Intel Core i9-12900HX, Go 1.27.1,
`GOMAXPROCS=4`; база `b7eb313f`, одинаковые новые тесты в обоих worktree.
Ниже медианы; ns/op зависят от состояния хоста.

| Вектор | До ns/op | После ns/op | До B/op / allocs | После B/op / allocs |
|--------|----------|-------------|------------------|--------------------|
| Восемь разных отказов | 7759 | 1564 | 5896 / 124 | 1080 / 20 |
| Те же отказы с кешированным фрагментом | 8966 | 2792 | 7240 / 164 | 2424 / 60 |
| Успех первой пробы | 104.7 | 98.92 | 48 / 1 | 48 / 1 |
| Успех первой пробы с кешированным фрагментом | 251.8 | 249.4 | 216 / 6 | 216 / 6 |

Отдельный HTTP-вектор через настоящие `TLSClientHello` → `HTTPHost`:
7689 → 7545 B/op, 29 → 23 аллокации. Временные результаты этого дополнительного
вектора не публикуются как стабильная оценка из-за нагрузки хоста.

Команда воспроизведения:

```sh
GOMAXPROCS=4 go test -mod=readonly -ldflags=-checklinkname=0 \
  -run 'TestPeekStream' -bench BenchmarkPeekStreamErrorAggregation \
  -benchmem -count=5 ./common/sniff
go test -mod=readonly -ldflags=-checklinkname=0 -race -count=1 \
  ./common/sniff ./common/process ./dns/transport ./protocol/group
go vet -mod=readonly ./common/sniff ./common/process ./dns/transport ./protocol/group
```

Сборки без тегов (`go build -mod=readonly -ldflags=-checklinkname=0 ./...`)
и Android ARM64 с полным `release/DEFAULT_BUILD_TAGS_OTHERS` прошли:

```sh
GOMAXPROCS=4 CGO_ENABLED=0 GOOS=android GOARCH=arm64 \
  go build -mod=readonly -trimpath -buildvcs=false \
  -tags "$(cat release/DEFAULT_BUILD_TAGS_OTHERS)" \
  -ldflags "$(cat release/LDFLAGS) -s -w" -o /tmp/sing-box-arm64 ./cmd/sing-box
```

Race detector и vet пройдены. Отдельные тесты подтверждают `errors.Is`/`errors.As`,
точный порядок уникальных сообщений, число проб больше восьми, повторное
чтение по обёрнутому `ErrNeedMoreData` с прежним общим дедлайном и очисткой, а существующие reader-тесты —
повторное чтение одних байтов каждым сниффером.

Это микробенчмарки stream-sniff, не изменение end-to-end задержки узлов.
Установка и стабильность нового Android-бинаря здесь не утверждаются.
