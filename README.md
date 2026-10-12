# qbot

`qbot` 是 QQ 机器人单聊和群聊的 Go SDK，提供 Access Token 管理、文本发送及 Webhook 消息接收。

HTTP 契约参考 [腾讯 API v2 文档](https://bot.q.qq.com/wiki/develop/api-v2/)，回调与沙箱地址参考 [tencent-connect/botgo](https://github.com/tencent-connect/botgo/tree/fe31c0dfe469001e0f783d2f07e7de7bd08b403f)。项目结构见 [项目地图](PROJECT_MAP.md)。

## 环境与安装

需要 Go 1.27 或更高版本：

```bash
go get github.com/go-sdk/qbot
```

机器人需在开放平台创建，并由调用方完成沙箱成员、接口权限、出口 IP 白名单、HTTPS 回调地址和事件监听配置。SDK 不修改开放平台配置。

## 创建客户端

```go
client := qbot.New(
	qbot.WithAppID(appID),
	qbot.WithAppSecret(appSecret),
	qbot.WithTimeout(15*time.Second),
)
```

`New` 不发送请求，也不启动后台协程。每次发送前自动取得 Token；同一 Client 的并发调用共享成功的刷新结果，等待刷新的调用可以通过 Context 取消。缓存通常在到期前 60 秒刷新；有效期不足 600 秒时，刷新提前量为有效期的十分之一。凭证仅保存在内存中。

需要提前检查凭证时可调用：

```go
token, err := client.GetAccessToken(ctx)
if err != nil {
	return err
}
// token.ExpiresAt 是缓存凭证的实际到期时间，不应打印 token.AccessToken。
_ = token.ExpiresAt
```

`GetAccessToken` 返回副本。`ExpiresIn` 是服务端最初返回的有效秒数，并非读取缓存时的剩余秒数。SDK 无损兼容其 JSON 数字与字符串表示。

由应用统一管理 Token 时，使用 `WithToken(accessToken)`；该配置优先于 AppID/AppSecret，SDK 不刷新静态 Token，其有效期字段为零。创建 Webhook Handler 时仍需 AppSecret。

## 发送文本

单聊使用 `author.user_openid`，群聊使用 `group_openid`；不要把群成员的 `member_openid` 用作单聊目标，不同 AppID 的 OpenID 也不能互换。

回复单聊消息：

```go
result, err := client.SendText(ctx, qbot.SendTextRequest{
	ToUserID:        message.Author.UserOpenID,
	Text:            "收到",
	MessageID:       message.ID,
	MessageSequence: 1,
})
if err != nil {
	return err
}
_ = result.MessageID
```

回复群聊消息：

```go
result, err := client.SendGroupText(ctx, qbot.SendGroupTextRequest{
	GroupID:         message.GroupID,
	Text:            "收到",
	MessageID:       message.ID,
	MessageSequence: 1,
})
if err != nil {
	return err
}
_ = result.MessageID
```

`MessageID` 对应 `msg_id`，取入站消息内层的 `d.id`；`EventID` 对应 `event_id`，取外层事件 `id`。二者互斥。只有平台支持回复的事件才能使用 `EventID`，普通消息回复应使用 `MessageID`。

`MessageSequence` 为零时发送 1。对同一消息多次回复时，由调用方递增序号；同一个 `msg_id + msg_seq` 重复发送会被平台去重。SDK 不分配或持久化回复序号。

不填写 `MessageID` 和 `EventID` 即为主动发送，仍受机器人权限、用户或群的主动消息开关及配额限制。被动回复也受服务端时效和次数限制；SDK 不在本地硬编码这些平台规则。文本保持原始内容，空白文本会被拒绝。

成功响应包含 `MessageID`、`Timestamp`、`ExtInfo.ReferenceIndex` 和 HTTP `TraceID`。SDK 检查 HTTP 状态和业务码，并要求发送响应含消息 ID；请求失败后不自动刷新重发，不自动重试消息发送。

## 接收消息

创建处理器并挂载到现有 HTTP 服务：

```go
handler, err := client.WebhookHandler(func(ctx context.Context, message qbot.Message) error {
	if message.Content == "" {
		return nil
	}
	switch message.EventType {
	case qbot.EventC2CMessageCreate:
		_, err := client.SendText(ctx, qbot.SendTextRequest{
			ToUserID:  message.Author.UserOpenID,
			Text:      "收到",
			MessageID: message.ID,
		})
		return err
	case qbot.EventGroupATMessageCreate, qbot.EventGroupMessageCreate:
		_, err := client.SendGroupText(ctx, qbot.SendGroupTextRequest{
			GroupID:   message.GroupID,
			Text:      "收到",
			MessageID: message.ID,
		})
		return err
	}
	return nil
})
if err != nil {
	return err
}
mux.Handle("/qqbot", handler)
```

代码中的 `ctx`、凭证、`message` 和 `mux` 由应用提供。示例展示接口衔接，应用应在回复前实现去重，避免重投时重复产生业务副作用。

Handler 支持以下消息事件：

| 事件                      | 常量                        | 场景                           |
|---------------------------|-----------------------------|--------------------------------|
| `C2C_MESSAGE_CREATE`      | `EventC2CMessageCreate`     | QQ 单聊                        |
| `GROUP_AT_MESSAGE_CREATE` | `EventGroupATMessageCreate` | 群内 @ 机器人                  |
| `GROUP_MESSAGE_CREATE`    | `EventGroupMessageCreate`   | 群全量消息，需要平台权限及配置 |

每次请求都先验证 `X-Signature-Ed25519` / `X-Signature-Timestamp` 和原始 Body，包括 `op=13` 的地址验证。验签失败返回 HTTP 401；非法 JSON、缺少必要消息字段或不支持的 Opcode 返回 HTTP 400。Body 上限为 1 MiB，不依赖 `Content-Length`。

地址验证通过后返回 `plain_token` 及其签名；心跳返回 `op=11`。消息处理函数返回 `nil` 时返回 `{"op":12,"d":0}`，返回错误时返回 `{"op":12,"d":1}`，由平台决定是否重投。其他事件类型验签后直接成功确认，不调用消息处理函数。回调响应不包含业务处理错误详情，调用方需要在自己的处理函数中记录适当的错误元数据。

HTTP 请求之间可能并发处理，同一事件或消息也可能重复投递。SDK 不串行化、不持久化、不去重，不承诺恰好处理一次；由应用结合 `EventID`、消息 `ID` 及回复序号管理幂等性。SDK 不启动 HTTP 服务或后台消费任务。耗时业务应在应用可靠接收后交给其队列处理，再返回成功确认。

## Client 配置

- 默认消息 API：`https://api.bot.qq.com`。
- Token 固定入口：`https://api.bot.qq.com/app/getAppAccessToken`，不受消息地址覆盖影响。
- `WithSandbox()` 使用 botgo 定义的 `https://sandbox.api.sgroup.qq.com`；Token 入口不变，平台沙箱是否可用需以当前账号配置及真实调用为准。
- `WithBaseURL(url)` 覆盖消息地址；与 `WithSandbox()` 同时使用时，后传入的地址配置生效。
- `WithTimeout` 默认 15 秒，覆盖一次 API 调用的总预算，包括等待 Token、刷新和消息请求，调用方更短的 Context 优先生效。
- `WithClient` 复用自定义 `core/restx.Client` 的代理、传输和调试配置。SDK 使用前应完成配置，使用期间不要并发修改该客户端或重新应用 Options。
- 默认地址只接受 HTTPS；可信开发环境可以显式使用 `WithInsecureHTTP()` 允许明文消息 API。

SDK 不配置自动重试；通过自定义 restx 客户端启用的重试、请求 Hook 和调试日志由调用方负责。调试可能暴露 Authorization、AppSecret、消息和媒体链接，应根据应用的敏感信息策略控制。

## 错误处理

通过 `core/errx` 判断错误类别和读取服务端元数据：

```go
switch {
case errx.Is(err, qbot.ErrUnauthorized):
	// HTTP 401；调用方检查或更新凭证。
case errx.Is(err, qbot.ErrRateLimited):
	// HTTP 429；调用方决定退避。
case errx.Is(err, qbot.ErrHTTPStatus):
	// 非 2xx HTTP 状态。
case errx.Is(err, qbot.ErrProtocol):
	// 非零业务错误码、非法 JSON 或缺少必要响应字段。
case errx.Is(err, qbot.ErrRequest):
	// DNS、连接或 TLS 等请求失败。
}

var apiErr *qbot.APIError
if errx.As(err, &apiErr) {
	// 可以读取 apiErr.HTTPStatus、apiErr.Code 和 apiErr.TraceID。
}
```

Token 接口检查 `code`，消息接口兼容 `err_code` 和 `code`。业务错误可能发生在 HTTP 200，判断业务分支时应读取 `APIError.Code`；`ErrUnauthorized` / `ErrRateLimited` 仅映射对应 HTTP 状态。APIError 可同时匹配 HTTP 状态和业务错误类别。

Context 取消或超时直接返回对应 Context 错误。SDK 的错误不包含凭证、消息、原始请求 URL 查询信息、服务端错误正文或完整响应。

## 支持边界

- 支持 Access Token 缓存及按需刷新、调用方管理的静态 Token、沙箱和自定义消息地址。
- 支持单聊和群聊纯文本主动发送、消息回复和平台允许的事件回复。
- 支持 Webhook 地址验证、验签、心跳和上述三种消息事件。
- 保留消息附件元数据，不下载、上传或发送媒体。
- 不实现频道、频道私信、WebSocket、Markdown、键盘、消息撤回及其他管理 API。
- 不持久化凭证和消息，不管理 HTTP 服务生命周期、平台配置、事件去重、回复序号或发送配额。

## 开发

```bash
make lint
go build ./...
```

本地测试通过以下命令显式执行：

```bash
make test
```

本地测试使用自定义 HTTP Transport 及 `httptest.NewRecorder`，不访问真实 QQ 接口，也不启动监听服务。`make test` 和直接运行 `go test` 都会继承环境变量；真实测试默认跳过，只有对应环境变量开关为 `1` 时才执行。静态检查、编译和本地测试均不能证明真实账号鉴权、平台回调、主动消息权限或发送配额。

### 真实接口测试

真实测试不启动服务、不测试 Webhook 回调。以下三个入口各自只执行对应测试：

| 命令                   | 开关                      | 实际行为                          |
|------------------------|---------------------------|-----------------------------------|
| `make test-network`    | `QBOT_NETWORK_TESTS=1`    | 获取真实 Access Token，不发送消息 |
| `make test-send`       | `QBOT_SEND_TESTS=1`       | 向指定用户发送一条文本            |
| `make test-send-group` | `QBOT_GROUP_SEND_TESTS=1` | 向指定群发送一条文本              |

鉴权测试必须提供开放平台 AppID 和 AppSecret，即使已有 `QBOT_ACCESS_TOKEN` 也会验证凭证获取：

```bash
export QBOT_APP_ID="<AppID>"
export QBOT_APP_SECRET="<AppSecret>"

make test-network
```

发送测试使用已有 `QBOT_ACCESS_TOKEN`，为空时使用 `QBOT_APP_ID` / `QBOT_APP_SECRET` 自动获取。下列命令会分别发送真实消息：

```bash
export QBOT_ACCESS_TOKEN="<可选，已有 Access Token>"
export QBOT_TO_USER_ID="<单聊用户 user_openid>"

make test-send
```

```bash
export QBOT_GROUP_ID="<群 group_openid>"

make test-send-group
```

可选发送参数：

| 环境变量                | 默认行为                                                                 |
|-------------------------|--------------------------------------------------------------------------|
| `QBOT_TEXT`             | 空值时发送 `测试消息 ` 加本地 RFC3339 时间                               |
| `QBOT_BASE_URL`         | 空值时使用默认正式环境；沙箱可设置为 `https://sandbox.api.sgroup.qq.com` |
| `QBOT_MESSAGE_ID`       | 被动回复的消息 ID，空值时不携带                                          |
| `QBOT_EVENT_ID`         | 平台允许回复的事件 ID，与 `QBOT_MESSAGE_ID` 互斥                         |
| `QBOT_MESSAGE_SEQUENCE` | 无符号 32 位整数；空值或零使用 SDK 默认序号 1                            |

不提供消息 ID 和事件 ID 时执行主动发送，受平台权限、主动消息开关和配额限制。被动回复应提供当前目标的有效消息或事件 ID；再次回复同一消息时需要递增序号。

测试使用 30 秒超时，显式关闭 HTTP 调试，鉴权成功只记录有效期，发送成功只记录返回的消息 ID 和追踪 ID，不输出凭证、目标 OpenID 或文本内容。测试不会重试发送；请仅在准备好真实凭证和目标后显式执行。
