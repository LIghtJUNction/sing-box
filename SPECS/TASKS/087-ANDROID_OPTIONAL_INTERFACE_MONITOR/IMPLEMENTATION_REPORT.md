# Проверка 087

## Изменение и основание

Android CLI с `auto_detect_interface=false` и без явного default binding
сохраняет попытки через системную маршрутизацию, даже если необязательный
monitor не нашёл default-интерфейс. Новый helper отменяет прежний update
context и сохраняет pending reset до достоверного восстановления. В остальных
режимах сеть остаётся на паузе с сообщением об ожидании интерфейса.

Исходный `dispatchInterfaceUpdateLocked` безусловно вызывал `NetworkPause()`.
Это проверено на базе `d0b1422df5c36f49e7dbbccd0d49732040051974`. Официальный
[Android monitor в фиксированной зависимости](https://github.com/SagerNet/sing-tun/blob/5b0a1a7a394e/monitor_android.go)
ищет таблицу по маске `0xffff`; отсутствие результата не доказывает отказ
обычного Android socket routing. Лог устройства не позволяет отдельно
доказать OEM-несовместимость или настоящий запуск без сети.

`InterfaceMonitor` и публичные options не меняются. DHCP auto-interface,
интерфейсные правила и протоколы, которым действительно нужна привязка,
продолжают видеть отсутствие интерфейса. Ошибки сокетов не перехватываются;
фиктивное состояние «сеть доступна» не публикуется.

## Выполненная проверка

Go 1.26.10, Linux amd64, `GOMAXPROCS=2`. Команды выполнены из корня core.

```sh
go test -p 2 ./route -run '^TestMissingDefaultInterface' -count=1
go test -mod=readonly -p 2 ./route -count=1
go test -race -p 2 ./route ./protocol/group ./transport/wireguard \
  -run '^(TestMissingDefaultInterface|TestResetNetworkBeforeInitialize|TestURLTestLazyStart.*Pause|TestApplyPauseEvent|TestOnPauseUpdated|TestPauseEvent|TestCloseAndTeardownBumpPauseStamp)' \
  -count=1
go vet -mod=readonly -p 2 ./route
GOOS=android GOARCH=arm64 CGO_ENABLED=0 \
  go build -mod=readonly -p 2 \
  -tags "$(cat release/DEFAULT_BUILD_TAGS_OTHERS)" ./route
git diff --check
```

Все команды после исправления завершились с кодом 0. До изменения helper
точно воспроизводил старые две строки `NetworkPause`/`Error`: новый тест
Android system routing получил `paused=true`, проверки отмены и pending
reset тоже отказали. Всего в красном прогоне отказали два верхнеуровневых
теста. После замены helper все три новых теста прошли.

| Проверка | Наблюдаемый результат |
| --- | --- |
| Новые регрессии | 3 верхнеуровневых теста; policy matrix содержит 8 подслучаев |
| Пакет `route`, default tags | Все 14 верхнеуровневых тестов прошли |
| Выборка с `-race` | 15 верхнеуровневых тестов: route 5, group 3, wireguard 7 |
| Уникальные тесты двух положительных выборок | 24 верхнеуровневых теста; повторные прогоны не прибавлены |
| Восстановление | Реальный callback и pause manager, два синтетических интерфейса; один reset на каждое восстановление |
| Android arm64 | Компиляция `./route` со всеми 23 production tags, CGO=0 |

## Исполняемый host core и TUN check

Рабочее дерево собрано в Linux amd64 CLI со всеми 23 production tags.
Версия явно помечена `1.15.0-alpha.13+magicnet-interface-fix-dev`:
это проверочный бинарь, не релизный артефакт или новая подписанная версия.

```sh
CGO_ENABLED=0 GOTOOLCHAIN=local GOWORK=off \
  go build -mod=readonly -p 2 -trimpath -buildvcs=false \
  -tags "$(cat release/DEFAULT_BUILD_TAGS_OTHERS)" \
  -ldflags "-X github.com/sagernet/sing-box/constant.Version=1.15.0-alpha.13+magicnet-interface-fix-dev $(cat release/LDFLAGS)" \
  -o /workspace/scratch/d050d6bac77d/sing-box-interface-check ./cmd/sing-box
```

`version` подтвердил Go 1.26.10, linux/amd64, CGO disabled и полный список
tags. `check` выполнен на временном минимальном JSON с одним TUN
(`address=["172.19.0.1/30"]`, `auto_route=false`), direct outbound и
`route.auto_detect_interface=false`; интерфейс и маршруты не создавались.

| Контроль | Exit | Вывод |
| --- | --- | --- |
| TUN без `stack` | 0 | Пустой; deprecated warning отсутствует |
| Тот же TUN с `stack="mixed"` | 0 | Предупреждение `stack` deprecated в 1.15 / удаление в 1.17 воспроизведено |

## Границы

Host-тесты не запускают Android netlink, TUN, eBPF или внешнее подключение.
Android package build проверяет компиляцию, а не работу устройства. Нужны
отдельные проверки запуска без сети, подключения и Wi-Fi/сотовой смены на
устройстве жалобы. Текущий статус — `I`, не `D`.

## Самопроверка утверждений

| Вопрос | Основание и итог |
| --- | --- |
| Что исправлено? | Измеряемая лишняя пауза и незакрытый update context, без новых сетевых возможностей |
| Ясна ли граница? | Android CLI, auto-detect выключен, нет default bind/strategy/type |
| Есть ли отрицательная проверка? | Старый helper провалил новые регрессии; исправленный прошёл |
| Полна ли оценка? | Host/race/vet и Android compile выполнены; device прямо оставлен открытым |
| Есть ли скрытое утверждение об онлайн-состоянии? | Monitor остаётся `nil`; настоящие ошибки подключения не подавляются |
