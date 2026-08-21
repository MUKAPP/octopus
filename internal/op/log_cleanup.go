package op

import (
	"context"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
)

const relayLogCleanupRetention = 7 * 24 * time.Hour

// RelayLogCleanupTask removes old terminal memory records and legacy persisted logs.
// New relay logs are not written to SQLite, but old relay_logs rows can remain after upgrading.
func RelayLogCleanupTask() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	now := time.Now()
	cutoff := now.Add(-relayLogCleanupRetention)
	memoryRemoved := RelayLogStorePrune(cutoff)
	if err := cleanupLegacyRelayLogs(ctx, cutoff); err != nil {
		log.Errorf("relay log cleanup failed: %v", err)
		return
	}
	log.Debugf("relay log cleanup completed: memory=%d, cutoff=%s", memoryRemoved, cutoff.Format(time.RFC3339))
}

func cleanupLegacyRelayLogs(ctx context.Context, cutoff time.Time) error {
	return db.GetDB().WithContext(ctx).Connection(func(tx *gorm.DB) error {
		result := tx.Where("time < ?", cutoff.Unix()).Delete(&model.RelayLog{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return db.ReclaimSQLiteFreePages(tx)
	})
}
