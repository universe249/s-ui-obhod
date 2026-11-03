package cronjob

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/logger"

	"github.com/op/go-logging"
)

func TestMakeBackupKeepsSeven(t *testing.T) {
	logger.InitLogger(logging.ERROR)
	if err := database.InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "backups")
	day := time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		if err := MakeBackup(dir, day.AddDate(0, 0, i)); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 7 || entries[0].Name() != "s-ui-2026-11-04.db" {
		t.Fatalf("%d files, first %s", len(entries), entries[0].Name())
	}
	b, _ := os.ReadFile(filepath.Join(dir, entries[6].Name()))
	if len(b) < 16 || string(b[:15]) != "SQLite format 3" {
		t.Fatal("not a database file")
	}
}
