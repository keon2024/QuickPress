package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"

	"quickpress/concurrency"
	"quickpress/config"
	"quickpress/web"
)

func main() {
	configPath := flag.String("config", "config/prod.yml", "本地配置文件路径")
	listen := flag.String("listen", "", "Web 控制台监听地址，例如 :8080")
	flag.Parse()

	resolvedConfigPath := config.ResolvePath(*configPath)
	loadedConfig, err := config.Load(resolvedConfigPath)
	if err != nil {
		log.Printf("读取配置失败，将使用默认配置启动控制台: %v", err)
		loadedConfig = config.Default()
	}

	serverListen := strings.TrimSpace(*listen)
	if serverListen == "" {
		serverListen = loadedConfig.App.Listen
	}
	if serverListen == "" {
		serverListen = ":8080"
	}

	manager := concurrency.NewManager()
	handler := web.New(manager, resolvedConfigPath)

	fmt.Printf("QuickPress 控制台已启动：%s\n", displayURL(serverListen))
	log.Fatal(http.ListenAndServe(serverListen, handler))
}

func displayURL(listen string) string {
	listen = strings.TrimSpace(listen)
	if strings.HasPrefix(listen, ":") {
		return "http://127.0.0.1" + listen
	}
	if strings.HasPrefix(listen, "0.0.0.0:") {
		return "http://127.0.0.1:" + strings.TrimPrefix(listen, "0.0.0.0:")
	}
	if strings.HasPrefix(listen, "http://") || strings.HasPrefix(listen, "https://") {
		return listen
	}
	return "http://" + listen
}
