package cronjob

// obhod: ежедневная копия базы (без статистики и журнала изменений) в db/backups, хранятся последние 7.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/logger"
)

const keepBackups = 7

type BackupJob struct{}

func NewBackupJob() *BackupJob { return &BackupJob{} }

func (j *BackupJob) Run() {
	if err := MakeBackup(filepath.Join(filepath.Dir(config.GetDBPath()), "backups"), time.Now()); err != nil {
		logger.Warning("daily backup: ", err)
	}
}

// MakeBackup пишет копию базы в dir и удаляет старые сверх keepBackups.
func MakeBackup(dir string, now time.Time) error {
	data, err := database.GetDb("stats,changes")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name := filepath.Join(dir, "s-ui-"+now.Format("2006-01-02")+".db")
	tmp := name + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, name); err != nil {
		return err
	}
	entries, _ := os.ReadDir(dir)
	var files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "s-ui-") && strings.HasSuffix(e.Name(), ".db") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for len(files) > keepBackups {
		_ = os.Remove(filepath.Join(dir, files[0]))
		files = files[1:]
	}
	logger.Info("daily backup: ", name)
	return nil
}
