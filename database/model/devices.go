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
}
