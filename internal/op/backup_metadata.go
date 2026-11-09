package op

import (
	"context"
	"fmt"

	"github.com/bestruirui/octopus/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Import snapshots once, then extend existing keys only by requests actually inserted.
// A newly inserted incoming snapshot already covers its metadata; adding those requests
// again would double count the first import. Existing snapshot conflicts never add the
// incoming cumulative total, so repeated and progressively newer imports stay idempotent.
func importRequestMetadataAndUsage(ctx context.Context, tx *gorm.DB, dump *model.DBDump) (int64, int64, error) {
	var usageRows, metadataRows int64
	insertedSnapshots := make(map[string]struct{}, len(dump.StatsUsage))
	for _, row := range dump.StatsUsage {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if result.Error != nil {
			return 0, 0, fmt.Errorf("import stats_usage: %w", result.Error)
		}
		if result.RowsAffected > 0 {
			usageRows++
			insertedSnapshots[row.KeyHash] = struct{}{}
		}
	}
	deltas := make(map[model.StatsUsageKey]model.StatsUsage)
	for _, row := range dump.RequestMetadata {
		// Individual INSERT outcomes identify real new IDs portably across SQLite, MySQL and
		// Postgres, including duplicate IDs inside one dump and concurrent inserts.
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if result.Error != nil {
			return 0, 0, fmt.Errorf("import request_metadata: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			continue
		}
		metadataRows++
		usage := statsUsageOfRequestMetadata(row)
		key := usage.Key()
		hash, err := key.Hash()
		if err != nil {
			return 0, 0, fmt.Errorf("import metadata usage key: %w", err)
		}
		if _, covered := insertedSnapshots[hash]; covered {
			continue
		}
		usage.KeyHash = hash
		if delta, ok := deltas[key]; ok {
			delta.Add(usage)
			deltas[key] = delta
		} else {
			deltas[key] = usage
		}
	}
	if err := persistStatsUsageRows(ctx, tx, deltas); err != nil {
		return 0, 0, fmt.Errorf("import metadata usage increments: %w", err)
	}
	return usageRows + int64(len(deltas)), metadataRows, nil
}

func statsUsageOfRequestMetadata(row model.RequestMetadata) model.StatsUsage {
	usage := model.StatsUsage{
		Date: row.Date, RequestModelName: row.RequestModelName, ActualModelName: row.ActualModelName,
		APIKeyID: row.APIKeyID, APIKeyName: row.APIKeyName, ChannelID: row.ChannelID, ChannelName: row.ChannelName, Success: row.Success,
		StatsMetrics: model.StatsMetrics{InputToken: row.InputToken, OutputToken: row.OutputToken,
			InputCost: row.InputCost, OutputCost: row.OutputCost, WaitTime: row.TotalDurationMS},
	}
	// Partial raw cache counts are retained for detailed analytics, but the legacy
	// aggregate only accounted usage that reached normalized RecordUsage.
	if row.UsagePresent && row.CacheReadToken != nil {
		usage.CachedToken = *row.CacheReadToken
	}
	if row.Success {
		usage.RequestSuccess = 1
	} else {
		usage.RequestFailed = 1
	}
	return usage
}
