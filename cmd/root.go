/*
Copyright © 2022 Joker
*/
package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sysafari.com/softpak/rattler/internal/config"
	"sysafari.com/softpak/rattler/internal/web"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "rattler",
	Short: "SoftPak Client",
	Long: `Rattler will simultaneously start the file server to access
the Export XML file and the tax bill file, and simultaneously start the
Import XML listener and the Export XML (NL|BE) file creation listener asynchronously.
For example:`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		// Initialize global configuration
		// 包括：
		// 1. 配置文件初始化
		// 2. 日志初始化
		// 3. 文件移动队列初始化
		if err := config.InitConfig(); err != nil {
			log.Fatalf("Failed to initialize configuration: %v", err)
		}

		// Initialize RabbitMQ
		if err := InitRabbitMQ(); err != nil {
			log.Fatalf("Failed to initialize RabbitMQ: %v", err)
		}

		// Start consumers
		if err := StartMessageQueueConsumers(); err != nil {
			log.Fatalf("Failed to start RabbitMQ consumers: %v", err)
		}

		// Setup graceful shutdown
		setupGracefulShutdown()

		// 开启Export XML文件监听
		StartWatchExportXmlDirWorker()

		// 开启税单文件监听
		StartWatchTaxBillDirWorker()


		// start file server
		web.StartServer()
	},
}

// setupGracefulShutdown sets up signal handling for graceful shutdown
func setupGracefulShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Info("Shutdown signal received, closing resources...")

		// Close RabbitMQ connections gracefully
		CloseRabbitMQ()

		log.Info("All resources closed, shutting down")
		os.Exit(0)
	}()
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}

}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", ".rattler.yaml", "config file (default is .rattler.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	//rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".rattler" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".rattler")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}

	initLog()
}

func initLog() {
	path, _ := os.Executable()
	_, exec := filepath.Split(path)
	logFile := exec + ".log"

	// Use the config package global initialization function
	// The actual initialization will happen in InitConfig() in the Run function
	// This is just to ensure logging is set up early if needed before full config initialization
	logDir := viper.GetString("log.directory")
	if logDir == "" {
		logDir = "out/log/"
	}
	logFilePath := filepath.Join(logDir, logFile)

	config.InitLog(logFilePath, viper.GetString("log.level"))
}
