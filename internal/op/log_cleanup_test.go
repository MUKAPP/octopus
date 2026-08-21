package op

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
)

func TestCleanupLegacyRelayLogs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cleanup.db")
	if err := db.InitDB("sqlite", path, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Unix(1_700_000_000, 0)
	cutoff := now.Add(-relayLogCleanupRetention)
	rows := []model.RelayLog{
		{ID: 8101, Time: cutoff.Add(-time.Second).Unix()},
		{ID: 8102, Time: cutoff.Unix()},
		{ID: 8103, Time: cutoff.Add(time.Second).Unix()},
	}
	if err := db.GetDB().Create(&rows).Error; err != nil {
		t.Fatalf("insert logs: %v", err)
	}

	if err := cleanupLegacyRelayLogs(context.Background(), cutoff); err != nil {
		t.Fatalf("cleanup legacy logs: %v", err)
	}

	var remaining []model.RelayLog
	if err := db.GetDB().Order("id").Find(&remaining).Error; err != nil {
		t.Fatalf("query logs: %v", err)
	}
	if len(remaining) != 2 || remaining[0].ID != 8102 || remaining[1].ID != 8103 {
		t.Fatalf("remaining logs = %+v, want boundary and recent rows", remaining)
	}
}
