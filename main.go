package main

import (
	"ashokshau/tg-web/internal/bot"
	"ashokshau/tg-web/internal/cleanup"
	"ashokshau/tg-web/internal/config"
	"ashokshau/tg-web/internal/db"
	"ashokshau/tg-web/internal/downloader"
	"ashokshau/tg-web/internal/webapp"
	"os"
	"time"

	"github.com/AshokShau/gotdbot"
)

//go:generate go run github.com/AshokShau/gotdbot/scripts/tools

func resolveTdLibPath() string {
	for _, candidate := range []string{"./libtdjson.so.1.8.69", "./libtdjson.so", "/usr/local/lib/libtdjson.so.1.8.69", "/usr/local/lib/libtdjson.so"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func main() {
	if err := config.LoadEnv(); err != nil {
		panic(err)
	}

	if err := db.InitDatabase(); err != nil {
		panic("failed to connect database: " + err.Error())
	}

	tdDir := "td"
	_ = os.RemoveAll(tdDir)
	_ = os.MkdirAll(tdDir, 0o755)

	libPath := resolveTdLibPath()
	manager := gotdbot.NewClientManager(libPath)
	clientConfig := gotdbot.DefaultClientConfig()
	clientConfig.AutoRetry = &gotdbot.AutoRetry{
		ChatNotFound: true,
		MaxFloodWait: 5 * time.Second,
	}

	clientConfig.DatabaseDirectory = tdDir
	client, err := manager.RegisterClient(config.ApiId, config.ApiHash, config.Token, clientConfig)
	if err != nil {
		panic("failed to register client: " + err.Error())
	}

	if config.DlBotToken != "" {
		dlClientConfig := gotdbot.DefaultClientConfig()
		dlClientConfig.AutoRetry = &gotdbot.AutoRetry{
			ChatNotFound: true,
		}
		dlClientConfig.DatabaseDirectory = tdDir + "_dl"
		_ = os.RemoveAll(dlClientConfig.DatabaseDirectory)
		_ = os.MkdirAll(dlClientConfig.DatabaseDirectory, 0o755)

		dlClient, err := manager.RegisterClient(config.ApiId, config.ApiHash, config.DlBotToken, dlClientConfig)
		if err != nil {
			client.Logger.Warnf("failed to register dl client: %s", err.Error())
			downloader.DlBot = client
		} else {
			downloader.DlBot = dlClient
			dlClient.Logger.Infof("dl client registered")
		}
	}

	cleanupScheduler, stopCleanup := cleanup.Start(client)
	bot.LoadModules(client, cleanupScheduler.CancelChat)
	if config.LoggerId != 0 {
		_, _ = client.SendMessage(config.LoggerId, "TeleMusic Web Player Bot has started!", nil)
	}
	webapp.RegisterRoutes(client, cleanupScheduler.CancelChat)

	manager.Idle()
	stopCleanup()
	_ = db.Instance.Close()
	client.Logger.Info("The bot is shutting down...")
	_ = os.RemoveAll(config.DownloadsDir)
}
