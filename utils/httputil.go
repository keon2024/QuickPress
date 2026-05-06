package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/valyala/fasthttp"
)

var client = &fasthttp.Client{
	MaxConnsPerHost:               2048,
	ReadTimeout:                   10 * time.Second,
	WriteTimeout:                  10 * time.Second,
	MaxIdleConnDuration:           30 * time.Second,
	NoDefaultUserAgentHeader:      true,
	DisableHeaderNamesNormalizing: true,
}

type HttpResp struct {
	StatusCode int
	Error      error
	JsonObj    *ast.Node
	StrBody    string
	Duration   time.Duration
	Headers    map[string]string
	RequestURL string
}

func (r *HttpResp) GetNode(path string) (*ast.Node, error) {
	if r == nil || r.JsonObj == nil {
		return nil, fmt.Errorf("no JSON object available")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path is empty")
	}

	current := r.JsonObj
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		next, err := getJSONPathPart(current, part, path)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

func getJSONPathPart(current *ast.Node, part, fullPath string) (*ast.Node, error) {
	for part != "" {
		if strings.HasPrefix(part, "[") {
			idx, rest, err := parseJSONPathIndex(part)
			if err != nil {
				return nil, fmt.Errorf("path %s invalid array index %q: %w", fullPath, part, err)
			}
			current = current.Index(idx)
			if current == nil {
				return nil, fmt.Errorf("path %s array index %d not found", fullPath, idx)
			}
			part = rest
			continue
		}

		if current.TypeSafe() == ast.V_ARRAY {
			idx, err := strconv.Atoi(part)
			if err == nil {
				current = current.Index(idx)
				if current == nil {
					return nil, fmt.Errorf("path %s array index %d not found", fullPath, idx)
				}
				return current, nil
			}
		}

		bracket := strings.Index(part, "[")
		key := part
		rest := ""
		if bracket >= 0 {
			key = part[:bracket]
			rest = part[bracket:]
		}
		if key == "" {
			return nil, fmt.Errorf("path %s has empty object key", fullPath)
		}
		current = current.Get(key)
		if current == nil {
			return nil, fmt.Errorf("path %s key %s not found", fullPath, key)
		}
		part = rest
	}
	return current, nil
}

func parseJSONPathIndex(part string) (int, string, error) {
	end := strings.Index(part, "]")
	if end < 0 {
		return 0, "", fmt.Errorf("missing closing bracket")
	}
	indexText := strings.TrimSpace(part[1:end])
	if indexText == "" {
		return 0, "", fmt.Errorf("empty index")
	}
	idx, err := strconv.Atoi(indexText)
	if err != nil {
		return 0, "", err
	}
	if idx < 0 {
		return 0, "", fmt.Errorf("negative index")
	}
	return idx, part[end+1:], nil
}

func (r *HttpResp) GetString(path string) (string, error) {
	node, err := r.GetNode(path)
	if err != nil {
		return "", err
	}
	switch node.TypeSafe() {
	case ast.V_STRING:
		v, err := node.String()
		if err != nil {
			return "", err
		}
		return v, nil
	case ast.V_NUMBER:
		v, err := node.Raw()
		if err != nil {
			return "", err
		}
		return string(v), nil
	case ast.V_TRUE, ast.V_FALSE:
		v, err := node.Bool()
		if err != nil {
			return "", err
		}
		if v {
			return "true", nil
		}
		return "false", nil
	default:
		return "", fmt.Errorf("path %s is not a scalar value", path)
	}
}

type BodyType string

const (
	BodyNone      BodyType = ""
	BodyForm      BodyType = "application/x-www-form-urlencoded"
	BodyJSON      BodyType = "application/json"
	BodyMultipart BodyType = "multipart/form-data"
)

type RequestOptions struct {
	Method   string
	URL      string
	Query    map[string]interface{}
	Body     map[string]interface{}
	BodyType BodyType
	Headers  map[string]string
	Timeout  time.Duration
}

func Do(opts RequestOptions) HttpResp {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	method := strings.ToUpper(strings.TrimSpace(opts.Method))
	if method == "" {
		method = fasthttp.MethodGet
	}
	req.Header.SetMethod(method)

	requestURL := BuildRequestURL(opts.URL, opts.Query)
	req.SetRequestURI(requestURL)

	for k, v := range opts.Headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(k, v)
	}

	var response HttpResp
	if len(opts.Body) > 0 && opts.BodyType == BodyNone {
		opts.BodyType = BodyJSON
	}

	switch opts.BodyType {
	case BodyJSON:
		data, err := json.Marshal(opts.Body)
		if err != nil {
			response.Error = err
			return response
		}
		req.SetBody(data)
		req.Header.SetContentType(string(BodyJSON))
	case BodyForm:
		form := url.Values{}
		for k, v := range opts.Body {
			form.Add(k, fmt.Sprint(v))
		}
		req.SetBodyString(form.Encode())
		req.Header.SetContentType(string(BodyForm))
	case BodyMultipart:
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		for k, v := range opts.Body {
			_ = writer.WriteField(k, fmt.Sprint(v))
		}
		_ = writer.Close()
		req.SetBody(buf.Bytes())
		req.Header.SetContentType(writer.FormDataContentType())
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	start := time.Now()
	err := client.DoTimeout(req, resp, timeout)
	duration := time.Since(start)
	if err != nil {
		response.Error = err
		response.Duration = duration
		response.RequestURL = requestURL
		return response
	}

	body := append([]byte(nil), resp.Body()...)
	response = HttpResp{
		StatusCode: resp.StatusCode(),
		StrBody:    string(body),
		Duration:   duration,
		Headers:    responseHeaders(resp),
		RequestURL: requestURL,
	}

	if looksLikeJSON(resp.Header.ContentType(), body) {
		node, err := sonic.GetFromString(response.StrBody)
		if err == nil {
			response.JsonObj = &node
		}
	}

	return response
}

func BuildRequestURL(rawURL string, query map[string]interface{}) string {
	resolvedURL := rawURL
	remainingQuery := map[string]interface{}{}
	for k, v := range query {
		placeholder := fmt.Sprintf("{%s}", k)
		if strings.Contains(resolvedURL, placeholder) {
			resolvedURL = strings.ReplaceAll(resolvedURL, placeholder, fmt.Sprint(v))
			continue
		}
		remainingQuery[k] = v
	}

	if len(remainingQuery) == 0 {
		return resolvedURL
	}

	parsed, err := url.Parse(resolvedURL)
	if err != nil {
		return resolvedURL
	}
	values := parsed.Query()
	for k, v := range remainingQuery {
		values.Set(k, fmt.Sprint(v))
	}
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

func responseHeaders(resp *fasthttp.Response) map[string]string {
	headers := map[string]string{}
	resp.Header.VisitAll(func(key, value []byte) {
		headers[string(key)] = string(value)
	})
	return headers
}

func looksLikeJSON(contentType []byte, body []byte) bool {
	ct := strings.ToLower(string(contentType))
	if strings.Contains(ct, "application/json") || strings.Contains(ct, "+json") {
		return true
	}
	trimmed := strings.TrimSpace(string(body))
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}
