package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

// DeviceJob (obhod) убирает из ядра прежние ключи устройств, срок которых вышел.
type DeviceJob struct {
	service.DeviceService
	runs int
}

func NewDeviceJob() *DeviceJob { return &DeviceJob{} }

func (j *DeviceJob) Run() {
	if err := j.DropExpiredPrev(); err != nil {
		logger.Warning("device job: ", err)
	}
	if j.runs%360 == 0 { // раз в 6 часов
		j.PruneEvents()
	}
	j.runs++
}
