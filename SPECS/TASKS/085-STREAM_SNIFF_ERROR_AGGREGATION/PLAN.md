# PLAN: 085 — STREAM_SNIFF_ERROR_AGGREGATION

1. В `common/sniff/sniff.go` заменить повторные `E.Errors(accumulated, err)`
   сбором ошибок и одним `E.Errors` после цикла. Slice создаётся лениво,
   только при отказе; вместимость равна числу проб.
2. Новым `common/sniff/error_aggregation_lx_test.go` проверить разные и
   повторные ошибки, более восьми проб, повторное чтение и успешный HTTP.
3. Сравнить одинаковые бенчмарки на базе и изменённом worktree; прогнать
   целевые проверки, race detector и сборки.

Зона конфликта с upstream — только цикл `PeekStream` в
`common/sniff/sniff.go`, все изменения под `lx:begin sniff-error-aggregation`.
Снифферы протоколов, TUN, маршрутизация, выбор узлов и URL-test не правятся.
