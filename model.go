package qbot

import "time"

// AccessTokenResponse 返回凭证及其有效期；静态 Token 的有效期字段为零。
// ExpiresIn 是服务端返回的原始有效秒数，读取缓存时应以 ExpiresAt 判断实际到期时间。
type AccessTokenResponse struct {
	AccessToken string    `json:"access_token"`
	ExpiresIn   int64     `json:"expires_in"`
	ExpiresAt   time.Time `json:"-"`
}

// EventType 区分 Webhook 推送的消息场景。
type EventType string

const (
	EventC2CMessageCreate     EventType = "C2C_MESSAGE_CREATE"
	EventGroupATMessageCreate EventType = "GROUP_AT_MESSAGE_CREATE"
	EventGroupMessageCreate   EventType = "GROUP_MESSAGE_CREATE"
)

// User 的不同 OpenID 只适用于对应场景和当前 AppID，不能跨场景或跨机器人互换。
type User struct {
	ID           string `json:"id,omitempty"`
	Username     string `json:"username,omitempty"`
	Bot          bool   `json:"bot,omitempty"`
	UserOpenID   string `json:"user_openid,omitempty"`
	MemberOpenID string `json:"member_openid,omitempty"`
	MemberRole   string `json:"member_role,omitempty"`
}

// MessageAttachment 只保留附件元数据，SDK 不下载媒体或发送富媒体消息。
type MessageAttachment struct {
	URL         string `json:"url,omitempty"`
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

// Message 保留事件内的消息及外层事件标识；ID 与 EventID 分别用于消息回复和事件回复。
type Message struct {
	ID          string              `json:"id"`
	Content     string              `json:"content"`
	Timestamp   string              `json:"timestamp"`
	Author      User                `json:"author"`
	GroupID     string              `json:"group_openid,omitempty"`
	Type        int                 `json:"message_type,omitempty"`
	Attachments []MessageAttachment `json:"attachments,omitempty"`
	EventID     string              `json:"-"`
	EventType   EventType           `json:"-"`
	Sequence    uint64              `json:"-"`
}

// SendTextRequest 配置单聊文本发送；ToUserID 使用入站消息的 author.user_openid。
type SendTextRequest struct {
	ToUserID        string
	Text            string
	MessageID       string
	EventID         string
	MessageSequence uint32
}

// SendGroupTextRequest 配置群聊文本发送；GroupID 使用入站消息的 group_openid。
type SendGroupTextRequest struct {
	GroupID         string
	Text            string
	MessageID       string
	EventID         string
	MessageSequence uint32
}

// MessageExtInfo 保存服务端返回的消息引用索引。
type MessageExtInfo struct {
	ReferenceIndex string `json:"ref_idx,omitempty"`
}

// SendTextResponse 返回服务端消息 ID、发送时间、引用信息及 HTTP 追踪 ID。
type SendTextResponse struct {
	MessageID string         `json:"id"`
	Timestamp string         `json:"timestamp"`
	ExtInfo   MessageExtInfo `json:"ext_info,omitempty"`
	TraceID   string         `json:"-"`
}
