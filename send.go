package qbot

import (
	"context"
	"net/url"
	"strings"

	"github.com/go-sdk/core/errx"
)

type sendTextBody struct {
	MessageType     int    `json:"msg_type"`
	Content         string `json:"content"`
	MessageID       string `json:"msg_id,omitempty"`
	EventID         string `json:"event_id,omitempty"`
	MessageSequence uint32 `json:"msg_seq"`
}

// SendText 向指定用户主动发送或被动回复文本；MessageSequence 为零时使用协议默认值 1。
func (c *Client) SendText(ctx context.Context, request SendTextRequest) (*SendTextResponse, error) {
	userID := strings.TrimSpace(request.ToUserID)
	if userID == "" {
		return nil, ErrUserIDRequired
	}
	return c.sendText(ctx, "users", userID, sendTextBody{
		Content:         request.Text,
		MessageID:       strings.TrimSpace(request.MessageID),
		EventID:         strings.TrimSpace(request.EventID),
		MessageSequence: request.MessageSequence,
	})
}

// SendGroupText 向指定群主动发送或被动回复文本；多次回复同一消息时由调用方递增序号。
func (c *Client) SendGroupText(ctx context.Context, request SendGroupTextRequest) (*SendTextResponse, error) {
	groupID := strings.TrimSpace(request.GroupID)
	if groupID == "" {
		return nil, ErrGroupIDRequired
	}
	return c.sendText(ctx, "groups", groupID, sendTextBody{
		Content:         request.Text,
		MessageID:       strings.TrimSpace(request.MessageID),
		EventID:         strings.TrimSpace(request.EventID),
		MessageSequence: request.MessageSequence,
	})
}

func (c *Client) sendText(ctx context.Context, scene, target string, body sendTextBody) (*SendTextResponse, error) {
	if target == "." || target == ".." {
		return nil, ErrInvalidTarget
	}
	if strings.TrimSpace(body.Content) == "" {
		return nil, ErrTextRequired
	}
	if body.MessageID != "" && body.EventID != "" {
		return nil, ErrInvalidReply
	}
	if body.MessageSequence == 0 {
		body.MessageSequence = 1
	}
	requestURL, err := resolveURL(c.baseURL, "v2/"+scene+"/"+url.PathEscape(target)+"/messages", c.allowHTTP)
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	token, err := c.accessToken(requestCtx)
	if err != nil {
		return nil, err
	}
	response, err := c.execute(requestCtx, requestURL, token.AccessToken, body)
	if err != nil {
		return nil, err
	}
	result := &SendTextResponse{}
	if err := decodeResponse(response, result); err != nil {
		return nil, errx.Wrap(err, "send text")
	}
	if strings.TrimSpace(result.MessageID) == "" {
		return nil, errx.Wrap(ErrProtocol, "missing sent message id")
	}
	result.TraceID = response.Header().Get(headerTraceID)
	return result, nil
}
