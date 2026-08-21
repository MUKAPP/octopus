package migrate

import (
	"github.com/bestruirui/octopus/internal/model"
	"gorm.io/gorm"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 4,
		Up:      removeRelayLogSettings,
	})
}

// 004: remove log settings that no longer apply to the in-memory log store.
func removeRelayLogSettings(db *gorm.DB) error {
	return db.Where("key IN ?", []string{
		"relay_log_keep_period",
		"relay_log_keep_enabled",
		"relay_log_content_enabled",
		"relay_log_content_max_bytes",
	}).Delete(&model.Setting{}).Error
}
