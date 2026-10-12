// Package qbot 提供 QQ 机器人单聊和群聊的 Access Token 管理、文本发送与 Webhook 接收能力。
//
// SDK 不启动 HTTP 服务，不持久化凭证、事件或消息序号。
// 调用方负责开放平台配置、HTTP 服务生命周期、消息去重及被动回复序号。
// 频道、WebSocket、媒体上传下载和富媒体消息发送不在当前支持范围内。
package qbot
