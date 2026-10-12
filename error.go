package qbot

import (
	"fmt"
	"net/http"

	"github.com/go-sdk/core/errx"
)

var (
	ErrAppIDRequired          = errx.New("app id is required")
	ErrAppSecretRequired      = errx.New("app secret is required")
	ErrUserIDRequired         = errx.New("user openid is required")
	ErrGroupIDRequired        = errx.New("group openid is required")
	ErrInvalidTarget          = errx.New("invalid message target")
	ErrTextRequired           = errx.New("text is required")
	ErrInvalidReply           = errx.New("message id and event id are mutually exclusive")
	ErrMessageHandlerRequired = errx.New("message handler is required")
	ErrInvalidURL             = errx.New("invalid api url")
	ErrRequest                = errx.New("qq request failed")
	ErrHTTPStatus             = errx.New("unexpected http status")
	ErrProtocol               = errx.New("qq protocol error")
	ErrUnauthorized           = errx.New("qq authentication failed")
	ErrRateLimited            = errx.New("qq rate limit exceeded")
	ErrInvalidSignature       = errx.New("invalid webhook signature")
)

// APIError 保留服务端状态、业务错误码和追踪 ID，不保留可能包含敏感数据的错误正文。
// errx.Is 可判断错误类别，errx.As 可取得这些元数据。
type APIError struct {
	HTTPStatus int
	Code       int
	TraceID    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("qq api error: status=%d code=%d", e.HTTPStatus, e.Code)
}

func (e *APIError) Unwrap() error {
	var causes []error
	if e.HTTPStatus < http.StatusOK || e.HTTPStatus >= http.StatusMultipleChoices {
		causes = append(causes, ErrHTTPStatus)
	}
	if e.Code != 0 {
		causes = append(causes, ErrProtocol)
	}
	switch e.HTTPStatus {
	case http.StatusUnauthorized:
		causes = append(causes, ErrUnauthorized)
	case http.StatusTooManyRequests:
		causes = append(causes, ErrRateLimited)
	}
	return errx.Join(causes...)
}
