# PLAN 019 — расширение lazy_start

**Фича:** [URLTEST_BALANCE](../../FEATURES/007-URLTEST_BALANCE/FEATURE.md)

1. Добавить opt-in bool `lazy_start`, default false, без новой feature/build-tag.
2. Изолировать lazy lifecycle в `urltest_lazy_start_lx.go`; общие точки касания
   только `option/group.go` и `protocol/group/urltest.go`, под lx-маркерами.
3. Использовать существующий ticker как activity lease; не вводить отдельный
   «использованный» статус. Сохранить seed/fallback, force API и nested dependency.
4. Защитить pending/scheduled начальный цикл mutex группы; pause callback не берёт
   этот mutex. Регистрацию reconcile-ить атомарными pause-флагами.
5. Связать автоматический network-reset с context группы и context вызывающего;
   повторно проверять lifecycle после очереди.
6. Отдельные коммиты: новые файлы/спеки/тесты, затем lx-швы общих файлов.
7. Проверить реальный URL-test over net.Pipe: 5 × 27 count fixture, defaults,
   first Touch/Attach, forced manual, idle, pause edges, nested, Close/race;
   собрать Android ARM64 с release tags. Устройство и его конфиг не менять.
