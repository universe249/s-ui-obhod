package model

// Device — одно устройство клиента (obhod). Устройство появляется, когда приложение скачивает подписку
// с заголовком x-hwid, и получает свои ключи: в ядре это отдельный пользователь «<клиент>~d<id>».
// Удалили устройство — его ключи сразу перестают работать, текущие соединения рвутся.
type Device struct {
	Id       uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	ClientId uint   `json:"clientId" gorm:"index;not null"`
	Hwid     string `json:"hwid" gorm:"index;not null"`
	// что прислало приложение: модель, система, User-Agent (для глаз администратора)
	Model     string `json:"model"`
	Os        string `json:"os"`
	UserAgent string `json:"userAgent"`
	// ключи устройства: uuid для vless/vmess/tuic, password для trojan/hysteria2/anytls/tuic/naive/hysteria
	Uuid     string `json:"uuid"`
	Password string `json:"password"`
	// прежние ключи после смены: работают до PrevUntil, чтобы приложение успело переключиться на новые
	PrevUuid     string `json:"-"`
	PrevPassword string `json:"-"`
	PrevUntil    int64  `json:"prevUntil" gorm:"default:0;not null"`
	CreatedAt    int64  `json:"createdAt" gorm:"default:0;not null"`
	LastSeen     int64  `json:"lastSeen" gorm:"default:0;not null"` // последнее скачивание подписки
	OnlineAt     int64  `json:"onlineAt" gorm:"default:0;not null"` // последний трафик
	RotatedAt    int64  `json:"rotatedAt" gorm:"default:0;not null"`
	// трафик устройства за всё время (байты)
	Up   int64 `json:"up" gorm:"default:0;not null"`
	Down int64 `json:"down" gorm:"default:0;not null"`
	// адрес, с которого устройство последний раз скачивало подписку
	LastIp string `json:"lastIp"`
}

// DeviceEvent — журнал: устройство добавлено/удалено, ключи сменены, попытка сверх лимита или без x-hwid.
type DeviceEvent struct {
	Id        uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	ClientId  uint   `json:"clientId" gorm:"index;not null"`
	DeviceId  uint   `json:"deviceId"`
	Kind      string `json:"kind" gorm:"index"` // added | deleted | rotated | limit | nohwid
	Hwid      string `json:"hwid"`
	Model     string `json:"model"`
	UserAgent string `json:"userAgent"`
	Ip        string `json:"ip"`
	Actor     string `json:"actor"` // кто удалил/сменил: логин панели, auto или токен API
	At        int64  `json:"at" gorm:"index;not null"`
}
