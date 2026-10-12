package qbot

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-sdk/core/codec/json"
	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/restx"
)

const headerTraceID = "X-Tps-trace-ID"

func (c *Client) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, c.timeout)
}

func resolveURL(baseURL, endpoint string, allowHTTP bool) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrInvalidURL
	}
	if parsed.Scheme != "https" && (!allowHTTP || parsed.Scheme != "http") {
		return "", ErrInvalidURL
	}
	return strings.TrimRight(parsed.String(), "/") + "/" + endpoint, nil
}

func (c *Client) execute(ctx context.Context, requestURL, token string, body any) (*restx.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := json.Marshal[[]byte](body)
	if err != nil {
		return nil, errx.Wrap(ErrProtocol, "encode qq request")
	}
	request := c.cli.R().SetContext(ctx).SetHeader("Content-Type", "application/json").SetBody(data)
	if token != "" {
		request.SetHeader("Authorization", "QQBot "+token)
	}
	response, err := request.Post(requestURL)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrRequest
	}
	return response, nil
}

func decodeResponse(response *restx.Response, target any) error {
	status := response.StatusCode()
	body := bytes.TrimSpace(response.Body())
	wire := struct {
		Code      int    `json:"code"`
		ErrorCode int    `json:"err_code"`
		TraceID   string `json:"trace_id"`
	}{}
	valid := len(body) > 0 && body[0] == '{' && json.Unmarshal(body, &wire) == nil
	code := wire.ErrorCode
	if code == 0 {
		code = wire.Code
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices || valid && code != 0 {
		traceID := response.Header().Get(headerTraceID)
		if traceID == "" {
			traceID = wire.TraceID
		}
		return &APIError{HTTPStatus: status, Code: code, TraceID: traceID}
	}
	if !valid || json.Unmarshal(body, target) != nil {
		return errx.Wrap(ErrProtocol, "decode qq response")
	}
	return nil
}
