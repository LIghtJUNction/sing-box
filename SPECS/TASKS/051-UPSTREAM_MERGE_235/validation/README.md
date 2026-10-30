# Воспроизводимая проверка жизненного цикла sing-tun

Проверка отвечает на два вопроса: удаляется ли временная TCP-запись после ACK и освобождаются ли NAT-сессии после FIN/RST. Это модульный эксперимент, а не измерение RSS Android.

## Фиксированные входы

- Старый sing-tun: `v0.9.7-0.20261002083955-3f8acd9da65b`.
- Новый sing-tun: `v0.9.7-0.20261028024126-5b0a1a7a394e`.
- Go 1.26.10, Linux amd64, `GOMAXPROCS=1`, default tags, три прогона.
- Исходные go.mod совпадают побайтно: SHA-256 `0dde4758a9da6cfd3e570c73d62390d49dbe24b7bed4c8960c98150916d3a2c6`.
- Общий тест `lifecycle_ab_test.go.txt`; адаптер меняет только сигнатуры Lookup/LookupBack, поскольку старый API не принимал TCP flags.

```sh
python3 run_lifecycle_ab.py --go /path/to/go --runs 3 --work-dir /tmp/new-lifecycle-ab
```

Рабочая папка должна быть новой. Runner копирует модули, не меняет Go module cache, проверяет 60 наблюдений и сохраняет результаты. Файлы old.log/new.log/results.json содержат исходный прогон; сам опубликованный runner также выполнен полностью, с теми же результатами для 10 000 потоков.

## Результат

| Нагрузка | Старая версия | Новая версия |
|---|---:|---:|
| 10 000 принятых TCP SYN после ACK: оставшиеся записи | 10 000 | 0 |
| Тот же путь: прирост HeapAlloc после GC, байт | 2 561 072 | 1 441 072 |
| 10 000 NAT-сессий, двусторонний FIN, после 31 с | 10 000 | 0 |
| 10 000 NAT-сессий, RST, после 31 с | 10 000 | 0 |
| 10 000 активных NAT-сессий, после 31 с | 10 000 | 10 000 |
| 10 000 NAT-сессий только с односторонним FIN | 10 000 | 10 000 |

Уменьшение удерживаемой памяти на пути ACK — 1 120 000 байт, около 1,07 MiB. Все три измерения на 10 000 потоков совпали. При 1 000 потоках сокращение около 112 000 байт; небольшая вариация абсолютных heap-счётчиков между перезапусками ожидаема.

HeapAlloc измеряется после runtime.GC относительно пустой таблицы; runtime.KeepAlive удерживает stage до чтения метрики. Даже после удаления записей Go map сохраняет ёмкость, поэтому новая версия не возвращается к нулевому приросту heap. NAT использует настоящий фоновый sweeper, а testing/synctest только ускоряет время; тест не удаляет записи вручную. Активные и полуоткрытые соединения служат контролем против слишком ранней очистки.

Из результатов не следует процент снижения RSS всего процесса, экономия батареи или ускорение реального Android-туннеля. Эти свойства требуют отдельного устройства и нагрузки.

## Проверки конечного исходного дерева

[IMPLEMENTATION_REPORT.md](../IMPLEMENTATION_REPORT.md) связывает результаты с исходным commit, merge и критериями приёмки. Канонический commit `d6316199ef905c8271e58b23dbf52456d2e49dde` и локальный проверенный `9caf2637c42113a40f1302b87ff9525220a0ff54` имеют одинаковый tree `a6b2907483435e507d5090cb5ce00d84899d10f5`. В версиях собранных бинарей сохранён фактический локальный SHA. Последующий commit отчёта меняет только документацию.

| Файл | Что подтверждает |
|---|---|
| [final-tree-audit.json](final-tree-audit.json) | Полнота переноса 166 upstream-путей и фиксированные merge-идентификаторы |
| [final-default-tests.log](final-default-tests.log) | 47 пакетов с `ok`: 45 с исполненными тестами, 2 с `[no tests to run]`; ещё 111 без тестовых файлов |
| [final-production-tests.log](final-production-tests.log) | 22 пакета с тестами прошли с полным production-набором тегов; ещё 11 без тестовых файлов |
| [final-lifecycle-race.log](final-lifecycle-race.log) | 11 выбранных lifecycle-тестов в route, dns и transport/wireguard; `-race`, `-count=1`, exit 0 |
| [final-vet.log](final-vet.log) | Явный `go vet` для route, dns и transport/wireguard, exit 0 |
| [final-wireguard-vet.log](final-wireguard-vet.log) | Явный `go vet ./...` из `submodules/wireguard-go`, exit 0 |
| [build-linux-final-source.log](build-linux-final-source.log) | Production-сборка конечного исходного дерева для Linux amd64 |
| [build-android.log](build-android.log) | Production-сборка того же дерева для Android arm64 |
| [final-source-integration-checks.json](final-source-integration-checks.json) | Версия и SHA-256 конечного Linux CLI, TUN-fixtures и четыре loopback DNS-проверки, все exit 0 |

Go 1.26.10 использовался во всех перечисленных конечных проверках. Default-прогон допускает стандартное повторное использование Go test cache; это отражено в исходном логе. Команды явного vet и race записаны в соответствующих файлах. Полный default-прогон имел следующие явные исключения; они не считаются пройденными тестами:

```sh
skip_tests=(
  TestUnshareNamespace
  TestDumpSocketDiagTCP
  TestDumpSocketDiagUDP
  TestLinuxSearcherFindProcessInfo
  TestTLSRecordFragment
  TestTLSFragment
  TestTLS2Fragment
  TestIntegrationConn_RealTLSHandshake
  TestIntegrationSpoofer
  TestIntegrationConn_InjectsThenForwardsRealCH
  TestIntegrationConn_IPv6_InjectsThenForwardsRealCH
  TestIntegrationConn_FakeAndRealHaveDistinctSNIs
  TestIntegrationConn_IPv6_FakeAndRealHaveDistinctSNIs
)
skip_re="^($(IFS='|'; printf '%s' "${skip_tests[*]}"))$"
go test -mod=readonly -p=2 -ldflags=-checklinkname=0 -skip "$skip_re" ./...
```

Причины исключений: `common/netns` — запрет запуска network namespace (`operation not permitted`); три `common/process` — недоступное сопоставление сокета и процесса в среде (`process not found`); три `common/tlsfragment` — отказ соединения с внешним `1.1.1.1:443`; шесть `common/tlsspoof` — запрет raw sockets (`operation not permitted`). Исключены ровно эти 13 верхнеуровневых тестов, а не пакеты целиком. Два пакета (`common/netns`, `common/tlsspoof`) после исключения не имели исполняемых тестов.

Production-прогон не сокращал теги из `release/DEFAULT_BUILD_TAGS_OTHERS`:

```sh
go test -mod=readonly -p=2 \
  -tags "$(cat release/DEFAULT_BUILD_TAGS_OTHERS)" \
  -ldflags "$(cat release/LDFLAGS)" \
  ./adapter/... ./dns/... ./route/... \
  ./protocol/chain ./protocol/group ./protocol/vless/... \
  ./protocol/masque ./protocol/tailscale ./protocol/wireguard \
  ./transport/wireguard ./transport/v2rayxhttp \
  ./common/ebpf ./protocol/ebpf ./include ./option ./lxd ./experimental/libbox
```

Production CLI собраны через `magicnet-android/scripts/build-sing-box.sh` из потребительского commit `0952cb01b001e41e1074c2e5f4a731a4ad42b222`; `MAGICNET_SINGBOX_SOURCE_DIR` указывал на чистый checkout проверенного ядра. Helper читает теги и ldflags из самого ядра, задаёт `-mod=readonly`, `-trimpath`, `-buildvcs=false`, `GOTOOLCHAIN=local`, `GOWORK=off`; для этих двух target — `CGO_ENABLED=0`.

```sh
# Из checkout magicnet-android; go в PATH — 1.26.10.
MAGICNET_SINGBOX_SOURCE_DIR=/path/to/clean/sing-box \
  bash scripts/build-sing-box.sh linux amd64 /tmp/sing-box-linux-amd64
MAGICNET_SINGBOX_SOURCE_DIR=/path/to/clean/sing-box \
  bash scripts/build-sing-box.sh android arm64 /tmp/sing-box-android-arm64
```

Сохранены только текстовые доказательства; бинарные файлы не включены. Android-сборка подтверждает компиляцию, а Linux DNS-проверки — работу loopback listeners. Ни одна из этих проверок не является device gate для Android TUN, AWG, pause/wake или RSS.
