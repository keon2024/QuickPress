package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/valyala/fasthttp"
	"mime/multipart"
	"net/url"
	"strings"
	"time"
)

// 全局配置
var client = &fasthttp.Client{
	MaxConnsPerHost: 1000,
	ReadTimeout:     3 * time.Second,
	WriteTimeout:    3 * time.Second,
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

func Do(opts RequestOptions) (int, []byte, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	method := strings.ToUpper(opts.Method)
	req.Header.SetMethod(method)

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

	// --- 构造 Body ---
	switch opts.BodyType {
	case BodyJSON:
		data, err := json.Marshal(opts.Body)
		if err != nil {
			return 0, nil, err
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
		return 0, nil, err
	}

	return resp.StatusCode(), append([]byte(nil), resp.Body()...), nil
}
