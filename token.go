package qbot

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/go-sdk/core/errx"
)

type tokenLifetime int64

// UnmarshalJSON 兼容官方文档中数字和字符串两种有效期表示。
func (v *tokenLifetime) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	value := string(data)
	if len(data) > 0 && data[0] == '"' {
		var err error
		value, err = strconv.Unquote(value)
		if err != nil {
			return ErrProtocol
		}
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 || seconds > int64((1<<63-1)/time.Second) {
		return ErrProtocol
	}
	*v = tokenLifetime(seconds)
	return nil
}

// GetAccessToken 返回静态 Token 或缓存凭证，临近过期时按需刷新，不启动后台协程。
// 并发调用共享成功的刷新结果；返回值是副本，调用方不能修改内部缓存。
func (c *Client) GetAccessToken(ctx context.Context) (*AccessTokenResponse, error) {
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	return c.accessToken(requestCtx)
}

func (c *Client) accessToken(ctx context.Context) (*AccessTokenResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.token != "" {
		return &AccessTokenResponse{AccessToken: c.token}, nil
	}
	if c.appID == "" {
		return nil, ErrAppIDRequired
	}
	if c.appSecret == "" {
		return nil, ErrAppSecretRequired
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.tokenMu.Lock()
		margin := min(time.Minute, time.Duration(c.cachedToken.ExpiresIn)*time.Second/10)
		if c.cachedToken.AccessToken != "" && time.Now().Add(margin).Before(c.cachedToken.ExpiresAt) {
			result := c.cachedToken
			c.tokenMu.Unlock()
			return &result, nil
		}
		if pending := c.tokenRefresh; pending != nil {
			c.tokenMu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-pending:
				continue
			}
		}
		pending := make(chan struct{})
		c.tokenRefresh = pending
		c.tokenMu.Unlock()

		result, err := c.fetchAccessToken(ctx)
		c.tokenMu.Lock()
		if err == nil {
			c.cachedToken = *result
		}
		c.tokenRefresh = nil
		close(pending)
		c.tokenMu.Unlock()
		return result, err
	}
}

func (c *Client) fetchAccessToken(ctx context.Context) (*AccessTokenResponse, error) {
	requestedAt := time.Now()
	body := struct {
		AppID        string `json:"appId"`
		ClientSecret string `json:"clientSecret"`
	}{AppID: c.appID, ClientSecret: c.appSecret}
	response, err := c.execute(ctx, defaultTokenURL, "", body)
	if err != nil {
		return nil, err
	}
	wire := struct {
		AccessToken string        `json:"access_token"`
		ExpiresIn   tokenLifetime `json:"expires_in"`
	}{}
	if err := decodeResponse(response, &wire); err != nil {
		return nil, errx.Wrap(err, "get access token")
	}
	if strings.TrimSpace(wire.AccessToken) == "" || wire.ExpiresIn <= 0 {
		return nil, errx.Wrap(ErrProtocol, "missing access token or lifetime")
	}
	expiresAt := requestedAt.Add(time.Duration(wire.ExpiresIn) * time.Second)
	if !time.Now().Before(expiresAt) {
		return nil, errx.Wrap(ErrProtocol, "access token already expired")
	}
	return &AccessTokenResponse{
		AccessToken: wire.AccessToken,
		ExpiresIn:   int64(wire.ExpiresIn),
		ExpiresAt:   expiresAt,
	}, nil
}
