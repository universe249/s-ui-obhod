package service

// obhod: устройства клиента. Подписка, скачанная приложением с заголовком x-hwid, регистрирует устройство, и оно
// получает свои ключи — в ядре это отдельный пользователь «<клиент>~d<id>». Общий ключ клиента в режиме устройств
// (DeviceLimit > 0) в ядро не попадает, поэтому скопированная из приложения ссылка — это ключ конкретного устройства:
// удалили устройство или сменили ему ключ — ссылка умерла.

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"

	"github.com/gofrs/uuid/v5"
	"gorm.io/gorm"
)

const deviceSep = "~d"

// ErrDeviceLimit — у клиента уже максимум устройств.
var ErrDeviceLimit = errors.New("device limit reached")

// ErrNoHwid — режим устройств включён, а приложение не прислало x-hwid.
var ErrNoHwid = errors.New("hwid required")

// DeviceInfo — что приложение сообщает о себе в заголовках подписки.
type DeviceInfo struct {
	Hwid, Model, Os, UserAgent string
}

// DeviceUserName — имя пользователя устройства в ядре.
func DeviceUserName(client string, id uint) string {
	return client + deviceSep + strconv.FormatUint(uint64(id), 10)
}

// DeviceOwner разбирает имя пользователя ядра: клиент и номер устройства (ok=false — обычный пользователь).
func DeviceOwner(user string) (client string, id uint, ok bool) {
	i := strings.LastIndex(user, deviceSep)
	if i <= 0 {
		return user, 0, false
	}
	n, err := strconv.ParseUint(user[i+len(deviceSep):], 10, 32)
	if err != nil {
		return user, 0, false
	}
	return user[:i], uint(n), true
}

// perDeviceProto — у каких протоколов ключ выдаётся на устройство. У Shadowsocks 2022, ShadowTLS и Snell ключ —
// это ключ шифрования строгого формата, их оставляем общими.
func perDeviceProto(t string) bool {
	switch t {
	case "vless", "vmess", "trojan", "anytls", "hysteria", "hysteria2", "tuic", "naive":
		return true
	}
	return false
}

// ValidHwid — тот же формат, что у Remnawave: 6–64 символа из латиницы, цифр, «=», «-», «_», «.».
func ValidHwid(h string) bool {
	if len(h) < 6 || len(h) > 64 {
		return false
	}
	for _, c := range h {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '=' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func newDevicePassword() string {
	const abc = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = abc[int(b[i])%len(abc)]
	}
	return string(b)
}

func newDeviceUuid() string {
	u, _ := uuid.NewV4()
	return u.String()
}

// deviceUser превращает настройки пользователя клиента для протокола в пользователя устройства (свои ключи и имя).
// prev=true — с прежними ключами (на время после смены).
func deviceUser(base map[string]interface{}, client string, d *model.Device, prev bool) map[string]interface{} {
	u := make(map[string]interface{}, len(base))
	for k, v := range base {
		u[k] = v
	}
	id, pw := d.Uuid, d.Password
	if prev {
		id, pw = d.PrevUuid, d.PrevPassword
	}
	name := DeviceUserName(client, d.Id)
	if _, ok := u["username"]; ok {
		u["username"] = name
	} else {
		u["name"] = name
	}
	if _, ok := u["uuid"]; ok {
		u["uuid"] = id
	}
	if _, ok := u["password"]; ok {
		u["password"] = pw
	}
	if _, ok := u["auth_str"]; ok {
		u["auth_str"] = pw
	}
	return u
}

// DeviceConfig — настройки клиента (поле config) с ключами устройства вместо общих: из них строится подписка.
func DeviceConfig(cfg json.RawMessage, client string, d *model.Device) (json.RawMessage, error) {
	var all map[string]map[string]interface{}
	if err := json.Unmarshal(cfg, &all); err != nil {
		return nil, err
	}
	for t, u := range all {
		if perDeviceProto(t) {
			all[t] = deviceUser(u, client, d, false)
		}
	}
	return json.Marshal(all)
}

// DeviceLinks заменяет в готовых ссылках клиента общие ключи на ключи устройства.
func DeviceLinks(links json.RawMessage, cfg json.RawMessage, d *model.Device) (json.RawMessage, error) {
	var all map[string]map[string]interface{}
	if err := json.Unmarshal(cfg, &all); err != nil {
		return nil, err
	}
	pairs := map[string]string{}
	for t, u := range all {
		if !perDeviceProto(t) {
			continue
		}
		if v, _ := u["uuid"].(string); v != "" {
			pairs[v] = d.Uuid
		}
		for _, k := range []string{"password", "auth_str"} {
			if v, _ := u[k].(string); v != "" {
				pairs[v] = d.Password
			}
		}
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(links, &list); err != nil {
		return nil, err
	}
	for _, l := range list {
		uri, _ := l["uri"].(string)
		if uri == "" {
			continue
		}
		// ss:// с общим ключом оставляем как есть; vless://uuid@, trojan://pass@, hysteria2://pass@, tuic://uuid:pass@
		if strings.HasPrefix(uri, "ss://") {
			continue
		}
		for from, to := range pairs {
			if len(from) >= 6 {
				uri = strings.ReplaceAll(uri, from, to)
			}
		}
		l["uri"] = uri
	}
	return json.Marshal(list)
}

type DeviceService struct {
	SettingService
}

// List — устройства клиента.
func (s *DeviceService) List(clientId uint) ([]model.Device, error) {
	var out []model.Device
	err := database.GetDB().Where("client_id = ?", clientId).Order("id").Find(&out).Error
	return out, err
}

// Register находит или заводит устройство для скачивания подписки. Если пора — меняет ему ключи
// (прежние работают ещё deviceGraceHours часов). changed=true — набор пользователей ядра изменился.
func (s *DeviceService) Register(client *model.Client, info DeviceInfo) (dev *model.Device, changed bool, err error) {
	if !ValidHwid(info.Hwid) {
		return nil, false, ErrNoHwid
	}
	now := time.Now().Unix()
	db := database.GetDB()
	err = db.Transaction(func(tx *gorm.DB) error {
		var d model.Device
		e := tx.Where("client_id = ? AND hwid = ?", client.Id, info.Hwid).First(&d).Error
		if database.IsNotFound(e) {
			var n int64
			if e := tx.Model(model.Device{}).Where("client_id = ?", client.Id).Count(&n).Error; e != nil {
				return e
			}
			if client.DeviceLimit > 0 && int(n) >= client.DeviceLimit {
				return ErrDeviceLimit
			}
			d = model.Device{ClientId: client.Id, Hwid: info.Hwid, Uuid: newDeviceUuid(), Password: newDevicePassword(),
				CreatedAt: now, RotatedAt: now}
			changed = true
		} else if e != nil {
			return e
		}
		// не затираем то, что приложение сообщило раньше, пустым значением
		if v := clip(info.Model, 80); v != "" {
			d.Model = v
		}
		if v := clip(info.Os, 40); v != "" {
			d.Os = v
		}
		if v := clip(info.UserAgent, 120); v != "" {
			d.UserAgent = v
		}
		d.LastSeen = now
		if !changed {
			rot, _ := s.GetDeviceRotateHours()
			if rot > 0 && now-d.RotatedAt >= int64(rot)*3600 {
				grace, _ := s.GetDeviceGraceHours()
				d.PrevUuid, d.PrevPassword, d.PrevUntil = d.Uuid, d.Password, now+int64(grace)*3600
				d.Uuid, d.Password, d.RotatedAt = newDeviceUuid(), newDevicePassword(), now
				changed = true
			}
		}
		if e := tx.Save(&d).Error; e != nil {
			return e
		}
		dev = &d
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return dev, changed, nil
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ClientInbounds — входящие клиента.
func clientInbounds(tx *gorm.DB, clientId uint) ([]uint, error) {
	var c model.Client
	if err := tx.Where("id = ?", clientId).First(&c).Error; err != nil {
		return nil, err
	}
	var ids []uint
	if err := json.Unmarshal(c.Inbounds, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// ApplyClient обновляет пользователей ядра на входящих клиента (без перезапуска ядра).
func ApplyClient(clientId uint) error {
	db := database.GetDB()
	ids, err := clientInbounds(db, clientId)
	if err != nil || len(ids) == 0 {
		return err
	}
	var in InboundService
	return in.UpdateInboundsUsers(db, ids)
}

// Delete удаляет устройства: их ключи перестают работать, соединения рвутся сразу.
func (s *DeviceService) Delete(ids []uint) error {
	db := database.GetDB()
	var devs []model.Device
	if err := db.Where("id IN ?", ids).Find(&devs).Error; err != nil {
		return err
	}
	if err := db.Where("id IN ?", ids).Delete(model.Device{}).Error; err != nil {
		return err
	}
	clients := map[uint]bool{}
	for _, d := range devs {
		clients[d.ClientId] = true
	}
	for id := range clients {
		if err := ApplyClient(id); err != nil {
			return err
		}
	}
	var st StatsService
	for _, d := range devs {
		var c model.Client
		if db.Where("id = ?", d.ClientId).First(&c).Error == nil {
			_ = st.CloseUserSessions(DeviceUserName(c.Name, d.Id))
		}
	}
	return nil
}

// Rotate немедленно меняет ключи устройств клиента (clientId=0 — всех). Прежние ключи действуют grace часов;
// при grace=0 они перестают работать сразу, и соединения со старыми ключами рвутся.
func (s *DeviceService) Rotate(clientId uint, graceHours int) (int, error) {
	db := database.GetDB()
	q := db.Model(model.Device{})
	if clientId > 0 {
		q = q.Where("client_id = ?", clientId)
	}
	var devs []model.Device
	if err := q.Find(&devs).Error; err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	clients := map[uint]bool{}
	for i := range devs {
		d := &devs[i]
		if graceHours > 0 {
			d.PrevUuid, d.PrevPassword, d.PrevUntil = d.Uuid, d.Password, now+int64(graceHours)*3600
		} else {
			d.PrevUuid, d.PrevPassword, d.PrevUntil = "", "", 0
		}
		d.Uuid, d.Password, d.RotatedAt = newDeviceUuid(), newDevicePassword(), now
		if err := db.Save(d).Error; err != nil {
			return 0, err
		}
		clients[d.ClientId] = true
	}
	for id := range clients {
		if err := ApplyClient(id); err != nil {
			return 0, err
		}
		if graceHours <= 0 {
			var c model.Client
			if db.Where("id = ?", id).First(&c).Error == nil {
				var st StatsService
				for _, d := range devs {
					if d.ClientId == id {
						_ = st.CloseUserSessions(DeviceUserName(c.Name, d.Id))
					}
				}
			}
		}
	}
	return len(devs), nil
}

// DropExpiredPrev убирает прежние ключи, срок которых вышел (раз в минуту из cron).
func (s *DeviceService) DropExpiredPrev() error {
	db := database.GetDB()
	now := time.Now().Unix()
	var devs []model.Device
	if err := db.Where("prev_until > 0 AND prev_until <= ?", now).Find(&devs).Error; err != nil {
		return err
	}
	clients := map[uint]bool{}
	for _, d := range devs {
		if err := db.Model(model.Device{}).Where("id = ?", d.Id).
			Updates(map[string]interface{}{"prev_uuid": "", "prev_password": "", "prev_until": 0}).Error; err != nil {
			return err
		}
		clients[d.ClientId] = true
	}
	for id := range clients {
		if err := ApplyClient(id); err != nil {
			logger.Warning("devices: update inbound users: ", err)
		}
	}
	return nil
}

// SetLimit задаёт клиенту лимит устройств (0 — режим выключен, общий ключ).
func (s *DeviceService) SetLimit(clientId uint, limit int) error {
	if limit < 0 || limit > 100 {
		return fmt.Errorf("limit must be 0..100")
	}
	db := database.GetDB()
	if err := db.Model(model.Client{}).Where("id = ?", clientId).Update("device_limit", limit).Error; err != nil {
		return err
	}
	return ApplyClient(clientId)
}

// ---- помощники для ClientService ----

func jsonHasKey(data json.RawMessage, key string) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(data, &m) != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func rawHasKey(data json.RawMessage, idx int, key string) bool {
	var arr []map[string]json.RawMessage
	if json.Unmarshal(data, &arr) != nil || idx < 0 || idx >= len(arr) {
		return false
	}
	_, ok := arr[idx][key]
	return ok
}

// keepDeviceLimit — форма старого интерфейса не знает о лимите устройств; не сбрасываем его.
func keepDeviceLimit(tx *gorm.DB, client *model.Client) {
	if client.Id == 0 {
		return
	}
	var old model.Client
	if tx.Model(model.Client{}).Select("device_limit").Where("id = ?", client.Id).First(&old).Error == nil {
		client.DeviceLimit = old.DeviceLimit
	}
}
