package service

// obhod: уведомления о событиях устройств. Если в настройках задан deviceWebhook, панель отправляет туда JSON
// о каждом событии (добавлено, удалено, ключи сменены, попытка сверх лимита, приложение без x-hwid).
// Так бот узнаёт о новом устройстве сразу и может написать человеку, не опрашивая панель.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
)

type deviceHookPayload struct {
	Kind    string           `json:"kind"`
	At      int64            `json:"at"`
	Actor   string           `json:"actor,omitempty"`
	Ip      string           `json:"ip,omitempty"`
	Client  deviceHookClient `json:"client"`
	Device  *deviceHookDev   `json:"device,omitempty"`
	Devices int64            `json:"devices"`
}

type deviceHookClient struct {
	Id          uint   `json:"id"`
	Name        string `json:"name"`
	DeviceLimit int    `json:"deviceLimit"`
}

type deviceHookDev struct {
	Id        uint   `json:"id,omitempty"`
	Hwid      string `json:"hwid"`
	Model     string `json:"model,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
}

var hookClient = &http.Client{Timeout: 6 * time.Second}

func sendDeviceWebhook(client *model.Client, dev *model.Device, e *model.DeviceEvent) {
	var set SettingService
	url, _ := set.getString("deviceWebhook")
	if url == "" {
		return
	}
	secret, _ := set.getString("deviceWebhookSecret")
	var n int64
	database.GetDB().Model(model.Device{}).Where("client_id = ?", client.Id).Count(&n)
	p := deviceHookPayload{Kind: e.Kind, At: e.At, Actor: e.Actor, Ip: e.Ip, Devices: n,
		Client: deviceHookClient{Id: client.Id, Name: client.Name, DeviceLimit: client.DeviceLimit}}
	if e.Hwid != "" || e.DeviceId != 0 {
		p.Device = &deviceHookDev{Id: e.DeviceId, Hwid: e.Hwid, Model: e.Model, UserAgent: e.UserAgent}
	}
	body, _ := json.Marshal(p)
	for try := 0; try < 2; try++ {
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			logger.Warning("device webhook: ", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if secret != "" {
			req.Header.Set("X-Obhod-Secret", secret)
		}
		resp, err := hookClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
		time.Sleep(3 * time.Second)
	}
	logger.Warning("device webhook: not delivered (", e.Kind, ")")
}

// SendPanelEvent (obhod) — событие панели, не связанное с устройствами (например, истекающий сертификат),
// на тот же адрес уведомлений.
func SendPanelEvent(kind string, data map[string]interface{}) {
	var set SettingService
	url, _ := set.getString("deviceWebhook")
	if url == "" {
		return
	}
	secret, _ := set.getString("deviceWebhookSecret")
	payload := map[string]interface{}{"kind": kind, "at": time.Now().Unix()}
	for k, v := range data {
		payload[k] = v
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("X-Obhod-Secret", secret)
	}
	if resp, err := hookClient.Do(req); err == nil {
		resp.Body.Close()
	} else {
		logger.Warning("panel webhook: ", err)
	}
}
