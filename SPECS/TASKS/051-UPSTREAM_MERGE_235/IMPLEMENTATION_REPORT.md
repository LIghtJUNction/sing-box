# IMPLEMENTATION_REPORT: 051 — UPSTREAM_MERGE_235

**Фича:** [UPSTREAM_SYNC](../../FEATURES/005-UPSTREAM_SYNC/FEATURE.md)

**Проверено:** 2026-10-30 UTC. **Статус:** I — исходный код реализован и проверен на host; device-прогон остаётся отдельным условием релиза.

Синхронизация включает полный upstream `testing` до `e5d91fa79f6319d5de940d6c81d81f0fe53b9476`, полный требуемый WireGuard delta и сохранённые возможности fork. Финальное дерево прошло целевые тесты, race detector, vet, сборки Linux amd64 / Android arm64 и host-проверки потребителя. A/B-эксперимент sing-tun подтвердил своевременное освобождение TCP-состояния и уменьшение удерживаемого heap на 1 120 000 байт для 10 000 принятых соединений после ACK. Это результат конкретного модульного сценария; RSS Android не измерялся.

Порядок доказательств:

1. Фиксированные исходники и корректность merge при переписанной истории upstream.
2. Перенесённые исправления и сохранение контрактов fork.
3. Реальные тесты и сборки с точными ограничениями среды.
4. Воспроизводимое измерение памяти и границы вывода.
5. Оставшиеся условия публикации и device-проверки.

## 1. Идентичность исходников и полнота merge

| Объект | Фиксированная ревизия |
|---|---|
| Исходный fork | `ef1ac197876a45aee818ee608b15740c10b8fa3b` |
| Ранее записанная upstream-база | `5143c708b1c5410ebdd30c70faeeafdbc3db3d3c`, 1.15.0-alpha.10 |
| Интегрированный PR #3484 | head `67574057d1e5584f131af53870078b5b0cbb35b8` |
| Содержательная база трёхстороннего слияния | `a4331b8d745971fdc1be33bba5513ab16105e2d3` |
| Целевой upstream | [`e5d91fa79f6319d5de940d6c81d81f0fe53b9476`](https://github.com/SagerNet/sing-box/commit/e5d91fa79f6319d5de940d6c81d81f0fe53b9476), testing после 1.15.0-alpha.13 |
| Канонический merge | [`3d3053dccec631c1b5406feb96a2096ecae9abd9`](https://github.com/LIghtJUNction/sing-box/commit/3d3053dccec631c1b5406feb96a2096ecae9abd9) |
| Канонический исходный commit | [`d6316199ef905c8271e58b23dbf52456d2e49dde`](https://github.com/LIghtJUNction/sing-box/commit/d6316199ef905c8271e58b23dbf52456d2e49dde) |
| Локальный commit, использованный в финальных проверках | `9caf2637c42113a40f1302b87ff9525220a0ff54` |
| Одинаковый tree обоих исходных commit | `a6b2907483435e507d5090cb5ce00d84899d10f5` |

Переписывание `upstream/testing` потребовало сравнения зафиксированных исходных деревьев. После интеграции PR #3484 содержательной трёхсторонней базой служил уже включённый срез `a4331b8…`; он не подменяет родителей Git-merge. Канонический merge сохраняет два реальных родителя: fork `88fbd1aa4ea9657352dd163be313914ec94a4052` и upstream `e5d91fa79f6319d5de940d6c81d81f0fe53b9476`. В локальном графе этому соответствуют merge `76ffc6c58a3e0fcdefb8f82c05de03b1c023d4b1` и первый родитель `b74544593d76ec02cffe32f65561371bb70d71a8`. История fork сохранена обычными merge; force-push не использовался.

Проверка полноты охватила все 166 путей, изменённых upstream между `a4331b8…` и `e5d91fa…`. Для 132 путей, не изменённых fork относительно этой базы, конечные blob совпадают с upstream; оставшиеся 34 пути требуют совмещения с дельтой fork. Не найдено потерянных upstream-добавлений, отсутствующих upstream-путей или путей, где требуемое изменение осталось старым fork-blob. `git merge-base` финального локального commit и upstream равен `e5d91fa…`. Список всех путей и машинные результаты сохранены в [final-tree-audit.json](validation/final-tree-audit.json).

При сохранении объектов через GitHub API изменились идентификаторы commit, но сохранённые деревья проверены на точное совпадение. Поэтому логи и `version` честно содержат локальный SHA `9caf2637c421`, а опубликованный объект исходников обозначен каноническим `d6316199…`. Commit этого отчёта добавляет только документацию поверх проверенного дерева. Финальные результаты ниже относятся к `9caf2637c…`, а не к промежуточному `88a084204…`.

## 2. Что изменено и почему

### Upstream lifecycle и зависимости

В `sing-tun` полностью обновлена зависимость с `v0.9.7-0.20261002083955-3f8acd9da65b` до `v0.9.7-0.20261028024126-5b0a1a7a394e`. Временная запись принятого TCP SYN удаляется после ACK; NAT учитывает двусторонний FIN и RST и освобождает завершённые сессии через штатный sweeper. Активные и односторонне закрытые соединения сохраняются. Зафиксированы согласованные `go.mod` / `go.sum`, test-module зависимости, `upstream.commit` и `upstream.version`.

Upstream cleanup в `route/conn.go` закрывает обе стороны неудачного UDP handshake и вызывает `onClose` ровно один раз. Синхронизация в `route/flow_tracker.go` защищает регистрацию и удаление flow при пересечении AttachFlow/CloseFlow. Регрессионный тест задерживает возврат remover после регистрации во второй группе и параллельно запускает закрытие: проверяется именно это перекрытие операций, без обещания нового контракта для регистрации после уже завершившегося CloseFlow.

Tracked-копия `submodules/wireguard-go` обновлена полной разницей деревьев `ca3bc60c4ce798c157689a9d653041470e627795` → `fdf0ad04658666a57f15da1b0eea30c08b0a94db`. Обновление затрагивает `conn/bind_std.go`, `device/device.go`, `device/receive.go`, `device/send.go` и `device/timers.go`: восстановление полного IPv6-адреса из переиспользуемого буфера, учёт времени сна при возрасте ключей и handshake, ожидание resume вне timer locks и отбрасывание устаревших callback. Переписанные upstream-коммиты PSK и blocking injection сверены по содержимому; эквивалентный уже перенесённый код сохранён. Точный provenance и регрессии описаны в [VENDORED.md](../../../submodules/wireguard-go/VENDORED.md).

### Сохранение fork и минимальные исправления после merge

| Зона | Сохранённый контракт и адаптация |
|---|---|
| DNS cache / query stream | Каждый отклонённый свежий или кешированный ответ имеет ровно одно событие отказа с правильными resolver/process metadata. Два новых upstream cache-rejection пути дополнены существующим `emitFailedQuery`. |
| WireGuard pause / idle-suspend | Callback читает актуальное состояние pause manager; отсутствие manager до Start безопасно. При отсутствии device состояние manager всё равно учитывается, а protocol suspension и idle ownership сохраняются. Новые locks не добавлены. |
| Chain / scoped connection manager | Внешний поток сохраняет классификацию через новый context API; тестовые fixtures используют новый constructor и производственный порядок регистрации URLTest history. |
| Tailscale / eBPF | Socket protection остаётся в системных bind-путях, primary UDP listener и динамических UDP egress sockets; новый upstream default-route hook сохранён. |
| AWG и прочие возможности | Сохранены AWG headers/padding/timers, bind recovery, XHTTP, VLESS Encryption, DNS-группы, URLTest, lx command/chain и idle-suspend. Ранее принятые socket-diag/sniff исправления не откатываются. |

Две обнаруженные регрессии слияния исправлены **пятью дополнительными production-строками**: две строки в `dns/client.go` и nil-manager guard из трёх строк в `transport/wireguard/endpoint.go`. Это размер двух дополнительных исправлений, а не всей синхронизации. Исправления изолированы в commit [`2be9bef…`](https://github.com/LIghtJUNction/sing-box/commit/2be9bef574f82ae956b88150ef6949e0c40f9d30) и [`e8b3566…`](https://github.com/LIghtJUNction/sing-box/commit/e8b356654a822fa9edf1068b680e694c1ee05c4e), помечены `// lx:` и проверяются отдельными тестовыми файлами.

Оба `release/DEFAULT_BUILD_TAGS*` остались неизменными относительно исходного fork; production-прогон использует весь `DEFAULT_BUILD_TAGS_OTHERS`. Module path остаётся `github.com/sagernet/sing-box`. Local replace WireGuard в корневом и test-модуле указывает на обновлённую tracked-копию, а не на старые исходники под новой require-версией. Размер UDP/GRO-буфера 65535 и действующие настройки GOMEMLIMIT/GOGC не уменьшались ради результата по памяти.

### Зоны следующего слияния

Собственные швы, которые нужно повторно проверить при следующем upstream merge: `dns/client.go`; `route/conn.go` и `route/flow_tracker.go`; новый context API в `adapter` и `protocol/chain`; socket protection в `protocol/tailscale`; pause/shutdown/idle-suspend в `transport/wireguard/endpoint.go`; пять перечисленных WireGuard-путей и local replace в `go.mod` / `test/go.mod`. Новые регрессии находятся в `route/conn_handshake_lifecycle_test.go`, `route/flow_tracker_lifecycle_test.go`, `dns/client_cache_observability_lx_test.go`, `transport/wireguard/endpoint_pause_state_lx_test.go`, `submodules/wireguard-go/conn/address_reuse_lx_test.go` и `submodules/wireguard-go/device/timers_pause_lx_test.go`.

## 3. Валидация конечного исходного дерева

Все финальные проверки использовали Go 1.26.10. Production CLI собирались реальным [helper потребителя](https://github.com/LIghtJUNction/magicnet-android/blob/0952cb01b001e41e1074c2e5f4a731a4ad42b222/scripts/build-sing-box.sh), с тегами и ldflags из ядра, `-mod=readonly`, `GOTOOLCHAIN=local`, `GOWORK=off`, `CGO_ENABLED=0`. Подробные команды и неизменённые текстовые логи собраны в [validation/README.md](validation/README.md).

| Проверка | Результат и область доказательства |
|---|---|
| Default `go test ./...` | 47 пакетов с `ok`: 45 выполняли тесты, 2 имеют `[no tests to run]` после явных исключений; ещё 111 пакетов без тестовых файлов. Использован `-ldflags=-checklinkname=0`. |
| Целевые tests с полными production tags | 22 пакета с исполненными тестами прошли; ещё 11 без тестовых файлов. Это целевой набор пакетов, не полный `./...` с production tags. |
| Финальный lifecycle `-race` | 11 выбранных тестов в `route`, `dns`, `transport/wireguard`, `-count=1`, exit 0. Проверяются handshake cleanup, пересечение Attach/Close, DNS attribution и pause/shutdown/idle. |
| Явный `go vet` | `route`, `dns`, `transport/wireguard` и отдельно все пакеты `submodules/wireguard-go`: exit 0. |
| Linux amd64 CLI | Полный production-набор; сборка конечного дерева и `version` прошли. |
| Android arm64 CLI | Тот же конечный исходный tree и production-набор; cross-build прошёл. |
| TUN config fixtures | Positive и reject-sentinel fixtures проверены конечным Linux CLI, exit 0. |
| Loopback DNS | IPv4 и IPv6, TCP и UDP — четыре проверки прошли конечным Linux CLI. |

Default-прогон явно исключил 13 верхнеуровневых тестов, которым недоступны необходимые возможности среды: один network namespace, три socket/process lookup, три внешних TLS-соединения и шесть raw-socket TLS spoofing tests. Все имена, точный anchored `-skip` и причины приведены в [validation/README.md](validation/README.md). Сами тесты и их assertions не ослаблены; эти исключения не считаются успешной проверкой соответствующей функциональности.

Красно-зелёные проверки подтверждают причинность исправлений. На исходном `ef1ac197…` UDP handshake не вызывал close callback, а пересечение Attach/Close обнаруживало data race и оставшуюся регистрацию; на обновлённом коде оба теста прошли с `-race -count=10`. На промежуточном `88a084204…` три кешированных DNS-пути не выдавали событие отказа, а не запущенный WireGuard endpoint паниковал при pause event; после минимальных исправлений тесты прошли с `-race -count=3`. Эти случаи затем повторно вошли в архивный финальный lifecycle-прогон. Отдельные WireGuard регрессии воспроизвели старые ошибки IPv6 address reuse и timer cancellation, как зафиксировано в provenance зависимости.

Идентичность проверенного Linux CLI: версия `1.15.0-alpha.13+LIghtJUNction.9caf2637c421`, размер 86 528 126 байт, SHA-256 `a74373e5b34d4c5363750472035e3151674808d2bf985a1428543dbffb09712b`. Это fingerprint тестового бинаря, а не измерение потребления памяти. [final-source-integration-checks.json](validation/final-source-integration-checks.json) фиксирует исходный SHA/tree, consumer base и все exit codes. После relink версии на канонический SHA fingerprint бинаря изменится; результаты относятся к одинаковому проверенному исходному дереву.

## 4. Измеренная память: что именно улучшилось

A/B сравнивает реальные таблицы sing-tun на двух фиксированных версиях, одинаковом Go 1.26.10, `GOMAXPROCS=1`, default tags и одинаковых входных пакетах. Выполнено три прогона для 1 000 и 10 000 flows. Исходные `go.mod` обеих зависимостей совпадают побайтно; общий тест отличается только адаптерами старых сигнатур Lookup/LookupBack. Runner, исходные логи и 60 наблюдений сохранены в [validation](validation/README.md).

Результат для 10 000 flows, одинаковый во всех трёх прогонах:

| Метрика | Старая версия | Новая версия |
|---|---:|---:|
| Временные TCP-записи после ACK, шт. | 10 000 | 0 |
| Прирост HeapAlloc после GC на этом пути, байт | 2 561 072 | 1 441 072 |
| NAT-сессии после двустороннего FIN и 31 с, шт. | 10 000 | 0 |
| NAT-сессии после RST и 31 с, шт. | 10 000 | 0 |
| Активные NAT-сессии после 31 с, шт. | 10 000 | 10 000 |
| NAT-сессии с односторонним FIN после 31 с, шт. | 10 000 | 10 000 |

Уменьшение удерживаемого heap на ACK-пути составляет **1 120 000 байт**, около **1,07 MiB**; при 1 000 flows — около **112 000 байт**. HeapAlloc измерен после `runtime.GC` относительно пустой таблицы, с `runtime.KeepAlive` для таблицы под проверкой. Go map сохраняет ёмкость после удаления элементов, поэтому итоговый прирост heap остаётся ненулевым. Счётчики retained state показывают освобождение объектов, а контрольные active/half-FIN сценарии проверяют отсутствие преждевременной очистки.

NAT-тест использует настоящий фоновый sweeper; `testing/synctest` ускоряет время, не удаляет записи вручную. Измерение не охватывает `allocs/op`, RSS процесса, Android memory pressure, throughput, батарею или реальный сетевой туннель. Процент снижения общей памяти приложения по этим данным не заявляется.

## 5. Статус приёмки и следующие шаги

Исходный код и перечисленные host-проверки завершены; состояние задачи — **I (implemented)**. Обновление gitlinks, версии и provenance в `magicnet-android` и `MagicNet` выполняется следующим шагом в потребительских репозиториях после публикации проверенного ядра. Подтверждение этих ссылок принадлежит их собственным доказательствам; этот отчёт не отмечает их заранее выполненными.

Device gate **не пройден**: Android TUN/interception, реальный AWG/WireGuard-трафик, pause/wake и смена сети на устройстве не проверялись. Перед отдельным релизным тегом необходим предусмотренный SPEC device-прогон, в том числе проверка сохранённых AWG и idle-suspend контрактов. Ни release tag, ни статус D (device-verified) в рамках этой валидации не создавались.

### Самопроверка утверждений

| Критерий | Итог |
|---|---|
| Вклад | Полная upstream-синхронизация и восстановление существующих fork-контрактов подтверждаются фиксированными деревьями и регрессиями. |
| Ясность | SHA, tree, версии зависимостей, команды, единицы измерения и область каждой проверки записаны явно. |
| Сила эксперимента | Есть красно-зелёные lifecycle-проверки и три одинаковых 10 000-flow измерения с активными контрольными сценариями. |
| Полнота оценки | Host-область покрыта перечисленными проверками; 13 исключений, отсутствие device gate и RSS-измерений видимы и не скрыты статусом. |
| Корректность метода | Сохранены production tags, UDP/GRO и GC-настройки; применены upstream lifecycle-механизмы и минимальные помеченные дополнения, без новой политики памяти. |
