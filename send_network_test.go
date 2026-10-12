package qbot

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-sdk/core/restx"
	"github.com/go-sdk/core/testx"
)

const (
	sendNetworkTestEnvironment      = "QBOT_SEND_TESTS"
	groupSendNetworkTestEnvironment = "QBOT_GROUP_SEND_TESTS"
)

// TestNetworkQQSendTextFromEnv 使用环境变量指定的凭证和用户发送一条真实单聊文本。
func TestNetworkQQSendTextFromEnv(t *testing.T) {
	if os.Getenv(sendNetworkTestEnvironment) != "1" {
		t.Skip(sendNetworkTestEnvironment + " is not enabled")
	}
	request := SendTextRequest{
		ToUserID:        requiredNetworkEnvironment(t, "QBOT_TO_USER_ID"),
		Text:            networkMessageText(),
		MessageID:       strings.TrimSpace(os.Getenv("QBOT_MESSAGE_ID")),
		EventID:         strings.TrimSpace(os.Getenv("QBOT_EVENT_ID")),
		MessageSequence: networkMessageSequence(t),
	}
	client := networkSendClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	result, err := client.SendText(ctx, request)
	testx.NoError(t, err)
	testx.NotEmpty(t, result.MessageID)
	t.Logf("text sent: message_id=%s trace_id=%s", result.MessageID, result.TraceID)
}

// TestNetworkQQSendGroupTextFromEnv 使用环境变量指定的凭证和群发送一条真实群聊文本。
func TestNetworkQQSendGroupTextFromEnv(t *testing.T) {
	if os.Getenv(groupSendNetworkTestEnvironment) != "1" {
		t.Skip(groupSendNetworkTestEnvironment + " is not enabled")
	}
	request := SendGroupTextRequest{
		GroupID:         requiredNetworkEnvironment(t, "QBOT_GROUP_ID"),
		Text:            networkMessageText(),
		MessageID:       strings.TrimSpace(os.Getenv("QBOT_MESSAGE_ID")),
		EventID:         strings.TrimSpace(os.Getenv("QBOT_EVENT_ID")),
		MessageSequence: networkMessageSequence(t),
	}
	client := networkSendClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	result, err := client.SendGroupText(ctx, request)
	testx.NoError(t, err)
	testx.NotEmpty(t, result.MessageID)
	t.Logf("group text sent: message_id=%s trace_id=%s", result.MessageID, result.TraceID)
}

func networkSendClient(t *testing.T) *Client {
	t.Helper()
	options := []Option{
		WithClient(restx.New().SetDebug(false)),
		WithTimeout(30 * time.Second),
	}
	if token := strings.TrimSpace(os.Getenv("QBOT_ACCESS_TOKEN")); token != "" {
		options = append(options, WithToken(token))
	} else {
		options = append(options, WithAppID(requiredNetworkEnvironment(t, "QBOT_APP_ID")), WithAppSecret(requiredNetworkEnvironment(t, "QBOT_APP_SECRET")))
	}
	if baseURL := strings.TrimSpace(os.Getenv("QBOT_BASE_URL")); baseURL != "" {
		options = append(options, WithBaseURL(baseURL))
	}
	return New(options...)
}

func networkMessageText() string {
	if text := strings.TrimSpace(os.Getenv("QBOT_TEXT")); text != "" {
		return text
	}
	return "测试消息 " + time.Now().Format(time.RFC3339)
}

func networkMessageSequence(t *testing.T) uint32 {
	t.Helper()
	value := strings.TrimSpace(os.Getenv("QBOT_MESSAGE_SEQUENCE"))
	if value == "" {
		return 0
	}
	sequence, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		t.Fatal("QBOT_MESSAGE_SEQUENCE must be an unsigned 32-bit integer")
	}
	return uint32(sequence)
}
