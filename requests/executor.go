package requests

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"quickpress/config"
	"quickpress/utils"
)

type Result struct {
	Success    bool
	Latency    time.Duration
	StatusCode int
	Error      string
	Request    string
	Steps      []StepResult
}

type StepResult struct {
	StartedAt       time.Time
	RequestName     string
	Method          string
	URL             string
	Query           map[string]any
	RequestHeaders  map[string]string
	RequestBodyType string
	RequestBody     string
	ExpectedStatus  int
	Contains        []string
	Extractors      map[string]string
	StatusCode      int
	ResponseHeaders map[string]string
	ResponseBody    string
	Duration        time.Duration
	Success         bool
	Error           string
}

type Executor struct {
	global   map[string]string
	requests []config.Request
}

var placeholderPattern = regexp.MustCompile(`\$\{([^}]+)\}|\{\{([^}]+)\}\}|\{([A-Za-z0-9_.-]+)\}`)

func NewExecutor(cfg config.Config) *Executor {
	global := make(map[string]string, len(cfg.Global))
	for k, v := range cfg.Global {
		global[k] = v
	}
	requestItems := make([]config.Request, len(cfg.Requests))
	copy(requestItems, cfg.Requests)
	return &Executor{global: global, requests: requestItems}
}

func (e *Executor) Run(row map[string]string) Result {
	vars := make(map[string]string, len(e.global)+len(row)+8)
	for k, v := range e.global {
		vars[k] = v
	}
	for k, v := range row {
		vars[k] = v
	}

	var totalLatency time.Duration
	lastStatus := 0
	steps := make([]StepResult, 0, len(e.requests))
	for _, req := range e.requests {
		now := time.Now()
		vars["timestamp"] = strconv.FormatInt(now.Unix(), 10)
		vars["timestamp_ms"] = strconv.FormatInt(now.UnixMilli(), 10)

		method := renderString(req.Method, vars)
		body := renderMap(req.Body, vars)
		bodyType := toBodyType(req.BodyType)
		if len(body) > 0 && bodyType == utils.BodyNone {
			bodyType = utils.BodyJSON
		}
		query := renderMap(req.Query, vars)
		if len(query) == 0 && len(req.Params) > 0 {
			query = renderMap(req.Params, vars)
		}
		if shouldUseQueryAsForm(method, query, body, bodyType) {
			body = query
			bodyType = utils.BodyForm
			query = map[string]interface{}{}
		}
		headers := renderStringMap(req.Headers, vars)
		requestURL := renderString(req.URL, vars)
		step := StepResult{
			StartedAt:       now,
			RequestName:     req.Name,
			Method:          method,
			URL:             utils.BuildRequestURL(requestURL, query),
			Query:           cloneAnyMap(query),
			RequestHeaders:  cloneStringMap(headers),
			RequestBodyType: string(bodyType),
			RequestBody:     formatRequestBody(body, bodyType),
			ExpectedStatus:  req.ExpectedStatus,
			Contains:        append([]string(nil), req.Contains...),
			Extractors:      cloneStringMap(req.Extractors),
		}
		opts := utils.RequestOptions{
			Method:   method,
			URL:      requestURL,
			Query:    query,
			Body:     body,
			Headers:  headers,
			Timeout:  time.Duration(req.TimeoutMS) * time.Millisecond,
			BodyType: bodyType,
		}

		resp := utils.Do(opts)
		totalLatency += resp.Duration
		lastStatus = resp.StatusCode
		step.StatusCode = resp.StatusCode
		step.ResponseHeaders = cloneStringMap(resp.Headers)
		step.ResponseBody = resp.StrBody
		step.Duration = resp.Duration
		if resp.Error != nil {
			step.Error = fmt.Sprintf("%s 请求失败: %v", req.Name, resp.Error)
			step.Success = false
			steps = append(steps, step)
			return Result{Success: false, Latency: totalLatency, StatusCode: resp.StatusCode, Error: step.Error, Request: req.Name, Steps: steps}
		}
		if req.ExpectedStatus > 0 && resp.StatusCode != req.ExpectedStatus {
			step.Error = fmt.Sprintf("%s 状态码断言失败: expect=%d actual=%d", req.Name, req.ExpectedStatus, resp.StatusCode)
			step.Success = false
			steps = append(steps, step)
			return Result{Success: false, Latency: totalLatency, StatusCode: resp.StatusCode, Error: step.Error, Request: req.Name, Steps: steps}
		}
		for _, expected := range req.Contains {
			needle := renderString(expected, vars)
			if !strings.Contains(resp.StrBody, needle) {
				step.Error = fmt.Sprintf("%s 文本断言失败: %s", req.Name, needle)
				step.Success = false
				steps = append(steps, step)
				return Result{Success: false, Latency: totalLatency, StatusCode: resp.StatusCode, Error: step.Error, Request: req.Name, Steps: steps}
			}
		}
		for key, path := range req.Extractors {
			value, err := resp.GetString(renderString(path, vars))
			if err != nil {
				step.Error = fmt.Sprintf("%s 提取变量 %s 失败: %v", req.Name, key, err)
				step.Success = false
				steps = append(steps, step)
				return Result{Success: false, Latency: totalLatency, StatusCode: resp.StatusCode, Error: step.Error, Request: req.Name, Steps: steps}
			}
			vars[key] = value
		}
		step.Success = true
		steps = append(steps, step)
	}

	return Result{Success: true, Latency: totalLatency, StatusCode: lastStatus, Steps: steps}
}

func renderString(input string, vars map[string]string) string {
	if input == "" {
		return ""
	}
	rendered := input
	for i := 0; i < 5; i++ {
		next := renderStringOnce(rendered, vars)
		if next == rendered {
			return next
		}
		rendered = next
	}
	return rendered
}

func renderStringOnce(input string, vars map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(input, func(match string) string {
		groups := placeholderPattern.FindStringSubmatch(match)
		key := ""
		for i := 1; i < len(groups); i++ {
			if groups[i] != "" {
				key = groups[i]
				break
			}
		}
		if key == "" {
			return match
		}
		if value, ok := vars[key]; ok {
			return value
		}
		return match
	})
}

func renderStringMap(input map[string]string, vars map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = renderString(v, vars)
	}
	return out
}

func renderMap(input map[string]interface{}, vars map[string]string) map[string]interface{} {
	if len(input) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(input))
	for k, v := range input {
		out[k] = renderValue(v, vars)
	}
	return out
}

func renderValue(input interface{}, vars map[string]string) interface{} {
	switch value := input.(type) {
	case string:
		return renderString(value, vars)
	case map[string]interface{}:
		return renderMap(value, vars)
	case []interface{}:
		out := make([]interface{}, len(value))
		for i := range value {
			out[i] = renderValue(value[i], vars)
		}
		return out
	default:
		return input
	}
}

func shouldUseQueryAsForm(method string, query, body map[string]interface{}, bodyType utils.BodyType) bool {
	if !strings.EqualFold(strings.TrimSpace(method), "POST") {
		return false
	}
	if len(query) == 0 || len(body) > 0 {
		return false
	}
	return bodyType == utils.BodyNone || bodyType == utils.BodyForm
}

func toBodyType(bodyType string) utils.BodyType {
	switch strings.ToLower(strings.TrimSpace(bodyType)) {
	case config.BodyJSON:
		return utils.BodyJSON
	case config.BodyForm:
		return utils.BodyForm
	case config.BodyMultipart:
		return utils.BodyMultipart
	default:
		return utils.BodyNone
	}
}

func cloneAnyMap(input map[string]interface{}) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneValue(value)
	}
	return out
}

func cloneValue(input any) any {
	switch value := input.(type) {
	case map[string]interface{}:
		return cloneAnyMap(value)
	case []interface{}:
		out := make([]any, len(value))
		for i := range value {
			out[i] = cloneValue(value[i])
		}
		return out
	default:
		return value
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func formatRequestBody(body map[string]interface{}, bodyType utils.BodyType) string {
	if len(body) == 0 {
		return ""
	}
	switch bodyType {
	case utils.BodyForm:
		form := url.Values{}
		keys := sortedKeys(body)
		for _, key := range keys {
			form.Set(key, fmt.Sprint(body[key]))
		}
		return form.Encode()
	case utils.BodyMultipart:
		data, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return fmt.Sprint(body)
		}
		return string(data)
	default:
		data, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return fmt.Sprint(body)
		}
		return string(data)
	}
}

func sortedKeys(input map[string]interface{}) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
