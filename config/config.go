package config

// Config 加载的配置文件
type Config struct {
	Concurrency Concurrency       `mapstructure:"concurrency"`
	Reader      reader            `mapstructure:"reader"`
	Global      map[string]string `mapstructure:"global"`
	Requests    []request         `mapstructure:"requests"`
}

// concurrency 配置文件中的并发配置
type Concurrency struct {
	Loop   int    `mapstructure:"loop"`
	Unit   string `mapstructure:"unit"`
	Stages []struct {
		Duration int `mapstructure:"duration"`
		Target   int `mapstructure:"target"`
	} `mapstructure:"stages"`
}

// reader 文件读取配置
type reader struct {
	Type string `mapstructure:"type"`
	File string `mapstructure:"file"`
}

// request 请求配置
type request struct {
	Name       string            `mapstructure:"name"`
	URL        string            `mapstructure:"url"`
	Method     string            `mapstructure:"method"`
	Headers    map[string]string `mapstructure:"headers"`
	Params     map[string]string `mapstructure:"params"`
	Assertions map[string]string `mapstructure:"assertions"`
}
