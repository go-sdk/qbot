package qbot

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"io"
	"net/http"
	"strings"

	"github.com/go-sdk/core/codec/json"
)

const (
	headerSignature = "X-Signature-Ed25519"
	headerTimestamp = "X-Signature-Timestamp"
	maxWebhookBody  = 1 << 20

	opDispatch     = 0
	opHeartbeat    = 1
	opHeartbeatACK = 11
	opCallbackACK  = 12
	opValidation   = 13
)

type webhookPayload struct {
	OPCode   *int       `json:"op"`
	Sequence uint64     `json:"s"`
	Type     EventType  `json:"t"`
	EventID  string     `json:"id"`
	Data     json.Value `json:"d"`
}

// WebhookHandler 创建可挂载到现有 HTTP 服务的处理器，不启动监听。
// 每次请求都先验签，消息处理成功返回 d=0，失败返回 d=1 由平台决定重投。
// HTTP 请求之间可并发处理，调用方负责回调的并发安全和重复事件去重。
func (c *Client) WebhookHandler(handler func(context.Context, Message) error) (http.Handler, error) {
	if c.appSecret == "" {
		return nil, ErrAppSecretRequired
	}
	if handler == nil {
		return nil, ErrMessageHandlerRequired
	}
	seed := c.appSecret
	for len(seed) < ed25519.SeedSize {
		seed = strings.Repeat(seed, 2)
	}
	privateKey := ed25519.NewKeyFromSeed([]byte(seed[:ed25519.SeedSize]))
	publicKey := ed25519.PublicKey(privateKey[ed25519.SeedSize:])
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer func() { _ = request.Body.Close() }()
		body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxWebhookBody))
		if err != nil {
			http.Error(writer, "invalid webhook body", http.StatusBadRequest)
			return
		}
		if err := verifyWebhookSignature(publicKey, request.Header, body); err != nil {
			http.Error(writer, ErrInvalidSignature.Error(), http.StatusUnauthorized)
			return
		}
		payload := webhookPayload{}
		if json.Unmarshal(body, &payload) != nil || payload.OPCode == nil {
			http.Error(writer, "invalid webhook payload", http.StatusBadRequest)
			return
		}
		switch *payload.OPCode {
		case opValidation:
			handleValidation(writer, payload.Data, privateKey)
		case opHeartbeat:
			var sequence *uint64
			if json.Unmarshal([]byte(payload.Data), &sequence) != nil || sequence == nil {
				http.Error(writer, "invalid heartbeat", http.StatusBadRequest)
				return
			}
			writeWebhookACK(writer, opHeartbeatACK, *sequence)
		case opDispatch:
			handleMessage(writer, request, payload, handler)
		default:
			http.Error(writer, "unsupported webhook opcode", http.StatusBadRequest)
		}
	}), nil
}

func verifyWebhookSignature(publicKey ed25519.PublicKey, header http.Header, body []byte) error {
	signature, err := hex.DecodeString(header.Get(headerSignature))
	if err != nil || len(signature) != ed25519.SignatureSize || signature[63]&224 != 0 {
		return ErrInvalidSignature
	}
	timestamp := header.Get(headerTimestamp)
	if timestamp == "" {
		return ErrInvalidSignature
	}
	content := append([]byte(timestamp), body...)
	if !ed25519.Verify(publicKey, content, signature) {
		return ErrInvalidSignature
	}
	return nil
}

func handleValidation(writer http.ResponseWriter, data json.Value, privateKey ed25519.PrivateKey) {
	challenge := struct {
		PlainToken string `json:"plain_token"`
		EventTS    string `json:"event_ts"`
	}{}
	if json.Unmarshal([]byte(data), &challenge) != nil || challenge.PlainToken == "" || challenge.EventTS == "" {
		http.Error(writer, "invalid webhook validation", http.StatusBadRequest)
		return
	}
	content := []byte(challenge.EventTS + challenge.PlainToken)
	response := struct {
		PlainToken string `json:"plain_token"`
		Signature  string `json:"signature"`
	}{PlainToken: challenge.PlainToken, Signature: hex.EncodeToString(ed25519.Sign(privateKey, content))}
	writeWebhookResponse(writer, response)
}

func handleMessage(writer http.ResponseWriter, request *http.Request, payload webhookPayload, handler func(context.Context, Message) error) {
	if payload.Type == "" {
		http.Error(writer, "missing webhook event type", http.StatusBadRequest)
		return
	}
	switch payload.Type {
	case EventC2CMessageCreate, EventGroupATMessageCreate, EventGroupMessageCreate:
	default:
		writeWebhookACK(writer, opCallbackACK, 0)
		return
	}
	message := Message{}
	if json.Unmarshal([]byte(payload.Data), &message) != nil || message.ID == "" {
		http.Error(writer, "invalid webhook message", http.StatusBadRequest)
		return
	}
	if payload.Type == EventC2CMessageCreate && message.Author.UserOpenID == "" || payload.Type != EventC2CMessageCreate && message.GroupID == "" {
		http.Error(writer, "missing webhook message target", http.StatusBadRequest)
		return
	}
	message.EventID = payload.EventID
	message.EventType = payload.Type
	message.Sequence = payload.Sequence
	if request.Context().Err() != nil || handler(request.Context(), message) != nil {
		writeWebhookACK(writer, opCallbackACK, 1)
		return
	}
	writeWebhookACK(writer, opCallbackACK, 0)
}

func writeWebhookACK(writer http.ResponseWriter, opcode int, data uint64) {
	response := struct {
		OPCode int    `json:"op"`
		Data   uint64 `json:"d"`
	}{OPCode: opcode, Data: data}
	writeWebhookResponse(writer, response)
}

func writeWebhookResponse(writer http.ResponseWriter, response any) {
	data, err := json.Marshal[[]byte](response)
	if err != nil {
		http.Error(writer, "encode webhook response failed", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write(data)
}
