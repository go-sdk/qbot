package qbot

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-sdk/core/codec/json"
	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/testx"
)

func signedWebhookRequest(t *testing.T, secret, body string) *http.Request {
	t.Helper()
	seed := strings.Repeat(secret, ed25519.SeedSize)[:ed25519.SeedSize]
	_, key, err := ed25519.GenerateKey(strings.NewReader(seed))
	testx.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/qqbot", strings.NewReader(body))
	request.Header.Set(headerTimestamp, "1725442341")
	signature := ed25519.Sign(key, []byte("1725442341"+body))
	request.Header.Set(headerSignature, hex.EncodeToString(signature))
	return request
}

func TestWebhookSignatureOfficialVector(t *testing.T) {
	// 固定值来自 botgo 的公开验签测试，确保不只验证同一实现的签名与验签互相匹配。
	seed := strings.Repeat("123456abcdef", 3)[:ed25519.SeedSize]
	key := ed25519.NewKeyFromSeed([]byte(seed))
	publicKey := ed25519.PublicKey(key[ed25519.SeedSize:])
	body := []byte(`{"id":"ROBOT1.0_veoihSEXDc8Q.g-6eLpNIa11bH8MisOjn-m-LKxCPntMk6exUXgcWCGpVO7L2QKTNZzjZzFFDSbiOFcqAPWyVA!!","content":"哦一下","timestamp":"2024-10-15T16:33:15+08:00","author":{"id":"675860273","user_openid":"675860273"}}`)
	header := http.Header{}
	header.Set(headerTimestamp, "1728981195")
	header.Set(headerSignature, "e949b5b94ef4103df903fb031d1d16e358db3db83e79e117edd404c8508be3ce8a76d7bad1bed353194c126a1a5915b4ad8b5288c1191cc53a12acffccd82004")
	testx.NoError(t, verifyWebhookSignature(publicKey, header, body))
	testx.ErrorIs(t, verifyWebhookSignature(publicKey, header, append(body, ' ')), ErrInvalidSignature)
}

func TestWebhookValidationOfficialVector(t *testing.T) {
	// 地址验证回包使用腾讯文档公开样例，不依赖本地签名 helper 生成期望值。
	const secret = "DG5g3B4j9X2KOErG"
	handler, err := New(WithAppSecret(secret)).WebhookHandler(func(context.Context, Message) error {
		t.Fatal("validation must not dispatch a message")
		return nil
	})
	testx.NoError(t, err)
	body := `{"d":{"plain_token":"Arq0D5A61EgUu4OxUvOp","event_ts":"1725442341"},"op":13}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, signedWebhookRequest(t, secret, body))
	testx.Equal(t, http.StatusOK, recorder.Code)
	testx.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assertJSONBody(t, `{"plain_token":"Arq0D5A61EgUu4OxUvOp","signature":"87befc99c42c651b3aac0278e71ada338433ae26fcb24307bdc5ad38c1adc2d01bcfcadc0842edac85e85205028a1132afe09280305f13aa6909ffc2d652c706"}`, recorder.Body.Bytes())
}

func TestWebhookMessageTypes(t *testing.T) {
	for _, eventType := range []EventType{EventC2CMessageCreate, EventGroupATMessageCreate, EventGroupMessageCreate} {
		t.Run(string(eventType), func(t *testing.T) {
			var messages []Message
			handler, err := New(WithAppSecret("test-secret")).WebhookHandler(func(_ context.Context, message Message) error {
				messages = append(messages, message)
				return nil
			})
			testx.NoError(t, err)
			body := `{"op":0,"s":18446744073709551615,"id":"outer-event","t":"` + string(eventType) + `","d":{"id":"inner-message","content":" hello ","author":{"user_openid":"user-1","member_openid":"member-1"},"group_openid":"group-1","attachments":[{"content_type":"image/png","url":"https://media.example.com/image"}]}}`
			request := signedWebhookRequest(t, "test-secret", body)
			request.ContentLength = -1
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			testx.Equal(t, http.StatusOK, recorder.Code)
			assertJSONBody(t, `{"op":12,"d":0}`, recorder.Body.Bytes())
			testx.Len(t, messages, 1)
			message := messages[0]
			testx.Equal(t, "inner-message", message.ID)
			testx.Equal(t, "outer-event", message.EventID)
			testx.Equal(t, eventType, message.EventType)
			testx.Equal(t, ^uint64(0), message.Sequence)
			testx.Equal(t, " hello ", message.Content)
			testx.Equal(t, "group-1", message.GroupID)
			testx.Equal(t, "user-1", message.Author.UserOpenID)
			testx.Len(t, message.Attachments, 1)
		})
	}
}

func TestWebhookHandlerFailure(t *testing.T) {
	handler, err := New(WithAppSecret("test-secret")).WebhookHandler(func(context.Context, Message) error {
		return errx.New("private business details")
	})
	testx.NoError(t, err)
	body := `{"op":0,"t":"C2C_MESSAGE_CREATE","d":{"id":"message-1","author":{"user_openid":"user-1"}}}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, signedWebhookRequest(t, "test-secret", body))
	testx.Equal(t, http.StatusOK, recorder.Code)
	assertJSONBody(t, `{"op":12,"d":1}`, recorder.Body.Bytes())
	testx.NotContains(t, recorder.Body.String(), "private business details")
}

func TestWebhookUnknownEventAndHeartbeat(t *testing.T) {
	handler, err := New(WithAppSecret("test-secret")).WebhookHandler(func(context.Context, Message) error {
		t.Fatal("unknown events and heartbeats must not dispatch messages")
		return nil
	})
	testx.NoError(t, err)
	for _, item := range []struct {
		body     string
		expected string
	}{
		{body: `{"op":0,"t":"FRIEND_ADD","d":{}}`, expected: `{"op":12,"d":0}`},
		{body: `{"op":1,"d":42}`, expected: `{"op":11,"d":42}`},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, signedWebhookRequest(t, "test-secret", item.body))
		testx.Equal(t, http.StatusOK, recorder.Code)
		assertJSONBody(t, item.expected, recorder.Body.Bytes())
	}
}

func TestWebhookRejectsInvalidSignatures(t *testing.T) {
	handler, err := New(WithAppSecret("test-secret")).WebhookHandler(func(context.Context, Message) error {
		t.Fatal("invalid signatures must not dispatch messages")
		return nil
	})
	testx.NoError(t, err)
	body := `{"op":0,"t":"C2C_MESSAGE_CREATE","d":{"id":"message-1","author":{"user_openid":"user-1"}}}`
	for _, item := range []struct {
		name   string
		mutate func(*http.Request)
	}{
		{name: "missing-signature", mutate: func(r *http.Request) { r.Header.Del(headerSignature) }},
		{name: "missing-timestamp", mutate: func(r *http.Request) { r.Header.Del(headerTimestamp) }},
		{name: "invalid-hex", mutate: func(r *http.Request) { r.Header.Set(headerSignature, "not-hex") }},
		{name: "short-signature", mutate: func(r *http.Request) { r.Header.Set(headerSignature, "aa") }},
		{name: "changed-timestamp", mutate: func(r *http.Request) { r.Header.Set(headerTimestamp, "1725442342") }},
		{name: "changed-body", mutate: func(r *http.Request) { r.Body = http.NoBody }},
	} {
		t.Run(item.name, func(t *testing.T) {
			request := signedWebhookRequest(t, "test-secret", body)
			item.mutate(request)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			testx.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
	recorder := httptest.NewRecorder()
	validation := httptest.NewRequest(http.MethodPost, "/qqbot", strings.NewReader(`{"op":13,"d":{"plain_token":"challenge","event_ts":"1725442341"}}`))
	handler.ServeHTTP(recorder, validation)
	testx.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestWebhookMalformedPayloads(t *testing.T) {
	handler, err := New(WithAppSecret("test-secret")).WebhookHandler(func(context.Context, Message) error {
		t.Fatal("invalid payloads must not dispatch messages")
		return nil
	})
	testx.NoError(t, err)
	for _, body := range []string{
		`{`, `null`, `{}`, `{"op":null}`, `{"op":99}`, `{"op":0,"d":{}}`,
		`{"op":1,"d":null}`, `{"op":1,"d":-1}`, `{"op":13,"d":{}}`,
		`{"op":0,"t":"C2C_MESSAGE_CREATE","d":{"id":"message-1"}}`,
		`{"op":0,"t":"GROUP_AT_MESSAGE_CREATE","d":{"id":"message-1"}}`,
		`{"op":0,"t":"GROUP_MESSAGE_CREATE","d":null}`,
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, signedWebhookRequest(t, "test-secret", body))
		testx.Equal(t, http.StatusBadRequest, recorder.Code, body)
	}
}

func TestWebhookMethodAndBodyLimit(t *testing.T) {
	handler, err := New(WithAppSecret("test-secret")).WebhookHandler(func(context.Context, Message) error {
		t.Fatal("invalid requests must not dispatch messages")
		return nil
	})
	testx.NoError(t, err)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/qqbot", nil))
	testx.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
	testx.Equal(t, http.MethodPost, recorder.Header().Get("Allow"))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/qqbot", strings.NewReader(strings.Repeat("x", maxWebhookBody+1))))
	testx.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestWebhookConfiguration(t *testing.T) {
	_, err := New().WebhookHandler(func(context.Context, Message) error { return nil })
	testx.ErrorIs(t, err, ErrAppSecretRequired)
	_, err = New(WithAppSecret("test-secret")).WebhookHandler(nil)
	testx.ErrorIs(t, err, ErrMessageHandlerRequired)
}

func TestWebhookCanceledContext(t *testing.T) {
	handler, err := New(WithAppSecret("test-secret")).WebhookHandler(func(context.Context, Message) error {
		t.Fatal("canceled contexts must not dispatch messages")
		return nil
	})
	testx.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	body := `{"op":0,"t":"C2C_MESSAGE_CREATE","d":{"id":"message-1","author":{"user_openid":"user-1"}}}`
	request := signedWebhookRequest(t, "test-secret", body).WithContext(ctx)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var ack struct {
		Data int `json:"d"`
	}
	testx.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &ack))
	testx.Equal(t, 1, ack.Data)
}
