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
	Hwid, Model, Os, UserAgent, Ip string
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
		if info.Ip != "" {
			d.LastIp = clip(info.Ip, 45)
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
	if errors.Is(err, ErrDeviceLimit) {
		LogDeviceEvent(client, nil, "limit", info, "")
	}
	if err != nil {
		return nil, false, err
	}
	if changed {
		kind := "rotated"
		if dev.CreatedAt == now {
			kind = "added"
		}
		LogDeviceEvent(client, dev, kind, info, "auto")
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
func (s *DeviceService) Delete(ids []uint, actor string) error {
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
	for i := range devs {
		d := &devs[i]
		var c model.Client
		if db.Where("id = ?", d.ClientId).First(&c).Error == nil {
			_ = st.CloseUserSessions(DeviceUserName(c.Name, d.Id))
			LogDeviceEvent(&c, d, "deleted", DeviceInfo{}, actor)
		}
	}
	return nil
}

// Rotate немедленно меняет ключи устройств клиента (clientId=0 — всех). Прежние ключи действуют grace часов;
// при grace=0 они перестают работать сразу, и соединения со старыми ключами рвутся.
func (s *DeviceService) Rotate(clientId uint, graceHours int, actor string) (int, error) {
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
		var cl model.Client
		if db.Where("id = ?", id).First(&cl).Error == nil {
			for i := range devs {
				if devs[i].ClientId == id {
					LogDeviceEvent(&cl, &devs[i], "rotated", DeviceInfo{}, actor)
				}
			}
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

// keepSubToken — токен подписки ведёт панель: при правке сохраняется прежний, новому клиенту выдаётся свой.
func keepSubToken(tx *gorm.DB, client *model.Client) {
	if client.Id != 0 {
		var old model.Client
		if tx.Model(model.Client{}).Select("sub_token").Where("id = ?", client.Id).First(&old).Error == nil && old.SubToken != "" {
			client.SubToken = old.SubToken
			return
		}
	}
	if client.SubToken == "" {
		client.SubToken = NewSubToken()
	}
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

// ---- журнал, сводка и уведомления ----

// LogDeviceEvent пишет событие в журнал и отправляет уведомление (если задан адрес). Повторы «limit»/«nohwid»
// от одного устройства чаще раза в 10 минут не пишем: приложение может обновлять подписку в цикле.
func LogDeviceEvent(client *model.Client, dev *model.Device, kind string, info DeviceInfo, actor string) {
	db := database.GetDB()
	now := time.Now().Unix()
	e := model.DeviceEvent{ClientId: client.Id, Kind: kind, Hwid: clip(info.Hwid, 64), Model: clip(info.Model, 80),
		UserAgent: clip(info.UserAgent, 120), Ip: clip(info.Ip, 45), Actor: clip(actor, 40), At: now}
	if dev != nil {
		e.DeviceId = dev.Id
		if e.Hwid == "" {
			e.Hwid = dev.Hwid
		}
		if e.Model == "" {
			e.Model = dev.Model
		}
		if e.UserAgent == "" {
			e.UserAgent = dev.UserAgent
		}
	}
	if kind == "limit" || kind == "nohwid" {
		var n int64
		db.Model(model.DeviceEvent{}).Where("client_id = ? AND kind = ? AND hwid = ? AND at > ?", client.Id, kind, e.Hwid, now-600).Count(&n)
		if n > 0 {
			return
		}
	}
	if err := db.Create(&e).Error; err != nil {
		logger.Warning("devices: event: ", err)
		return
	}
	go sendDeviceWebhook(client, dev, &e)
}

// Events — последние события устройств клиента (clientId=0 — всех).
func (s *DeviceService) Events(clientId uint, limit int) ([]model.DeviceEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	q := database.GetDB().Model(model.DeviceEvent{})
	if clientId > 0 {
		q = q.Where("client_id = ?", clientId)
	}
	var out []model.DeviceEvent
	err := q.Order("id desc").Limit(limit).Find(&out).Error
	return out, err
}

// DeviceView — устройство с живыми данными ядра: сколько сейчас соединений и с каких адресов.
type DeviceView struct {
	model.Device
	Sessions int      `json:"sessions"`
	Ips      []string `json:"ips"`
}

// Views — устройства клиента с текущими соединениями.
func (s *DeviceService) Views(client *model.Client) ([]DeviceView, error) {
	devs, err := s.List(client.Id)
	if err != nil {
		return nil, err
	}
	byUser := map[string][]string{}
	var st StatsService
	if sessions, err := st.GetSessions("", ""); err == nil {
		for _, ss := range sessions {
			if _, _, ok := DeviceOwner(ss.User); ok {
				byUser[ss.User] = append(byUser[ss.User], hostOnly(ss.Source))
			}
		}
	}
	out := make([]DeviceView, 0, len(devs))
	for _, d := range devs {
		srcs := byUser[DeviceUserName(client.Name, d.Id)]
		v := DeviceView{Device: d, Sessions: len(srcs), Ips: uniq(srcs, 5)}
		out = append(out, v)
	}
	return out, nil
}

func hostOnly(addr string) string {
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return strings.Trim(addr[:i], "[]")
	}
	return addr
}

func uniq(in []string, max int) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
			if len(out) >= max {
				break
			}
		}
	}
	return out
}

// ClientDeviceSummary — для списка клиентов: сколько устройств и сколько попыток сверх лимита за сутки.
type ClientDeviceSummary struct {
	Devices      int   `json:"devices"`
	Limit        int   `json:"limit"`
	LimitHits24h int   `json:"limitHits24h"`
	LastAdded    int64 `json:"lastAdded"`
}

func (s *DeviceService) Summary() (map[uint]ClientDeviceSummary, error) {
	db := database.GetDB()
	out := map[uint]ClientDeviceSummary{}
	var clients []model.Client
	if err := db.Model(model.Client{}).Select("id", "device_limit").Where("device_limit > 0").Find(&clients).Error; err != nil {
		return nil, err
	}
	for _, c := range clients {
		out[c.Id] = ClientDeviceSummary{Limit: c.DeviceLimit}
	}
	var counts []struct {
		ClientId  uint
		N         int
		LastAdded int64
	}
	db.Model(model.Device{}).Select("client_id, count(*) as n, max(created_at) as last_added").Group("client_id").Scan(&counts)
	for _, r := range counts {
		v := out[r.ClientId]
		v.Devices, v.LastAdded = r.N, r.LastAdded
		out[r.ClientId] = v
	}
	var hits []struct {
		ClientId uint
		N        int
	}
	db.Model(model.DeviceEvent{}).Select("client_id, count(*) as n").Where("kind = ? AND at > ?", "limit", time.Now().Unix()-86400).Group("client_id").Scan(&hits)
	for _, r := range hits {
		v := out[r.ClientId]
		v.LimitHits24h = r.N
		out[r.ClientId] = v
	}
	return out, nil
}

// ClientInfo — всё о клиенте одним ответом (для бота): учётная запись, устройства, последние события.
type ClientInfo struct {
	Id           uint                `json:"id"`
	Name         string              `json:"name"`
	SubToken     string              `json:"subToken"`
	Enable       bool                `json:"enable"`
	Volume       int64               `json:"volume"`
	Used         int64               `json:"used"`
	Expiry       int64               `json:"expiry"`
	OnlineAt     int64               `json:"onlineAt"`
	DeviceLimit  int                 `json:"deviceLimit"`
	Devices      []DeviceView        `json:"devices"`
	Events       []model.DeviceEvent `json:"events"`
	LimitHits24h int                 `json:"limitHits24h"`
}

func (s *DeviceService) ClientInfo(id uint, name string) (*ClientInfo, error) {
	db := database.GetDB()
	var c model.Client
	q := db.Model(model.Client{})
	if id > 0 {
		q = q.Where("id = ?", id)
	} else {
		q = q.Where("name = ?", name)
	}
	if err := q.First(&c).Error; err != nil {
		return nil, err
	}
	views, err := s.Views(&c)
	if err != nil {
		return nil, err
	}
	evs, _ := s.Events(c.Id, 20)
	var hits int64
	db.Model(model.DeviceEvent{}).Where("client_id = ? AND kind = ? AND at > ?", c.Id, "limit", time.Now().Unix()-86400).Count(&hits)
	return &ClientInfo{Id: c.Id, Name: c.Name, SubToken: c.SubToken, Enable: c.Enable, Volume: c.Volume, Used: c.Up + c.Down, Expiry: c.Expiry,
		OnlineAt: c.OnlineAt, DeviceLimit: c.DeviceLimit, Devices: views, Events: evs, LimitHits24h: int(hits)}, nil
}

// PruneEvents удаляет записи журнала старше 90 дней.
func (s *DeviceService) PruneEvents() {
	database.GetDB().Where("at < ?", time.Now().Unix()-90*86400).Delete(model.DeviceEvent{})
}
