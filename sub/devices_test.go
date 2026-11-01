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
	if err := ds.Delete([]uint{devs[0].Id}); err != nil {
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
