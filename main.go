package main

import (
	"fmt"

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
	host := viper.GetString("global.gd_host")

	// Example of accessing a config value
	fmt.Println("Host: ", host)

}
