package cronjob

import (
	"time"

	"github.com/alireza0/s-ui/service"
)

type CheckCoreJob struct {
	service.ConfigService
	downSince time.Time
	notified  bool
}

func NewCheckCoreJob() *CheckCoreJob {
	return &CheckCoreJob{}
}

func (s *CheckCoreJob) Run() {
	err := s.ConfigService.StartCore()
	// obhod: ядро не работает дольше минуты (и это не плановое обслуживание) — сообщить один раз; поднялось — тоже
	if s.ConfigService.CoreRunning() {
		if s.notified {
			service.SendPanelEvent("core_up", map[string]interface{}{"downFor": int(time.Since(s.downSince).Seconds())})
		}
		s.downSince, s.notified = time.Time{}, false
		return
	}
	if s.ConfigService.InMaintenance() {
		return
	}
	if s.downSince.IsZero() {
		s.downSince = time.Now()
	}
	if !s.notified && time.Since(s.downSince) > time.Minute {
		s.notified = true
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		service.SendPanelEvent("core_down", map[string]interface{}{"error": msg})
	}
}
