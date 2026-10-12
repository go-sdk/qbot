# 项目地图

## 项目定位

`github.com/go-sdk/qbot` 提供 QQ 单聊和群聊的鉴权、文本发送及 Webhook 接收。使用流程、配置、安全和支持边界以 [README](README.md) 为准。

## 目录结构

```text
qbot/
├── .editorconfig                   编辑器格式规范
├── .github/workflows/golang.yml    持续集成与 Tag Release
├── AGENTS.md                       仓库协作规范
├── client.go                       Client、默认值和 Options
├── token.go                        Access Token 缓存及按需刷新
├── protocol.go                     URL、HTTP 请求和响应错误分类
├── model.go                        消息、凭证和公开请求响应类型
├── send.go                         单聊和群聊文本发送
├── webhook.go                      验签、地址验证、心跳及消息事件确认
├── error.go                        哨兵错误及 APIError 元数据
├── client_test.go                  本地传输替身与缓存并发测试
├── webhook_test.go                 签名向量及本地回调行为测试
├── network_test.go                 环境变量驱动的真实鉴权测试
├── send_network_test.go            显式启用的真实单聊和群聊文本发送测试
├── doc.go                          根包说明
├── README.md                       使用说明及契约唯一入口
├── LICENSE                         Apache-2.0 许可证
├── Makefile                        依赖整理、静态检查和测试入口
├── go.mod                          模块及依赖
└── go.sum                          依赖校验
```

## 调用链

```text
Client.GetAccessToken
    -> 使用 WithToken 提供的凭证
    -> 或检查内存缓存，等待正在进行的刷新
    -> POST 固定入口 /app/getAppAccessToken
    -> 检查 HTTP 状态、code、凭证和有效期

Client.SendText / Client.SendGroupText
    -> 校验目标、文本及回复参数
    -> 解析消息 API 地址
    -> 取得 Token
    -> POST /v2/users/{user_openid}/messages
       或 /v2/groups/{group_openid}/messages
    -> 检查 HTTP 状态、err_code/code 和消息 ID

Client.WebhookHandler
    -> 创建现有 HTTP 服务可挂载的 Handler
    -> 读取有大小限制的原始 Body
    -> Ed25519 验签
    -> op=13: 返回地址验证签名
    -> op=1: 返回心跳确认
    -> op=0: 识别事件，解析消息并调用处理函数
    -> 返回 op=12 的成功或失败确认
```

Token 状态受互斥锁保护，HTTP 请求在锁外执行；等待刷新时通过 Channel 支持 Context 取消。消息确认与重投语义见 [README](README.md#接收消息)。

## 依赖与验证

仅直接依赖 `core`，HTTP、JSON 和错误处理复用其基础能力，Ed25519 使用标准库。无需修改 `core`、`server`、`app` 或 `weclawbot`。

`make lint` 整理依赖并运行 golangci-lint；`go build ./...` 仅编译；`make test` 执行测试，需要明确授权，真实测试默认跳过。真实鉴权和发送测试的入口、参数及副作用见 [README](README.md#真实接口测试)。持续集成沿用相邻 SDK 的依赖检查、静态检查、测试和 Tag Release 流程。
