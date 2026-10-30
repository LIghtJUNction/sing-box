# План синхронизации

1. Проверить существующий PR #3484 и сохранить применимые исправления через обычный merge; изменения fork после его ответвления сохраняются.
2. Трёхсторонне перенести полную WireGuard-разницу ca3bc60c4ce798c157689a9d653041470e627795 → fdf0ad046586. Обновить VENDORED.md и regression tests.
3. Слить фиксированный upstream e5d91fa79f6319d5de940d6c81d81f0fe53b9476. Проверить scoped cleanup, flow tracker и собственные lifecycle hooks при разрешении конфликтов.
4. Сохранить fork replace, release build-теги и lx-only файлы. Поднять upstream.commit/upstream.version и test-module зависимости согласованно.
5. Воспроизвести целевые ошибки на старом коде, затем проверить новый код и race detector. Собрать оба CLI target с production tags; проверить минимальный конфиг и интеграционные сценарии потребителя.
6. Записать реальные результаты в IMPLEMENTATION_REPORT.md, затем обновить фиксированные ссылки потребителя и provenance рецепта.

## Зоны ручной адаптации

- upstream-файлы: go.mod/go.sum, adapter/* lifecycle interfaces, route/flow_tracker.go, route/conn.go, transport/device, transport/wireguard, protocol endpoints, consumer tests.
- fork-файлы: lx lifecycle/cleanup hooks и regression tests; lx build-tags остаются включены.
- tracked dependency: submodules/wireguard-go; полный upstream delta плюс сохранённая AWG разница.
- protobuf не редактируется вручную; при изменении proto требуется штатная регенерация.
