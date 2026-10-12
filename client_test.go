package qbot

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-sdk/core/codec/json"
	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/restx"
	"github.com/go-sdk/core/testx"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newTestClient(transport roundTripFunc, opts ...Option) *Client {
	cli := restx.New().SetDebug(false).SetTransport(transport)
	return New(append([]Option{WithClient(cli)}, opts...)...)
}

func assertJSONBody(t *testing.T, expected string, body []byte) {
	t.Helper()
	var want, got map[string]any
	testx.NoError(t, json.Unmarshal(expected, &want))
	testx.NoError(t, json.Unmarshal(body, &got))
	testx.Equal(t, want, got)
}

func TestGetAccessToken(t *testing.T) {
	for _, lifetime := range []string{`7200`, `"7200"`} {
		t.Run(lifetime, func(t *testing.T) {
			calls := 0
			client := newTestClient(func(request *http.Request) (*http.Response, error) {
				calls++
				testx.Equal(t, http.MethodPost, request.Method)
				testx.Equal(t, defaultTokenURL, request.URL.String())
				testx.Empty(t, request.Header.Get("Authorization"))
				body, err := io.ReadAll(request.Body)
				testx.NoError(t, err)
				assertJSONBody(t, `{"appId":"test-app","clientSecret":"test-secret"}`, body)
				return response(http.StatusOK, `{"access_token":"test-token","expires_in":`+lifetime+`}`), nil
			}, WithAppID("test-app"), WithAppSecret("test-secret"))

			before := time.Now()
			first, err := client.GetAccessToken(t.Context())
			testx.NoError(t, err)
			testx.Equal(t, int64(7200), first.ExpiresIn)
			testx.True(t, first.ExpiresAt.After(before.Add(7199*time.Second)))
			first.AccessToken = "modified-copy"
			second, err := client.GetAccessToken(t.Context())
			testx.NoError(t, err)
			testx.Equal(t, "test-token", second.AccessToken)
			testx.Equal(t, 1, calls)

			client.tokenMu.Lock()
			client.cachedToken.ExpiresAt = time.Now().Add(30 * time.Second)
			client.tokenMu.Unlock()
			_, err = client.GetAccessToken(t.Context())
			testx.NoError(t, err)
			testx.Equal(t, 2, calls)
		})
	}
}

func TestGetAccessTokenConcurrent(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	client := newTestClient(func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return response(http.StatusOK, `{"access_token":"test-token","expires_in":7200}`), nil
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	}, WithAppID("test-app"), WithAppSecret("test-secret"))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	results := make(chan error, 16)
	var wait sync.WaitGroup
	for range 16 {
		wait.Go(func() {
			token, err := client.GetAccessToken(ctx)
			if err == nil && token.AccessToken != "test-token" {
				err = errx.New("unexpected cached token")
			}
			results <- err
		})
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("token request did not start")
	}
	close(release)
	wait.Wait()
	close(results)
	for err := range results {
		testx.NoError(t, err)
	}
	testx.Equal(t, int32(1), calls.Load())
}

func TestWaitingForTokenCanCancel(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	client := newTestClient(func(request *http.Request) (*http.Response, error) {
		close(started)
		select {
		case <-release:
			return response(http.StatusOK, `{"access_token":"test-token","expires_in":7200}`), nil
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	}, WithAppID("test-app"), WithAppSecret("test-secret"))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	first := make(chan error, 1)
	go func() {
		_, err := client.GetAccessToken(ctx)
		first <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("token request did not start")
	}
	waiter, stop := context.WithTimeout(ctx, 20*time.Millisecond)
	defer stop()
	_, err := client.GetAccessToken(waiter)
	testx.ErrorIs(t, err, context.DeadlineExceeded)
	close(release)
	testx.NoError(t, <-first)
}

func TestTokenValidationAndRecovery(t *testing.T) {
	for _, body := range []string{
		`{"code":100016,"message":"test-secret"}`,
		`{"access_token":"test-token"}`,
		`{"access_token":"test-token","expires_in":0}`,
		`{"access_token":"test-token","expires_in":9223372036854775807}`,
		`{"access_token":"test-token","expires_in":"bad"}`,
		`null`,
	} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			client := newTestClient(func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return response(http.StatusOK, body), nil
				}
				return response(http.StatusOK, `{"access_token":"test-token","expires_in":7200}`), nil
			}, WithAppID("test-app"), WithAppSecret("test-secret"))
			_, err := client.GetAccessToken(t.Context())
			testx.ErrorIs(t, err, ErrProtocol)
			testx.NotContains(t, err.Error(), "test-secret")
			testx.NotContains(t, err.Error(), "test-token")
			_, err = client.GetAccessToken(t.Context())
			testx.NoError(t, err)
			testx.Equal(t, 2, calls)
		})
	}
}

func TestSendText(t *testing.T) {
	client := newTestClient(func(request *http.Request) (*http.Response, error) {
		testx.Equal(t, "/v2/users/user-1/messages", request.URL.Path)
		testx.Equal(t, "QQBot test-token", request.Header.Get("Authorization"))
		body, err := io.ReadAll(request.Body)
		testx.NoError(t, err)
		assertJSONBody(t, `{"msg_type":0,"content":" notice ","msg_id":"incoming","msg_seq":1}`, body)
		result := response(http.StatusOK, `{"id":"outgoing","timestamp":"2026-10-12T12:00:00+08:00","ext_info":{"ref_idx":"ref-1"}}`)
		result.Header.Set(headerTraceID, "trace-1")
		return result, nil
	}, WithToken("test-token"))
	result, err := client.SendText(t.Context(), SendTextRequest{ToUserID: "user-1", Text: " notice ", MessageID: "incoming"})
	testx.NoError(t, err)
	testx.Equal(t, "outgoing", result.MessageID)
	testx.Equal(t, "ref-1", result.ExtInfo.ReferenceIndex)
	testx.Equal(t, "trace-1", result.TraceID)
}

func TestSendGroupTextWithAutomaticToken(t *testing.T) {
	calls := 0
	client := newTestClient(func(request *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			testx.Equal(t, defaultTokenURL, request.URL.String())
			return response(http.StatusOK, `{"access_token":"test-token","expires_in":"7200"}`), nil
		}
		testx.Equal(t, "sandbox.api.sgroup.qq.com", request.URL.Host)
		testx.Equal(t, "/v2/groups/group-1/messages", request.URL.Path)
		testx.Equal(t, "QQBot test-token", request.Header.Get("Authorization"))
		body, err := io.ReadAll(request.Body)
		testx.NoError(t, err)
		assertJSONBody(t, `{"msg_type":0,"content":"notice","event_id":"event-1","msg_seq":2}`, body)
		return response(http.StatusOK, `{"id":"outgoing"}`), nil
	}, WithAppID("test-app"), WithAppSecret("test-secret"), WithSandbox())
	_, err := client.SendGroupText(t.Context(), SendGroupTextRequest{GroupID: "group-1", Text: "notice", EventID: "event-1", MessageSequence: 2})
	testx.NoError(t, err)
	testx.Equal(t, 2, calls)
}

func TestActiveSendAndEscapedTarget(t *testing.T) {
	client := newTestClient(func(request *http.Request) (*http.Response, error) {
		testx.Equal(t, "/prefix/v2/users/user%2F%3F%23/messages", request.URL.EscapedPath())
		testx.Empty(t, request.URL.RawQuery)
		body, err := io.ReadAll(request.Body)
		testx.NoError(t, err)
		assertJSONBody(t, `{"msg_type":0,"content":"notice","msg_seq":1}`, body)
		return response(http.StatusOK, `{"id":"outgoing"}`), nil
	}, WithToken("test-token"), WithBaseURL("https://api.example.com/prefix"))
	_, err := client.SendText(t.Context(), SendTextRequest{ToUserID: "user/?#", Text: "notice"})
	testx.NoError(t, err)
}

func TestAPIError(t *testing.T) {
	for _, item := range []struct {
		name   string
		status int
		body   string
		cause  error
		code   int
	}{
		{name: "business", status: 200, body: `{"err_code":40034005,"message":"test-token","trace_id":"trace-1"}`, cause: ErrProtocol, code: 40034005},
		{name: "legacy", status: 202, body: `{"code":304023,"message":"pending"}`, cause: ErrProtocol, code: 304023},
		{name: "unauthorized", status: 401, body: `{"code":11243}`, cause: ErrUnauthorized, code: 11243},
		{name: "rate-limit", status: 429, body: `{"err_code":40034100}`, cause: ErrRateLimited, code: 40034100},
		{name: "non-json", status: 503, body: "test-token", cause: ErrHTTPStatus},
	} {
		t.Run(item.name, func(t *testing.T) {
			calls := 0
			client := newTestClient(func(*http.Request) (*http.Response, error) {
				calls++
				return response(item.status, item.body), nil
			}, WithToken("test-token"))
			_, err := client.SendText(t.Context(), SendTextRequest{ToUserID: "user-1", Text: "notice"})
			testx.ErrorIs(t, err, item.cause)
			var apiErr *APIError
			testx.True(t, errx.As(err, &apiErr))
			testx.Equal(t, item.status, apiErr.HTTPStatus)
			testx.Equal(t, item.code, apiErr.Code)
			if item.name == "business" {
				testx.Equal(t, "trace-1", apiErr.TraceID)
			}
			testx.NotContains(t, err.Error(), "test-token")
			testx.Equal(t, 1, calls)
		})
	}
}

func TestInvalidSuccessResponses(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `[]`, `{"id":123}`, `{"id":"test-token"`, `{"err_code":"invalid"}`, `{"id":""}`} {
		client := newTestClient(func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, body), nil
		}, WithToken("test-token"))
		_, err := client.SendText(t.Context(), SendTextRequest{ToUserID: "user-1", Text: "notice"})
		testx.ErrorIs(t, err, ErrProtocol)
		testx.NotContains(t, err.Error(), "test-token")
	}
}

func TestValidationBeforeRequests(t *testing.T) {
	client := newTestClient(func(*http.Request) (*http.Response, error) {
		t.Fatal("validation must not send a request")
		return nil, nil
	}, WithToken("test-token"))
	_, err := client.SendText(t.Context(), SendTextRequest{})
	testx.ErrorIs(t, err, ErrUserIDRequired)
	_, err = client.SendGroupText(t.Context(), SendGroupTextRequest{})
	testx.ErrorIs(t, err, ErrGroupIDRequired)
	_, err = client.SendText(t.Context(), SendTextRequest{ToUserID: "user-1", Text: " "})
	testx.ErrorIs(t, err, ErrTextRequired)
	_, err = client.SendText(t.Context(), SendTextRequest{ToUserID: "..", Text: "notice"})
	testx.ErrorIs(t, err, ErrInvalidTarget)
	_, err = client.SendText(t.Context(), SendTextRequest{ToUserID: "user-1", Text: "notice", MessageID: "message-1", EventID: "event-1"})
	testx.ErrorIs(t, err, ErrInvalidReply)
	_, err = New().GetAccessToken(t.Context())
	testx.ErrorIs(t, err, ErrAppIDRequired)
	_, err = New(WithAppID("test-app")).GetAccessToken(t.Context())
	testx.ErrorIs(t, err, ErrAppSecretRequired)
}

func TestBaseURL(t *testing.T) {
	for _, baseURL := range []string{"http://api.example.com", "https://user:password@api.example.com", "https://api.example.com?token=hidden", "https://api.example.com#fragment", "file:///tmp", "://bad"} {
		client := newTestClient(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid URL must not send a request")
			return nil, nil
		}, WithBaseURL(baseURL), WithToken("test-token"))
		_, err := client.SendText(t.Context(), SendTextRequest{ToUserID: "user-1", Text: "notice"})
		testx.ErrorIs(t, err, ErrInvalidURL)
	}
	client := newTestClient(func(request *http.Request) (*http.Response, error) {
		testx.Equal(t, "http", request.URL.Scheme)
		return response(http.StatusOK, `{"id":"outgoing"}`), nil
	}, WithBaseURL("http://api.example.com"), WithToken("test-token"), WithInsecureHTTP())
	_, err := client.SendText(t.Context(), SendTextRequest{ToUserID: "user-1", Text: "notice"})
	testx.NoError(t, err)
}

func TestRequestErrors(t *testing.T) {
	client := newTestClient(func(*http.Request) (*http.Response, error) {
		return nil, errx.New("transport details: test-token")
	}, WithToken("test-token"))
	_, err := client.SendText(t.Context(), SendTextRequest{ToUserID: "user-1", Text: "notice"})
	testx.ErrorIs(t, err, ErrRequest)
	testx.NotContains(t, err.Error(), "test-token")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = client.SendText(ctx, SendTextRequest{ToUserID: "user-1", Text: "notice"})
	testx.ErrorIs(t, err, context.Canceled)
}

func TestTokenAndSendShareDeadline(t *testing.T) {
	var deadline time.Time
	client := newTestClient(func(request *http.Request) (*http.Response, error) {
		current, ok := request.Context().Deadline()
		testx.True(t, ok)
		if request.URL.String() == defaultTokenURL {
			deadline = current
			return response(http.StatusOK, `{"access_token":"test-token","expires_in":7200}`), nil
		}
		testx.Equal(t, deadline, current)
		return response(http.StatusOK, `{"id":"outgoing"}`), nil
	}, WithAppID("test-app"), WithAppSecret("test-secret"), WithTimeout(time.Second))
	_, err := client.SendText(t.Context(), SendTextRequest{ToUserID: "user-1", Text: "notice"})
	testx.NoError(t, err)
}

func TestTokenLifetimeFormats(t *testing.T) {
	for _, value := range []string{`null`, `true`, `-1`, `0`, `1.5`, `"bad"`} {
		var seconds tokenLifetime
		testx.Error(t, json.Unmarshal(value, &seconds))
	}
}
