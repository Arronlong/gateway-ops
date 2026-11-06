// Package networkproxy 提供代理地址校验和按请求隔离的 HTTP 代理传输。
package networkproxy

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var ErrInvalidURL = errors.New("代理地址必须是有效的 HTTP/HTTPS URL，可包含端口、用户名和密码")

func Parse(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, ErrInvalidURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || len(raw) > 4096 {
		return nil, ErrInvalidURL
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, ErrInvalidURL
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return nil, ErrInvalidURL
	}
	u.Path, u.RawPath = "", ""
	return u, nil
}

type contextKey struct{}

func WithURL(ctx context.Context, proxyURL *url.URL) context.Context {
	return context.WithValue(ctx, contextKey{}, proxyURL)
}

func NewTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	fallback := transport.Proxy
	// 代理来自当前请求，连接池由 net/http 按代理地址和目标隔离。
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		if u, ok := req.Context().Value(contextKey{}).(*url.URL); ok && u != nil {
			return u, nil
		}
		if fallback != nil {
			return fallback(req)
		}
		return nil, nil
	}
	return transport
}
