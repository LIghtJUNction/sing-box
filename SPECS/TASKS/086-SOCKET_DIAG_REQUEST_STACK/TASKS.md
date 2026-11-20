# TASKS: 086 — SOCKET_DIAG_REQUEST_STACK

- [x] Измерить baseline encoder на текущем `8d51dc99`.
- [x] Убрать временное выделение request, оставив encoding прежним.
- [x] Проверить четыре комбинации family/protocol и compiler escape.
- [x] Сравнить существующий controlled-datagram query pool.
- [x] Прогнать `common/process` с race detector и vet.
- [x] Завершить обычную и Android ARM64 release-сборки.
- [ ] Отдельная полевая оценка RSS/CPU/батареи; сейчас не заявляется.
