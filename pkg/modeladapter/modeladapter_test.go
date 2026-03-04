package modeladapter_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Completer interface tests ---

// Compile-time interface check: a mock satisfies Completer.
var _ modeladapter.Completer = (*mockCompleter)(nil)

type mockCompleter struct {
	msg message.Message
	err error
}

func (m *mockCompleter) Complete(_ context.Context, _ *chat.Chat, _ []toolbox.Tool) (message.Message, error) {
	return m.msg, m.err
}

func TestCompleter_Success(t *testing.T) {
	reply := message.NewText("bot", role.Assistant, "hello back")
	p := &mockCompleter{msg: reply}

	c := chat.New(message.NewText("alice", role.User, "hello"))
	got, err := p.Complete(context.Background(), c, nil)

	require.NoError(t, err)
	assert.Equal(t, role.Assistant, got.Role)
	assert.Equal(t, "hello back", got.TextContent())
}

func TestCompleter_Error(t *testing.T) {
	p := &mockCompleter{err: errors.New("api error")}

	c := chat.New(message.NewText("alice", role.User, "hello"))
	_, err := p.Complete(context.Background(), c, nil)

	assert.EqualError(t, err, "api error")
}

// --- Client tests ---

func TestNewClient_Defaults(t *testing.T) {
	c := modeladapter.NewClient("https://api.example.com", modeladapter.Auth{})
	assert.Nil(t, c.LastRateLimitInfo())
}

func TestNewRequest_BearerAuth(t *testing.T) {
	c := modeladapter.NewClient("https://api.example.com", modeladapter.Auth{Key: "sk-test"})

	req, err := c.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/v1/chat", req.URL.String())
	assert.Equal(t, "Bearer sk-test", req.Header.Get("Authorization"))
}

func TestNewRequest_CustomHeader(t *testing.T) {
	auth := modeladapter.Auth{Key: "sk-test", Header: "x-api-key"}
	c := modeladapter.NewClient("https://api.example.com", auth)

	req, err := c.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Equal(t, "sk-test", req.Header.Get("x-api-key"))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestNewRequest_CustomHeaderWithScheme(t *testing.T) {
	auth := modeladapter.Auth{Key: "sk-test", Header: "x-api-key", Scheme: "Token"}
	c := modeladapter.NewClient("https://api.example.com", auth)

	req, err := c.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Equal(t, "Token sk-test", req.Header.Get("x-api-key"))
}

func TestNewRequest_NoAuth(t *testing.T) {
	c := modeladapter.NewClient("https://api.example.com", modeladapter.Auth{})

	req, err := c.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestNewRequest_ExtraHeaders(t *testing.T) {
	c := modeladapter.NewClient("https://api.example.com", modeladapter.Auth{},
		modeladapter.WithHeaders(map[string]string{
			"anthropic-version": "2024-01-01",
			"x-custom":          "value",
		}))

	req, err := c.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Equal(t, "2024-01-01", req.Header.Get("anthropic-version"))
	assert.Equal(t, "value", req.Header.Get("x-custom"))
}

func TestDo_Passthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := modeladapter.NewClient(srv.URL, modeladapter.Auth{},
		modeladapter.WithHTTPClient(srv.Client()))

	req, err := c.NewRequest(context.Background(), http.MethodGet, "/ping", nil)
	require.NoError(t, err)

	resp, err := c.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", string(body))
}

func TestPostJSON_Success(t *testing.T) {
	type reqBody struct {
		Model string `json:"model"`
	}
	type respBody struct {
		ID string `json:"id"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var got reqBody
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		assert.Equal(t, "gpt-4", got.Model)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(respBody{ID: "chatcmpl-123"})
	}))
	defer srv.Close()

	c := modeladapter.NewClient(srv.URL, modeladapter.Auth{Key: "sk-test"},
		modeladapter.WithHTTPClient(srv.Client()))

	var dest respBody
	err := c.PostJSON(context.Background(), "/v1/chat", reqBody{Model: "gpt-4"}, &dest)
	require.NoError(t, err)
	assert.Equal(t, "chatcmpl-123", dest.ID)
}

func TestPostJSON_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	c := modeladapter.NewClient(srv.URL, modeladapter.Auth{},
		modeladapter.WithHTTPClient(srv.Client()))

	var dest map[string]string
	err := c.PostJSON(context.Background(), "/v1/chat", map[string]string{"model": "gpt-4"}, &dest)
	assert.ErrorContains(t, err, "unexpected status 401")
}

func TestPostJSON_MarshalError(t *testing.T) {
	c := modeladapter.NewClient("https://api.example.com", modeladapter.Auth{})

	err := c.PostJSON(context.Background(), "/v1/chat", make(chan int), nil)
	assert.ErrorContains(t, err, "marshal payload")
}

func TestPostJSON_NilDest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := modeladapter.NewClient(srv.URL, modeladapter.Auth{},
		modeladapter.WithHTTPClient(srv.Client()))

	err := c.PostJSON(context.Background(), "/v1/chat", map[string]string{"model": "gpt-4"}, nil)
	assert.NoError(t, err)
}

// --- WebSocket tests ---

func wsEchoHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Logf("ws accept: %v", err)
			return
		}
		defer func() { _ = conn.CloseNow() }()

		typ, msg, err := conn.Read(r.Context())
		if err != nil {
			return
		}

		_ = conn.Write(r.Context(), typ, msg)
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
}

func TestDialWS_Success(t *testing.T) {
	srv := httptest.NewServer(wsEchoHandler(t))
	defer srv.Close()

	c := modeladapter.NewClient(srv.URL, modeladapter.Auth{Key: "sk-test"},
		modeladapter.WithHTTPClient(srv.Client()))

	ctx := context.Background()

	conn, resp, err := c.DialWS(ctx, "/ws")
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()

	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	// Verify echo round-trip.
	err = conn.Write(ctx, websocket.MessageText, []byte("hello"))
	require.NoError(t, err)

	typ, msg, err := conn.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, typ)
	assert.Equal(t, "hello", string(msg))
}

func TestDialWS_BearerAuth(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}

		_ = conn.Close(websocket.StatusNormalClosure, "")
	}))
	defer srv.Close()

	c := modeladapter.NewClient(srv.URL, modeladapter.Auth{Key: "sk-test"},
		modeladapter.WithHTTPClient(srv.Client()))

	conn, _, err := c.DialWS(context.Background(), "/ws")
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()

	assert.Equal(t, "Bearer sk-test", gotAuth)
}

func TestDialWS_CustomHeaderAuth(t *testing.T) {
	var gotKey string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}

		_ = conn.Close(websocket.StatusNormalClosure, "")
	}))
	defer srv.Close()

	auth := modeladapter.Auth{Key: "sk-test", Header: "x-api-key"}
	c := modeladapter.NewClient(srv.URL, auth,
		modeladapter.WithHTTPClient(srv.Client()))

	conn, _, err := c.DialWS(context.Background(), "/ws")
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()

	assert.Equal(t, "sk-test", gotKey)
}

func TestDialWS_ExtraHeaders(t *testing.T) {
	var gotCustom string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCustom = r.Header.Get("x-custom")

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}

		_ = conn.Close(websocket.StatusNormalClosure, "")
	}))
	defer srv.Close()

	c := modeladapter.NewClient(srv.URL, modeladapter.Auth{},
		modeladapter.WithHTTPClient(srv.Client()),
		modeladapter.WithHeaders(map[string]string{"x-custom": "value"}))

	conn, _, err := c.DialWS(context.Background(), "/ws")
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()

	assert.Equal(t, "value", gotCustom)
}

func TestDialWS_ConnectionError(t *testing.T) {
	c := modeladapter.NewClient("http://127.0.0.1:1", modeladapter.Auth{})

	_, _, err := c.DialWS(context.Background(), "/ws")
	assert.ErrorContains(t, err, "dial websocket")
}

// --- ParseRetryAfter tests ---

func TestParseRetryAfter_IntegerSeconds(t *testing.T) {
	d := modeladapter.ParseRetryAfter("30")
	assert.Equal(t, 30*time.Second, d)
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	// Use a future date to ensure positive duration.
	future := time.Now().Add(time.Minute).UTC()
	val := future.Format(http.TimeFormat)
	d := modeladapter.ParseRetryAfter(val)
	// Should be roughly 1 minute (±a few seconds for test execution time).
	assert.InDelta(t, time.Minute.Seconds(), d.Seconds(), 5)
}

func TestParseRetryAfter_PastDate(t *testing.T) {
	past := time.Now().Add(-time.Hour).UTC()
	val := past.Format(http.TimeFormat)
	d := modeladapter.ParseRetryAfter(val)
	assert.Equal(t, time.Duration(0), d)
}

func TestParseRetryAfter_Empty(t *testing.T) {
	d := modeladapter.ParseRetryAfter("")
	assert.Equal(t, time.Duration(0), d)
}

func TestParseRetryAfter_Invalid(t *testing.T) {
	d := modeladapter.ParseRetryAfter("not-a-number-or-date")
	assert.Equal(t, time.Duration(0), d)
}

// --- PostJSON stores rate limit info ---

func TestPostJSON_StoresRateLimitInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-ratelimit-remaining-requests", "5")
		w.Header().Set("x-ratelimit-remaining-tokens", "2000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := modeladapter.NewClient(srv.URL, modeladapter.Auth{},
		modeladapter.WithHTTPClient(srv.Client()),
		modeladapter.WithHeaderParser(modeladapter.ParseOpenAIRateLimitHeaders))

	// Before the call, no rate limit info.
	assert.Nil(t, c.LastRateLimitInfo())

	err := c.PostJSON(context.Background(), "/v1/chat", map[string]string{"model": "test"}, nil)
	require.NoError(t, err)

	info := c.LastRateLimitInfo()
	require.NotNil(t, info)
	assert.Equal(t, 5, info.RemainingRequests)
	assert.Equal(t, 2000, info.RemainingTokens)
}
