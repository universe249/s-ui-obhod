package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

// obhod: вход с кодом из бота — после пароля код уходит в бот, без кода и с неверным кодом сессии нет.
func TestLoginWithTelegramCode(t *testing.T) {
	codes := make(chan map[string]interface{}, 4)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&p)
		codes <- p
	}))
	defer hook.Close()
	srv := httptest.NewServer(loginEngine(t))
	defer srv.Close()
	db := database.GetDB()
	db.Create(&model.Setting{Key: "login2fa", Value: "true"})
	db.Create(&model.Setting{Key: "deviceWebhook", Value: hook.URL})

	login := func(extra url.Values) (*http.Response, map[string]interface{}) {
		form := url.Values{"user": {"admin"}, "pass": {"admin"}}
		for k, v := range extra {
			form[k] = v
		}
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", srv.URL)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&m)
		resp.Body.Close()
		return resp, m
	}
	hasSession := func(resp *http.Response) bool {
		for _, c := range resp.Cookies() {
			if c.Name == "s-ui" && c.Value != "" && c.MaxAge >= 0 {
				return true
			}
		}
		return false
	}

	resp, m := login(nil)
	if obj, _ := m["obj"].(map[string]interface{}); obj["needCode"] != true || hasSession(resp) {
		t.Fatalf("first step must ask for a code without a session: %v", m)
	}
	var code string
	for len(codes) > 0 || code == "" {
		p := <-codes
		if p["kind"] == "login_code" {
			code, _ = p["code"].(string)
		}
	}
	if len(code) != 6 {
		t.Fatalf("code %q", code)
	}
	if resp, m := login(url.Values{"code": {"000000x"}}); m["success"] == true || hasSession(resp) {
		t.Fatalf("wrong code accepted: %v", m)
	}
	if resp, m := login(url.Values{"code": {code}}); m["success"] != true || !hasSession(resp) {
		t.Fatalf("right code rejected: %v", m)
	}
	if _, m := login(url.Values{"code": {code}}); m["success"] == true {
		t.Fatal("code must be single-use")
	}

	// бот недоступен — войти нельзя, и сообщение подсказывает, как выключить проверку
	db.Model(model.Setting{}).Where("key = ?", "deviceWebhook").Update("value", "http://127.0.0.1:1/hook")
	if _, m := login(nil); m["success"] == true || !strings.Contains(m["msg"].(string), "-login2fa false") {
		t.Fatalf("undelivered code: %v", m)
	}
}
