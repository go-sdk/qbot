package qbot

import (
	"strings"
	"sync"
	"time"

	"github.com/go-sdk/core/restx"
)

const (
	defaultBaseURL        = "https://api.bot.qq.com"
	sandboxBaseURL        = "https://sandbox.api.sgroup.qq.com"
	defaultTokenURL       = "https://api.bot.qq.com/app/getAppAccessToken"
	defaultRequestTimeout = 15 * time.Second
)

type Option func(*Client)

// Client 提供 QQ 单聊和群聊的鉴权、文本发送及 Webhook 接收能力，不应在创建后复制。
type Client struct {
	cli *restx.Client

	appID     string
	appSecret string
	token     string
	baseURL   string
	timeout   time.Duration
	allowHTTP bool

	tokenMu      sync.Mutex
	cachedToken  AccessTokenResponse
	tokenRefresh chan struct{}
}

// New 创建客户端；凭证只保存在内存中，创建时不发起请求或启动后台协程。
func New(opts ...Option) *Client {
	c := &Client{
		cli:     restx.New(),
		baseURL: defaultBaseURL,
		timeout: defaultRequestTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// WithAppID 配置开放平台分配的机器人 AppID。
func WithAppID(appID string) Option {
	return func(c *Client) {
		c.appID = strings.TrimSpace(appID)
	}
}

// WithAppSecret 配置用于获取 Access Token 和 Webhook 验签的机器人密钥。
func WithAppSecret(secret string) Option {
	return func(c *Client) {
		c.appSecret = strings.TrimSpace(secret)
	}
}

// WithToken 使用调用方管理的 Access Token，优先于 AppID/AppSecret，SDK 不刷新该 Token。
func WithToken(token string) Option {
	return func(c *Client) {
		c.token = strings.TrimSpace(token)
	}
}

// WithClient 注入自定义 restx 客户端，用于调整传输配置或本地测试。
func WithClient(cli *restx.Client) Option {
	return func(c *Client) {
		if cli != nil {
			c.cli = cli
		}
	}
}

// WithTimeout 配置一次公开 API 调用的总超时，包括等待和获取 Token 的时间。
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.timeout = timeout
		}
	}
}

// WithBaseURL 覆盖消息 API 地址；不改变获取 Token 的固定入口。
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/"); baseURL != "" {
			c.baseURL = baseURL
		}
	}
}

// WithSandbox 使用 botgo 定义的沙箱 API 地址，凭证入口仍使用正式环境。
func WithSandbox() Option {
	return WithBaseURL(sandboxBaseURL)
}

// WithInsecureHTTP 允许明文消息 API 地址，仅用于可信的开发或测试环境。
func WithInsecureHTTP() Option {
	return func(c *Client) {
		c.allowHTTP = true
	}
}
