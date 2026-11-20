# TASKS 019 — расширение lazy_start

**Фича:** [URLTEST_BALANCE](../../FEATURES/007-URLTEST_BALANCE/FEATURE.md)

- [x] Opt-in option default false; собственные startup/reset гейты.
- [x] First Touch/Attach без ожидания interval; cold reads не активируют группу.
- [x] Pending first-check owner, pause/wake registration, idle generation.
- [x] Forced manual и nested parent dependency не изменены.
- [x] Close отменяет начальный цикл и external-context reset; callback удаляется,
      ticker окончательно останавливается после удаления.
- [x] net.Pipe fixture подтверждает startup 135 → 0 и reset 135 → 0 до трафика.
- [x] Новые regression/race tests и две языковые страницы конфигурации.
- [x] Финальный full-package race/vet, untagged и full release-tags host build.
- [ ] Release-tags Android ARM64 build (финальный release gate; результат собирается отдельно).
- [ ] Полевой замер батареи/RTT (вне текущего host-патча; не заявлять результат).
