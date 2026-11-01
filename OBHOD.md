# s-ui (obhod)

Форк [alireza0/s-ui](https://github.com/alireza0/s-ui) v1.6.7 с режимом устройств. Интерфейс — alireza0/s-ui-frontend@f473aeb
с правками (вкладка «Устройства», поле «Лимит устройств»), лежит в `frontend/` (без подмодуля).

- `deviceLimit` у клиента: 0 — как в оригинале (общий ключ); N > 0 — подписку получают только приложения,
  присылающие `x-hwid` (Happ, v2RayTun и др.), не больше N устройств.
- У каждого устройства свои ключи (пользователь ядра `<клиент>~d<id>`); общий ключ клиента в ядро не попадает.
  Shadowsocks 2022, ShadowTLS и Snell остаются общими (ключ строгого формата).
- Удаление устройства сразу отключает его ключи и рвёт текущие соединения.
- Смена ключей: по кнопке/API (`deviceRotate`) или автоматически при скачивании подписки раз в
  `deviceRotateHours` часов; прежние ключи работают ещё `deviceGraceHours` часов.
- Лимит превышен или нет `x-hwid` — вместо серверов строка-подсказка и заголовки
  `x-hwid-max-devices-reached`, `x-hwid-limit`, `x-hwid-not-supported` (как у Remnawave).
- Трафик устройств идёт в счёт клиента.

API (`/api` и `/apiv2`): `GET devices?c=<id>`, `POST deviceDel ids=1,2`, `POST deviceRotate c=<id|0> grace=<часы>`,
`POST deviceLimit c=<id> limit=<0..100>`.

Сборка: `(cd frontend && npm ci && npm run build) && cp -R frontend/dist/* web/html/`, затем
`. ./build-tags.sh; go build -tags "$(tags_for test),netgo,osusergo" -ldflags "-s -w $(ldflags_for test) -linkmode external -extldflags -static" -o s-ui .`
