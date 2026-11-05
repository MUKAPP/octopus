package cmd

import (
	"fmt"

	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/spf13/cobra"
)

var passwdConfigFile string

var passwdCmd = &cobra.Command{
	Use:   "passwd <password>",
	Short: "Set the existing user password (service must be stopped)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if args[0] == "" {
			return fmt.Errorf("password must not be empty")
		}
		if err := conf.Load(passwdConfigFile); err != nil {
			return err
		}
		if err := db.InitDB(conf.AppConfig.Database.Type, conf.AppConfig.Database.Path, conf.IsDebug()); err != nil {
			return fmt.Errorf("database init error: %w", err)
		}
		defer db.Close()

		var user model.User
		if err := db.GetDB().First(&user).Error; err != nil {
			return fmt.Errorf("failed to load user: %w", err)
		}
		user.Password = args[0]
		if err := user.HashPassword(); err != nil {
			return err
		}
		if err := db.GetDB().Model(&user).Update("password", user.Password).Error; err != nil {
			return fmt.Errorf("failed to update password: %w", err)
		}

		cmd.Println("Password updated. Restart the server to apply the change.")
		return nil
	},
}

func init() {
	passwdCmd.Flags().StringVar(&passwdConfigFile, "config", "", "config file (default is ./data/config.json)")
	// 不回显解析失败的参数：以连字符开头的密码可能被误当作 flag。
	passwdCmd.SetFlagErrorFunc(func(_ *cobra.Command, _ error) error {
		return fmt.Errorf("invalid passwd flags; use --help for usage and -- before a password beginning with a hyphen")
	})
	rootCmd.AddCommand(passwdCmd)
}
