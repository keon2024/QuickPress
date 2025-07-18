package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/url"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/valyala/fasthttp"
)

// 全局配置
var client = &fasthttp.Client{
	MaxConnsPerHost: 1000,
	ReadTimeout:     3 * time.Second,
	WriteTimeout:    3 * time.Second,
}

type HttpResp struct {
	StatusCode int
	Error      error
	JsonObj    *ast.Node
}

func (r *HttpResp) GetNode(path string) (*ast.Node, error) {
	if r.JsonObj == nil {
		return nil, fmt.Errorf("no JSON object available")
	}
	if !strings.Contains(path, ".") {
		return r.JsonObj.Get(path), nil
	}
	// 按.分割路径
	parts := strings.Split(path, ".")
	value := r.JsonObj.GetByPath(parts)

	return value, nil
}

type BodyType string

const (
	BodyNone      BodyType = "" // no body
	BodyForm      BodyType = "application/x-www-form-urlencoded"
	BodyJSON      BodyType = "application/json"
	BodyMultipart BodyType = "multipart/form-data"
)

type RequestOptions struct {
	Method   string // "GET", "POST"
	URL      string
	Query    map[string]interface{} // All query parameters
	Body     map[string]interface{} // Payload (json, form, multipart)
	BodyType BodyType
	Headers  map[string]string
	Timeout  time.Duration
}

func Do(opts RequestOptions) HttpResp {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	method := strings.ToUpper(opts.Method)
	req.Header.SetMethod(method)

	// 如果 url 上有占位符{id},则根据 query 中参数填充占位符
	if strings.Contains(opts.URL, "{") && len(opts.Query) > 0 {
		for k, v := range opts.Query {
			placeholder := fmt.Sprintf("{%s}", k)
			if strings.Contains(opts.URL, placeholder) {
				opts.URL = strings.ReplaceAll(opts.URL, placeholder, fmt.Sprint(v))
				delete(opts.Query, k) // 删除已替换的参数
			}
		}
	}
	// --- 构造 Query ---
	if len(opts.Query) > 0 {
		q := make([]string, 0)
		for k, v := range opts.Query {
			q = append(q, fmt.Sprintf("%s=%s", k, fmt.Sprint(v)))
		}
		if strings.Contains(opts.URL, "?") {
			opts.URL += "&" + strings.Join(q, "&")
		} else {
			opts.URL += "?" + strings.Join(q, "&")
		}
	}
	req.SetRequestURI(opts.URL)

	// --- 设置 Headers ---
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	var response HttpResp
	// --- 构造 Body ---
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
		writer.Close()
		req.SetBody(buf.Bytes())
		req.Header.SetContentType(writer.FormDataContentType())
	}

	// --- 设置 Timeout ---
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	err := client.DoTimeout(req, resp, timeout)
	if err != nil {
		response.Error = err
		return response
	}
	node, err := sonic.GetFromString(string(resp.Body()))
	if err != nil {
		response.Error = err
		return response
	}

	return HttpResp{
		StatusCode: resp.StatusCode(),
		Error:      nil,
		JsonObj:    &node,
	}

}
