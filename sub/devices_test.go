package sub

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service"

	"github.com/gin-gonic/gin"
)

const baseUUID = "11111111-2222-4333-8444-555555555555"
const basePass = "BasePassword123"

func devTestSetup(t *testing.T, limit int) (*gin.Engine, *model.Client) {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	links := []map[string]string{
		{"type": "local", "remark": "v", "uri": "vless://" + baseUUID + "@a.example:443?type=tcp&security=reality#v"},
		{"type": "local", "remark": "h", "uri": "hysteria2://" + basePass + "@a.example:443?sni=a.example#h"},
		{"type": "local", "remark": "s", "uri": "ss://Y2hhY2hhMjA6" + basePass + "@a.example:1#s"},
	}
	lb, _ := json.Marshal(links)
	c := &model.Client{Name: "alice", Enable: true, DeviceLimit: limit, Inbounds: json.RawMessage(`[]`), Links: lb,
		Config: json.RawMessage(`{"vless":{"name":"alice","uuid":"` + baseUUID + `"},"hysteria2":{"name":"alice","password":"` + basePass + `"},"shadowsocks":{"name":"alice","password":"` + basePass + `"}}`)}
	if err := database.GetDB().Create(c).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	NewSubHandler(r.Group("/sub"))
	return r, c
}

func fetch(t *testing.T, r *gin.Engine, hwid string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/sub/alice", nil)
	if hwid != "" {
		req.Header.Set("X-Hwid", hwid)
		req.Header.Set("X-Device-Model", "iPhone 15")
		req.Header.Set("User-Agent", "Happ/3.0")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	body, _ := base64.StdEncoding.DecodeString(w.Body.String())
	return w, string(body)
}

func TestSubWithoutDeviceModeUnchanged(t *testing.T) {
	r, _ := devTestSetup(t, 0)
	w, body := fetch(t, r, "")
	if w.Code != 200 || !strings.Contains(body, baseUUID) || !strings.Contains(body, basePass) {
		t.Fatalf("%d %q", w.Code, body)
	}
}

func TestSubDeviceMode(t *testing.T) {
	r, c := devTestSetup(t, 2)
	// без x-hwid — подсказка вместо серверов
	w, body := fetch(t, r, "")
	if w.Header().Get("x-hwid-not-supported") != "true" || strings.Contains(body, baseUUID) {
		t.Fatalf("no hwid: %v %q", w.Header(), body)
	}
	// первое устройство: свои ключи вместо общих, ss остаётся общим
	_, b1 := fetch(t, r, "hwid-aaaaaa")
	if strings.Contains(b1, baseUUID) || strings.Contains(b1, "hysteria2://"+basePass) || !strings.Contains(b1, "ss://Y2hhY2hhMjA6"+basePass) {
		t.Fatalf("device links still have shared keys: %q", b1)
	}
	reqNoInfo := httptest.NewRequest(http.MethodGet, "/sub/alice", nil)
	reqNoInfo.Header.Set("X-Hwid", "hwid-aaaaaa")
	r.ServeHTTP(httptest.NewRecorder(), reqNoInfo) // без модели — название устройства не должно пропасть
	_, b1again := fetch(t, r, "hwid-aaaaaa")
	if b1again != b1 {
		t.Fatal("same device must get the same keys")
	}
	_, b2 := fetch(t, r, "hwid-bbbbbb")
	if b2 == b1 {
		t.Fatal("second device must get its own keys")
	}
	w3, _ := fetch(t, r, "hwid-cccccc")
	if w3.Header().Get("x-hwid-max-devices-reached") != "true" {
		t.Fatalf("limit not enforced: %v", w3.Header())
	}
	var ds service.DeviceService
	devs, _ := ds.List(c.Id)
	if len(devs) != 2 || devs[0].Model != "iPhone 15" || devs[0].UserAgent != "Happ/3.0" {
		t.Fatalf("devices: %+v", devs)
	}
	// удалили первое — место освободилось
	if err := ds.Delete([]uint{devs[0].Id}, "test"); err != nil {
		t.Fatal(err)
	}
	if w, _ := fetch(t, r, "hwid-cccccc"); w.Header().Get("x-hwid-max-devices-reached") != "" {
		t.Fatal("slot not freed")
	}
	// повторно пришедшее удалённое устройство — уже новое, и в лимит не влезает
	if w, _ := fetch(t, r, "hwid-aaaaaa"); w.Header().Get("x-hwid-max-devices-reached") != "true" {
		t.Fatal("deleted device came back")
	}
}

func TestSubDeviceRotation(t *testing.T) {
	r, c := devTestSetup(t, 1)
	_, b1 := fetch(t, r, "hwid-aaaaaa")
	db := database.GetDB()
	db.Create(&model.Setting{Key: "deviceRotateHours", Value: "24"})
	db.Model(model.Device{}).Where("client_id = ?", c.Id).Update("rotated_at", time.Now().Add(-25*time.Hour).Unix())
	_, b2 := fetch(t, r, "hwid-aaaaaa")
	if b2 == b1 {
		t.Fatal("keys not rotated")
	}
	var d model.Device
	db.Where("client_id = ?", c.Id).First(&d)
	if d.PrevUuid == "" || d.PrevUntil <= time.Now().Unix() || !strings.Contains(b1, d.PrevUuid) {
		t.Fatalf("previous keys not kept for grace: %+v", d)
	}
	// срок вышел — прежние ключи убираются
	db.Model(model.Device{}).Where("id = ?", d.Id).Update("prev_until", time.Now().Unix()-1)
	var ds service.DeviceService
	if err := ds.DropExpiredPrev(); err != nil {
		t.Fatal(err)
	}
	db.Where("id = ?", d.Id).First(&d)
	if d.PrevUuid != "" || d.PrevUntil != 0 {
		t.Fatal("prev keys not dropped")
	}
}

func TestDeviceEventsSummaryAndWebhook(t *testing.T) {
	got := make(chan map[string]interface{}, 10)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Obhod-Secret") != "s3cret" {
			w.WriteHeader(403)
			return
		}
		var p map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&p)
		got <- p
	}))
	defer hook.Close()
	r, c := devTestSetup(t, 1)
	db := database.GetDB()
	db.Create(&model.Setting{Key: "deviceWebhook", Value: hook.URL})
	db.Create(&model.Setting{Key: "deviceWebhookSecret", Value: "s3cret"})

	fetch(t, r, "hwid-aaaaaa")
	fetch(t, r, "hwid-bbbbbb") // сверх лимита
	fetch(t, r, "hwid-bbbbbb") // повтор в течение 10 минут — в журнал не пишется
	kinds := map[string]int{}
	for i := 0; i < 2; i++ {
		select {
		case p := <-got:
			kinds[p["kind"].(string)]++
			if cl := p["client"].(map[string]interface{}); cl["name"] != "alice" {
				t.Fatalf("payload %v", p)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("webhook not delivered, got %v", kinds)
		}
	}
	if kinds["added"] != 1 || kinds["limit"] != 1 {
		t.Fatalf("webhook kinds %v", kinds)
	}
	var ds service.DeviceService
	evs, _ := ds.Events(c.Id, 10)
	if len(evs) != 2 || evs[0].Kind != "limit" || evs[1].Kind != "added" || evs[1].Model != "iPhone 15" {
		t.Fatalf("events %+v", evs)
	}
	sum, _ := ds.Summary()
	if s := sum[c.Id]; s.Devices != 1 || s.Limit != 1 || s.LimitHits24h != 1 {
		t.Fatalf("summary %+v", sum)
	}
	info, err := ds.ClientInfo(0, "alice")
	if err != nil || len(info.Devices) != 1 || info.LimitHits24h != 1 || len(info.Events) != 2 || info.Devices[0].LastIp == "" {
		t.Fatalf("clientInfo %+v %v", info, err)
	}
	if err := ds.Delete([]uint{info.Devices[0].Id}, "admin"); err != nil {
		t.Fatal(err)
	}
	evs, _ = ds.Events(c.Id, 1)
	if evs[0].Kind != "deleted" || evs[0].Actor != "admin" {
		t.Fatalf("delete event %+v", evs[0])
	}
}
