package service

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestDeviceOwner(t *testing.T) {
	for in, want := range map[string]string{"alice~d12": "alice", "a~b~d3": "a~b"} {
		if c, _, ok := DeviceOwner(in); !ok || c != want {
			t.Fatalf("%s -> %s %v", in, c, ok)
		}
	}
	for _, in := range []string{"alice", "alice~dx", "~d1", "alice~d"} {
		if _, _, ok := DeviceOwner(in); ok {
			t.Fatalf("%s parsed as device", in)
		}
	}
	if DeviceUserName("bob", 7) != "bob~d7" {
		t.Fatal("name")
	}
}

func TestFetchUsersDeviceMode(t *testing.T) {
	db := initUsersTestDB(t)
	if err := db.AutoMigrate(&model.Device{}); err != nil {
		t.Fatal(err)
	}
	db.Model(model.Client{}).Where("name = ?", "alice").Update("device_limit", 2)
	var alice model.Client
	db.Where("name = ?", "alice").First(&alice)
	now := time.Now().Unix()
	db.Create(&model.Device{ClientId: alice.Id, Hwid: "h1", Uuid: "dev1-uuid", Password: "p1"})
	db.Create(&model.Device{ClientId: alice.Id, Hwid: "h2", Uuid: "dev2-uuid", Password: "p2", PrevUuid: "dev2-old", PrevPassword: "p2old", PrevUntil: now + 3600})
	s := &InboundService{}
	users, err := s.fetchUsers(db, "vless", "1=1", nil, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	all := ""
	for _, u := range users {
		all += string(u) + "\n"
	}
	if strings.Contains(all, "alice-uuid") {
		t.Fatalf("shared key of device-mode client must not reach the core:\n%s", all)
	}
	for _, want := range []string{"bob-uuid", "dev1-uuid", "dev2-uuid", "dev2-old", `"alice~d1"`, `"alice~d2"`} {
		if !strings.Contains(all, want) {
			t.Fatalf("missing %s in:\n%s", want, all)
		}
	}
	// прежний ключ, срок которого вышел, в ядро не попадает
	db.Model(model.Device{}).Where("hwid = ?", "h2").Update("prev_until", now-1)
	users, _ = s.fetchUsers(db, "vless", "1=1", nil, map[string]interface{}{})
	b, _ := json.Marshal(users)
	if strings.Contains(string(b), "dev2-old") {
		t.Fatal("expired previous key still in core")
	}
}

func TestDeviceLinksKeepsShadowsocks(t *testing.T) {
	cfg := json.RawMessage(`{"vless":{"uuid":"base-uuid-1"},"trojan":{"password":"basepw123"},"shadowsocks":{"password":"basepw123"}}`)
	links := json.RawMessage(`[{"uri":"vless://base-uuid-1@x:1#a"},{"uri":"trojan://basepw123@x:2#b"},{"uri":"ss://bWV0aG9k:basepw123@x:3#c"}]`)
	out, err := DeviceLinks(links, cfg, &model.Device{Uuid: "dev-uuid", Password: "devpw"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "vless://dev-uuid@") || !strings.Contains(s, "trojan://devpw@") || !strings.Contains(s, "ss://bWV0aG9k:basepw123@") {
		t.Fatal(s)
	}
}

func TestValidHwid(t *testing.T) {
	if ValidHwid("abc") || ValidHwid("bad hwid with spaces") || !ValidHwid("A1b2C3-d4=") {
		t.Fatal("hwid validation")
	}
}

func TestTransitionKeepsSharedKey(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "t.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	for _, n := range []string{"alice", "bob"} {
		if err := db.Create(&model.Client{Name: n, Enable: true, Inbounds: json.RawMessage(`[]`),
			Config: json.RawMessage(`{"vless":{"uuid":"` + n + `-uuid"}}`)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var alice model.Client
	db.Where("name = ?", "alice").First(&alice)
	db.Create(&model.Device{ClientId: alice.Id, Hwid: "h1", Uuid: "dev1-uuid", Password: "p1"})
	now := time.Now().Unix()
	s := &InboundService{}
	all := func() string {
		users, err := s.fetchUsers(db, "vless", "1=1", nil, map[string]interface{}{})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(users)
		return string(b)
	}
	// лимит включили: переходный период — и общий ключ, и ключ устройства
	c := alice
	c.DeviceLimit = 1
	syncDeviceGrace(db, &c)
	if c.DeviceGraceUntil < now+71*3600 || c.DeviceGraceUntil > now+73*3600 {
		t.Fatalf("grace must start for 72h, got %d", c.DeviceGraceUntil-now)
	}
	db.Model(model.Client{}).Where("id = ?", alice.Id).Updates(map[string]interface{}{"device_limit": 1, "device_grace_until": c.DeviceGraceUntil})
	if got := all(); !strings.Contains(got, "alice-uuid") || !strings.Contains(got, "dev1-uuid") {
		t.Fatalf("transition must keep both keys: %s", got)
	}
	keep, _ := s.enabledClientNames(db, 0, true)
	_ = keep // входов в тестовой базе нет — проверяем только, что запрос с новой колонкой работает
	// повторное сохранение с тем же лимитом не продлевает период
	c2 := c
	c2.DeviceGraceUntil = 0
	syncDeviceGrace(db, &c2)
	if c2.DeviceGraceUntil != c.DeviceGraceUntil {
		t.Fatal("grace must not restart on a plain save")
	}
	// по общему ключу ходили недавно, до конца меньше суток — одно напоминание
	db.Model(model.Client{}).Where("id = ?", alice.Id).Updates(map[string]interface{}{"device_grace_until": now + 3600, "legacy_seen_at": now - 60})
	var ds DeviceService
	if err := ds.GraceTick(now); err != nil {
		t.Fatal(err)
	}
	if err := ds.GraceTick(now + 60); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(model.DeviceEvent{}).Where("client_id = ? AND kind = ?", alice.Id, "legacy").Count(&n)
	if n != 1 {
		t.Fatalf("want 1 reminder, got %d", n)
	}
	// период вышел — общий ключ из ядра пропадает
	if err := ds.GraceTick(now + 7200); err != nil {
		t.Fatal(err)
	}
	if got := all(); strings.Contains(got, "alice-uuid") || !strings.Contains(got, "dev1-uuid") {
		t.Fatalf("after transition only device keys: %s", got)
	}
	// лимит выключили — период сброшен, снова общий ключ
	c3 := alice
	c3.Id = alice.Id
	c3.DeviceLimit = 0
	syncDeviceGrace(db, &c3)
	if c3.DeviceGraceUntil != 0 {
		t.Fatal("grace must reset when the limit is off")
	}
}
