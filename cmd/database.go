package cmd

import (
	"fmt"
	"os"

	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/db"
	"github.com/spf13/cobra"
)

var compactConfigFile string

var databaseCmd = &cobra.Command{
	Use:   "database",
	Short: "Database maintenance commands",
}

var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact a SQLite database offline (service must be stopped)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := conf.Load(compactConfigFile); err != nil {
			return err
		}
		if conf.AppConfig.Database.Type != "sqlite" {
			return fmt.Errorf("database compact only supports database.type=sqlite, got %q", conf.AppConfig.Database.Type)
		}

		path := conf.AppConfig.Database.Path
		before, after, err := db.CompactSQLite(path)
		if err != nil {
			return err
		}

		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat database file: %w", err)
		}
		fmt.Printf("database compact: %s\n", path)
		fmt.Printf("  before: page_size=%d page_count=%d freelist_count=%d auto_vacuum=%d\n",
			before.PageSize, before.PageCount, before.FreelistCount, before.AutoVacuum)
		fmt.Printf("  after:  page_size=%d page_count=%d freelist_count=%d auto_vacuum=%d size=%d bytes\n",
			after.PageSize, after.PageCount, after.FreelistCount, after.AutoVacuum, info.Size())
		fmt.Println("  quick_check: ok")
		return nil
	},
}

func init() {
	compactCmd.Flags().StringVar(&compactConfigFile, "config", "", "config file (default is ./data/config.json)")
	_ = compactCmd.MarkFlagRequired("config")
	databaseCmd.AddCommand(compactCmd)
	rootCmd.AddCommand(databaseCmd)
}
