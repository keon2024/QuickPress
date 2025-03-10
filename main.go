package main

import (
	"fmt"
	"quickpress/concurrency"
	"quickpress/config"

	"github.com/spf13/viper"
)

const (
	configPath = "config/prod.yml"
)

func main() {
	viper.SetConfigFile(configPath)

	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Error reading config file, %s", err)
	}
	var conf config.Config
	if err := viper.Unmarshal(&conf); err != nil {
		fmt.Printf("Error unmarshalling config, %s", err)
	}

	// Example of accessing a config value
	fmt.Println("Host: ", conf.Global["gd_host"])
	// 并发执行任务
	concurrency.GroutinePool(conf.Concurrency)
}
