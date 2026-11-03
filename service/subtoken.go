package service

// obhod: секретные адреса подписок. У каждого клиента свой случайный SubToken; подписка ищется по нему,
// а по имени — только пока включено subByName (для старых ссылок).

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
)

func NewSubToken() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	return "s" + base64.RawURLEncoding.EncodeToString(b)
}

// EnsureSubTokens выдаёт токены клиентам, у которых их ещё нет (после обновления с обычной s-ui).
func EnsureSubTokens() {
	db := database.GetDB()
	var ids []uint
	if err := db.Model(model.Client{}).Where("sub_token IS NULL OR sub_token = ''").Pluck("id", &ids).Error; err != nil {
		logger.Warning("sub tokens: ", err)
		return
	}
	for _, id := range ids {
		db.Model(model.Client{}).Where("id = ?", id).Update("sub_token", NewSubToken())
	}
	if len(ids) > 0 {
		logger.Info("sub tokens issued: ", len(ids))
	}
}

// ClientBySub — включённый клиент по адресу подписки: секретный токен или (пока разрешено) имя.
func ClientBySub(subId string) (*model.Client, error) {
	db := database.GetDB()
	c := &model.Client{}
	err := db.Model(model.Client{}).Where("enable = true AND sub_token = ? AND sub_token != ''", subId).First(c).Error
	if err == nil {
		return c, nil
	}
	if !database.IsNotFound(err) {
		return nil, err
	}
	if (&SettingService{}).SubByName() {
		err = db.Model(model.Client{}).Where("enable = true AND name = ?", subId).First(c).Error
		if err == nil {
			return c, nil
		}
	}
	return nil, err
}
