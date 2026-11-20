# PLAN: 086 — SOCKET_DIAG_REQUEST_STACK

1. `packSocketDiagRequest` возвращает `[sizeOfSocketDiagRequest]byte`;
   zero initialization заменяет `make` без правки encoding-полей.
2. Exact-query и dump передают `request[:]` синхронным syscall-обёрткам.
   Пул, timeout, retry и разбор ответа не изменяются.
3. Приспособить существующий controlled-datagram fixture к private типу
   возврата; добавить encoding-бенчмарк четырёх family/protocol комбинаций.
4. Проверить `-race`, vet, stack escape analysis и обе сборки.

Зона возможного merge-конфликта — только helper signature и два callsite
в `common/process/socket_diag_linux.go`, под lx-маркерами.
