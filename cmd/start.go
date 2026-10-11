package cmd

import (
	"path/filepath"

	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/server"
	"github.com/bestruirui/octopus/internal/task"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/shutdown"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start " + conf.APP_NAME,
	PreRun: func(cmd *cobra.Command, args []string) {
		conf.PrintBanner()
		conf.Load(cfgFile)
		log.SetLevel(conf.AppConfig.Log.Level)
	},
	Run: func(cmd *cobra.Command, args []string) {
		shutdown.Init(log.Logger)
		defer shutdown.Listen()
		if err := db.InitDB(conf.AppConfig.Database.Type, conf.AppConfig.Database.Path, conf.IsDebug()); err != nil {
			log.Errorf("database init error: %v", err)
			return
		}
		shutdown.Register(db.Close)

		if err := op.InitCache(); err != nil {
			log.Errorf("cache init error: %v", err)
			return
		}
		shutdown.Register(op.SaveCache)

		if err := op.UserInit(); err != nil {
			log.Errorf("user init error: %v", err)
			return
		}

		// 日志图片缓存初始化失败只降级本地预览（warn），不阻断服务；远程图片 URL 仍可用。
		if err := op.InitRelayLogImageFiles(relayLogImageDirectory()); err != nil {
			log.Warnf("relay log image cache init failed: %v", err)
		}

		if err := server.Start(); err != nil {
			log.Errorf("server start error: %v", err)
			return
		}
		shutdown.Register(server.Close)

		task.Init()
		go task.RUN()
	},
}

func init() {
	startCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./data/config.json)")
	rootCmd.AddCommand(startCmd)
}

// relayLogImageDirectory 返回日志图片缓存目录：默认位于配置旁（默认 data/log-images），
// 显式 --config 时随配置目录隔离；相对配置路径仍沿用进程相对目录的原有语义。
func relayLogImageDirectory() string {
	configFile := cfgFile
	if configFile == "" {
		configFile = viper.ConfigFileUsed()
	}
	if configFile == "" {
		return filepath.Join("data", "log-images")
	}
	return filepath.Join(filepath.Dir(configFile), "log-images")
}
