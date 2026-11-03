package cronjob

import (
	"time"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

// DeviceJob (obhod) убирает из ядра прежние ключи устройств, срок которых вышел.
type DeviceJob struct {
	service.DeviceService
	runs     int
	certSent map[uint]time.Time
}

func NewDeviceJob() *DeviceJob { return &DeviceJob{} }

func (j *DeviceJob) Run() {
	if err := j.DropExpiredPrev(); err != nil {
		logger.Warning("device job: ", err)
	}
	if err := j.GraceTick(time.Now().Unix()); err != nil {
		logger.Warning("device job: grace: ", err)
	}
	if j.runs%360 == 0 { // раз в 6 часов
		j.PruneEvents()
		j.checkCerts()
	}
	j.runs++
}

// checkCerts: сертификат истёк, истекает меньше чем через 14 дней или не читается — уведомление (не чаще раза в сутки).
func (j *DeviceJob) checkCerts() {
	var in service.InboundService
	certs, err := in.Certs()
	if err != nil {
		return
	}
	if j.certSent == nil {
		j.certSent = map[uint]time.Time{}
	}
	for _, c := range certs {
		if c.Source == "acme" || (c.Error == "" && c.DaysLeft >= 14) {
			continue
		}
		if time.Since(j.certSent[c.TlsId]) < 23*time.Hour {
			continue
		}
		j.certSent[c.TlsId] = time.Now()
		logger.Warning("certificate ", c.Name, ": days left ", c.DaysLeft, " ", c.Error)
		service.SendPanelEvent("cert", map[string]interface{}{"cert": c})
	}
}
