package main

import (
	"fmt"
	"quickpress/config"
	"quickpress/reader"
	"time"

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
	// 读取文件，写入channel
	stopSignal := make(chan bool)
	r1 := reader.NewReader(reader.CSV, reader.ReaderConfig{
		FilePath: "/Users/edy/Desktop/data.csv", StopSignal: stopSignal})

	go func() {
		time.Sleep(30 * time.Second)
		close(stopSignal)
		fmt.Println("关闭stopSignal")
	}()
	lines := r1.Read()
	for line := range lines {
		fmt.Println(line)
	}
	time.Sleep(2 * time.Second)
	fmt.Println("程序结束")

	// 构造requests请求链，从channel中读取数据

	// 并发执行任务
	// concurrency.GroutinePool(conf.Concurrency, func() {
	// 	time.Sleep(time.Second)
	// 	fmt.Println("Hello World! 协程号：")
	// })
}
