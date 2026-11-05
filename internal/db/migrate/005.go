package migrate

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 5,
		Up:      repairStatsHourlyRows,
	})
}

// 005: repair stats_hourlies written while hour was treated as an auto-increment key.
// hour=0 was inserted as NULL, so its cumulative value landed on newly allocated rows:
// rows outside 0-23 are never read and grow without bound; rows dated today for hours
// that have not happened yet can only be such copies and would be added to later today.
func repairStatsHourlyRows(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if !db.Migrator().HasTable("stats_hourlies") {
		return nil
	}

	// MySQL generates a new value when 0 is inserted into an AUTO_INCREMENT column,
	// so the column must stop being auto-increment for hour=0 to be stored as 0.
	// SQLite and PostgreSQL keep explicitly supplied key values unchanged.
	if db.Dialector.Name() == "mysql" {
		var count int64
		if err := db.Raw(
			"SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ? AND extra LIKE ?",
			"stats_hourlies", "hour", "%auto_increment%",
		).Scan(&count).Error; err != nil {
			return fmt.Errorf("failed to inspect stats_hourlies.hour: %w", err)
		}
		if count > 0 {
			if err := db.Exec("ALTER TABLE stats_hourlies MODIFY COLUMN `hour` bigint NOT NULL").Error; err != nil {
				return fmt.Errorf("failed to drop auto_increment from stats_hourlies.hour: %w", err)
			}
		}
	}

	if err := db.Exec("DELETE FROM stats_hourlies WHERE hour < 0 OR hour > 23").Error; err != nil {
		return fmt.Errorf("failed to delete invalid stats_hourlies rows: %w", err)
	}
	now := time.Now()
	if err := db.Exec(
		"DELETE FROM stats_hourlies WHERE date = ? AND hour > ?",
		now.Format("20060102"), now.Hour(),
	).Error; err != nil {
		return fmt.Errorf("failed to delete future stats_hourlies rows: %w", err)
	}
	return nil
}
