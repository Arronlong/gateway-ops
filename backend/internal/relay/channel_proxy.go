package relay

import (
	"errors"
	"net/http"

	channelservice "github.com/guliansheng/gateway-ops/internal/channel"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func (s *Service) applyChannelProxy(target *pelicanTarget, channel storage.Channel) error {
	if channel.ProxyID == nil {
		return nil
	}
	resolver := channelservice.Service{Proxies: s.proxies, Cipher: s.cipher}
	proxyURL, err := resolver.ResolveProxyURL(channel.ProxyID)
	if err != nil {
		return err
	}
	base := target.client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	original, ok := base.(*http.Transport)
	if !ok {
		return errors.New("当前传输层不支持渠道代理")
	}
	transport := original.Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	// 渠道明确选择的代理优先；不修改共享客户端或其他账号的传输层。
	target.close()
	target.transport = transport
	target.client.Transport = transport
	target.secrets = append(target.secrets, proxyURL.String())
	if password, ok := proxyURL.User.Password(); ok {
		target.secrets = append(target.secrets, password)
	}
	return nil
}

func (s *Service) keyGroupSessionTarget(target *pelicanTarget, proxyID *uint) (*pelicanTarget, error) {
	lookup := *target
	lookup.transport = nil
	lookup.secrets = append([]string(nil), target.secrets...)
	base := target.client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	original, ok := base.(*http.Transport)
	if !ok {
		if proxyID == nil {
			return &lookup, nil
		}
		return nil, errors.New("当前传输层不支持渠道代理")
	}
	lookup.transport = original.Clone()
	// 查询凭据属于当前会话，不能继承测智目标或前一个渠道的固定代理。
	lookup.transport.Proxy = http.ProxyFromEnvironment
	lookup.client.Transport = lookup.transport
	if err := s.applyChannelProxy(&lookup, storage.Channel{ProxyID: proxyID}); err != nil {
		lookup.close()
		return nil, err
	}
	return &lookup, nil
}
