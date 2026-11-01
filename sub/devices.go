package sub

// obhod: подписка в режиме устройств. Приложение присылает x-hwid (Happ, v2RayTun, «Обход для чайников» и др.);
// устройство регистрируется и получает свои ключи. Превышен лимит или нет x-hwid — вместо серверов одна
// строка-подсказка и заголовки, по которым Happ/v2RayTun показывают причину (как у Remnawave).

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strconv"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"

	"github.com/gin-gonic/gin"
)

type subRefusal struct {
	text    string
	headers map[string]string
}

func deviceInfo(c *gin.Context) service.DeviceInfo {
	h := c.Request.Header
	return service.DeviceInfo{
		Hwid:      h.Get("X-Hwid"),
		Model:     h.Get("X-Device-Model"),
		Os:        firstNonEmpty(h.Get("X-Device-Os"), h.Get("X-Ver-Os")),
		UserAgent: h.Get("User-Agent"),
	}
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

// resolveClient находит клиента подписки; в режиме устройств возвращает копию клиента с ключами устройства.
func resolveClient(subId string, info service.DeviceInfo) (*model.Client, *subRefusal, error) {
	client := &model.Client{}
	if err := database.GetDB().Model(model.Client{}).Where("enable = true and name = ?", subId).First(client).Error; err != nil {
		return nil, nil, err
	}
	if client.DeviceLimit <= 0 {
		return client, nil, nil
	}
	var ds service.DeviceService
	dev, changed, err := ds.Register(client, info)
	switch {
	case errors.Is(err, service.ErrNoHwid):
		return nil, &subRefusal{
			text:    "⚠ Это приложение не сообщает идентификатор устройства. Используй Happ, v2RayTun или «Обход для чайников»",
			headers: map[string]string{"x-hwid-not-supported": "true"},
		}, nil
	case errors.Is(err, service.ErrDeviceLimit):
		return nil, &subRefusal{
			text: "⚠ Достигнут лимит устройств (" + strconv.Itoa(client.DeviceLimit) + "). Освободи место в боте или купи ещё устройство",
			headers: map[string]string{"x-hwid-max-devices-reached": "true", "x-hwid-limit": "true",
				"x-hwid-device-limit": strconv.Itoa(client.DeviceLimit)},
		}, nil
	case err != nil:
		return nil, nil, err
	}
	if changed {
		if err := service.ApplyClient(client.Id); err != nil {
			logger.Warning("devices: update inbound users: ", err)
		}
	}
	cp := *client
	if cp.Config, err = service.DeviceConfig(client.Config, client.Name, dev); err != nil {
		return nil, nil, err
	}
	if cp.Links, err = service.DeviceLinks(client.Links, client.Config, dev); err != nil {
		return nil, nil, err
	}
	return &cp, nil, nil
}

// refuse отдаёт одну строку-подсказку вместо серверов (её покажет любой клиент как имя «сервера»).
func refuse(c *gin.Context, r *subRefusal) {
	for k, v := range r.headers {
		c.Writer.Header().Set(k, v)
	}
	c.Writer.Header().Set("Profile-Update-Interval", "1")
	link := "vless://00000000-0000-4000-8000-000000000000@0.0.0.0:1?type=tcp&security=none#" + url.PathEscape(r.text)
	c.String(200, base64.StdEncoding.EncodeToString([]byte(link)))
}
