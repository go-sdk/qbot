package qbot

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sdk/core/restx"
	"github.com/go-sdk/core/testx"
)

const networkTestEnvironment = "QBOT_NETWORK_TESTS"

// TestNetworkQQAccessToken 使用开放平台凭证验证真实鉴权，不发送消息。
func TestNetworkQQAccessToken(t *testing.T) {
	if os.Getenv(networkTestEnvironment) != "1" {
		t.Skip(networkTestEnvironment + " is not enabled")
	}
	appID := requiredNetworkEnvironment(t, "QBOT_APP_ID")
	appSecret := requiredNetworkEnvironment(t, "QBOT_APP_SECRET")
	client := New(
		WithAppID(appID),
		WithAppSecret(appSecret),
		WithClient(restx.New().SetDebug(false)),
		WithTimeout(30*time.Second),
	)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	result, err := client.GetAccessToken(ctx)
	testx.NoError(t, err)
	testx.True(t, result.AccessToken != "")
	testx.Greater(t, result.ExpiresIn, int64(0))
	testx.True(t, result.ExpiresAt.After(time.Now()))
	t.Logf("token acquired: expires_in=%d expires_at=%s", result.ExpiresIn, result.ExpiresAt.Format(time.RFC3339))
}

func requiredNetworkEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}
