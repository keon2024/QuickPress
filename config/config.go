package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	UnitSecond = "s"
	UnitMinute = "m"
	UnitHour   = "h"

	ReaderCSV = "csv"

	BodyNone      = ""
	BodyJSON      = "json"
	BodyForm      = "form"
	BodyMultipart = "multipart"
)

// Config 是 QuickPress 的完整配置。
type Config struct {
	App         AppConfig         `json:"app" yaml:"app" mapstructure:"app"`
	Concurrency Concurrency       `json:"concurrency" yaml:"concurrency" mapstructure:"concurrency"`
	Reader      ReaderConfig      `json:"reader" yaml:"reader" mapstructure:"reader"`
	Global      map[string]string `json:"global,omitempty" yaml:"global,omitempty" mapstructure:"global"`
	Requests    []Request         `json:"requests" yaml:"requests" mapstructure:"requests"`
}

type AppConfig struct {
	Listen string `json:"listen,omitempty" yaml:"listen,omitempty" mapstructure:"listen"`
}

type Concurrency struct {
	Loop   int     `json:"loop" yaml:"loop" mapstructure:"loop"`
	Unit   string  `json:"unit" yaml:"unit" mapstructure:"unit"`
	Stages []Stage `json:"stages" yaml:"stages" mapstructure:"stages"`
}

type Stage struct {
	Label    string `json:"label,omitempty" yaml:"label,omitempty" mapstructure:"label"`
	Duration int    `json:"duration" yaml:"duration" mapstructure:"duration"`
	Target   int    `json:"target" yaml:"target" mapstructure:"target"`
}

type ReaderConfig struct {
	Type string `json:"type,omitempty" yaml:"type,omitempty" mapstructure:"type"`
	File string `json:"file,omitempty" yaml:"file,omitempty" mapstructure:"file"`
}

type Request struct {
	Name           string                 `json:"name" yaml:"name" mapstructure:"name"`
	Method         string                 `json:"method" yaml:"method" mapstructure:"method"`
	URL            string                 `json:"url" yaml:"url" mapstructure:"url"`
	TimeoutMS      int                    `json:"timeout_ms,omitempty" yaml:"timeout_ms,omitempty" mapstructure:"timeout_ms"`
	BodyType       string                 `json:"body_type,omitempty" yaml:"body_type,omitempty" mapstructure:"body_type"`
	Headers        map[string]string      `json:"headers,omitempty" yaml:"headers,omitempty" mapstructure:"headers"`
	Query          map[string]interface{} `json:"query,omitempty" yaml:"query,omitempty" mapstructure:"query"`
	Params         map[string]interface{} `json:"params,omitempty" yaml:"params,omitempty" mapstructure:"params"`
	Body           map[string]interface{} `json:"body,omitempty" yaml:"body,omitempty" mapstructure:"body"`
	ExpectedStatus int                    `json:"expected_status,omitempty" yaml:"expected_status,omitempty" mapstructure:"expected_status"`
	Contains       []string               `json:"contains,omitempty" yaml:"contains,omitempty" mapstructure:"contains"`
	Extractors     map[string]string      `json:"extractors,omitempty" yaml:"extractors,omitempty" mapstructure:"extractors"`
}

func Default() Config {
	cfg := Config{
		App: AppConfig{
			Listen: ":8080",
		},
		Concurrency: Concurrency{
			Loop: -1,
			Unit: UnitSecond,
			Stages: []Stage{
				{Label: "预热", Duration: 10, Target: 2},
				{Label: "稳定", Duration: 30, Target: 6},
				{Label: "冲刺", Duration: 60, Target: 10},
			},
		},
		Reader: ReaderConfig{
			Type: ReaderCSV,
			File: "",
		},
		Global: map[string]string{
			"host":  "https://postman-echo.com",
			"scene": "quickpress",
		},
		Requests: []Request{
			{
				Name:           "查询示例",
				Method:         "GET",
				URL:            "${host}/get",
				TimeoutMS:      3000,
				ExpectedStatus: 200,
				Query: map[string]interface{}{
					"scene":   "${scene}",
					"keyword": "hello",
				},
				Contains: []string{"hello"},
			},
		},
	}
	cfg.Normalize()
	return cfg
}

func Load(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		cfg := Default()
		cfg.Normalize()
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	return Parse(data)
}

func Parse(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	cfg.Normalize()
	return cfg, nil
}

func Save(path string, cfg Config) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("配置文件路径不能为空")
	}

	data, err := Marshal(cfg)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return os.WriteFile(path, data, 0o644)
}

func Marshal(cfg Config) ([]byte, error) {
	cfg.Normalize()
	return yaml.Marshal(cfg)
}

func ResolvePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func (c *Config) Normalize() {
	if c.App.Listen == "" {
		c.App.Listen = ":8080"
	}
	if c.Concurrency.Loop == 0 {
		c.Concurrency.Loop = 1
	}
	c.Concurrency.Unit = normalizeUnit(c.Concurrency.Unit)
	if len(c.Concurrency.Stages) == 0 {
		c.Concurrency.Stages = []Stage{{Label: "默认阶段", Duration: 30, Target: 1}}
	}

	sort.Slice(c.Concurrency.Stages, func(i, j int) bool {
		return c.Concurrency.Stages[i].Duration < c.Concurrency.Stages[j].Duration
	})
	prev := 0
	for i := range c.Concurrency.Stages {
		stage := &c.Concurrency.Stages[i]
		if stage.Duration <= prev {
			stage.Duration = prev + 1
		}
		if stage.Target < 0 {
			stage.Target = 0
		}
		prev = stage.Duration
	}

	c.Reader.Type = strings.ToLower(strings.TrimSpace(c.Reader.Type))
	if c.Reader.Type == "" && c.Reader.File != "" {
		c.Reader.Type = ReaderCSV
	}

	if c.Global == nil {
		c.Global = map[string]string{}
	}
	if c.Requests == nil {
		c.Requests = []Request{}
	}

	for i := range c.Requests {
		req := &c.Requests[i]
		req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
		if req.Method == "" {
			req.Method = "GET"
		}
		if req.TimeoutMS <= 0 {
			req.TimeoutMS = 5000
		}
		req.BodyType = normalizeBodyType(req.BodyType)
		if req.Headers == nil {
			req.Headers = map[string]string{}
		}
		if req.Query == nil && len(req.Params) > 0 {
			req.Query = cloneAnyMap(req.Params)
		}
		if req.Query == nil {
			req.Query = map[string]interface{}{}
		}
		if req.Body == nil {
			req.Body = map[string]interface{}{}
		}
		if req.Extractors == nil {
			req.Extractors = map[string]string{}
		}
		if req.Contains == nil {
			req.Contains = []string{}
		}
	}
}

func (c Config) ValidateForRun() error {
	if c.Concurrency.Loop < -1 {
		return errors.New("loop 只能是正整数或 -1")
	}
	if len(c.Concurrency.Stages) == 0 {
		return errors.New("至少需要一个并发阶段")
	}
	for i, stage := range c.Concurrency.Stages {
		if stage.Duration <= 0 {
			return fmt.Errorf("第 %d 个阶段的 duration 必须大于 0", i+1)
		}
		if stage.Target < 0 {
			return fmt.Errorf("第 %d 个阶段的 target 不能小于 0", i+1)
		}
	}
	if len(c.Requests) == 0 {
		return errors.New("至少需要配置一个请求")
	}
	for i, req := range c.Requests {
		if strings.TrimSpace(req.URL) == "" {
			return fmt.Errorf("第 %d 个请求缺少 URL", i+1)
		}
	}
	return nil
}

func normalizeUnit(unit string) string {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case UnitMinute:
		return UnitMinute
	case UnitHour:
		return UnitHour
	default:
		return UnitSecond
	}
}

func normalizeBodyType(bodyType string) string {
	switch strings.ToLower(strings.TrimSpace(bodyType)) {
	case BodyJSON:
		return BodyJSON
	case BodyForm:
		return BodyForm
	case BodyMultipart:
		return BodyMultipart
	default:
		return BodyNone
	}
}

func cloneAnyMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
