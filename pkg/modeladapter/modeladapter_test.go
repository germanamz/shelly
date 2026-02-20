package modeladapter_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/chatty/chat"
	"github.com/germanamz/shelly/pkg/chatty/message"
	"github.com/germanamz/shelly/pkg/chatty/role"
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

func (m *mockCompleter) Complete(_ context.Context, _ *chat.Chat) (message.Message, error) {
	return m.msg, m.err
}

func TestCompleter_Success(t *testing.T) {
	reply := message.NewText("bot", role.Assistant, "hello back")
	p := &mockCompleter{msg: reply}

	c := chat.New(message.NewText("alice", role.User, "hello"))
	got, err := p.Complete(context.Background(), c)

	require.NoError(t, err)
	assert.Equal(t, role.Assistant, got.Role)
	assert.Equal(t, "hello back", got.TextContent())
}

func TestCompleter_Error(t *testing.T) {
	p := &mockCompleter{err: errors.New("api error")}

	c := chat.New(message.NewText("alice", role.User, "hello"))
	_, err := p.Complete(context.Background(), c)

	assert.EqualError(t, err, "api error")
}

// Compile-time interface check: ModelAdapter itself satisfies Completer.
var _ modeladapter.Completer = (*modeladapter.ModelAdapter)(nil)

// --- ModelAdapter struct (base) tests ---

func TestModelAdapter_StubComplete(t *testing.T) {
	var a modeladapter.ModelAdapter

	_, err := a.Complete(context.Background(), chat.New())
	assert.EqualError(t, err, "adapter: Complete not implemented")
}

func TestNew_DefaultClient(t *testing.T) {
	a := modeladapter.New("https://api.example.com", modeladapter.Auth{}, nil)
	assert.Nil(t, a.Client)
}

func TestNew_ModelFields(t *testing.T) {
	a := modeladapter.New("https://api.example.com", modeladapter.Auth{}, nil)
	a.Name = "gpt-4"
	a.Temperature = 0.7
	a.MaxTokens = 1024

	assert.Equal(t, "gpt-4", a.Name)
	assert.InDelta(t, 0.7, a.Temperature, 1e-9)
	assert.Equal(t, 1024, a.MaxTokens)
}

func TestNewRequest_BearerAuth(t *testing.T) {
	a := modeladapter.New("https://api.example.com", modeladapter.Auth{Key: "sk-test"}, nil)

	req, err := a.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/v1/chat", req.URL.String())
	assert.Equal(t, "Bearer sk-test", req.Header.Get("Authorization"))
}

func TestNewRequest_CustomHeader(t *testing.T) {
	auth := modeladapter.Auth{Key: "sk-test", Header: "x-api-key"}
	a := modeladapter.New("https://api.example.com", auth, nil)

	req, err := a.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Equal(t, "sk-test", req.Header.Get("x-api-key"))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestNewRequest_CustomHeaderWithScheme(t *testing.T) {
	auth := modeladapter.Auth{Key: "sk-test", Header: "x-api-key", Scheme: "Token"}
	a := modeladapter.New("https://api.example.com", auth, nil)

	req, err := a.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Equal(t, "Token sk-test", req.Header.Get("x-api-key"))
}

func TestNewRequest_NoAuth(t *testing.T) {
	a := modeladapter.New("https://api.example.com", modeladapter.Auth{}, nil)

	req, err := a.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestNewRequest_ExtraHeaders(t *testing.T) {
	a := modeladapter.New("https://api.example.com", modeladapter.Auth{}, nil)
	a.Headers = map[string]string{
		"anthropic-version": "2024-01-01",
		"x-custom":          "value",
	}

	req, err := a.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
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

	a := modeladapter.New(srv.URL, modeladapter.Auth{}, srv.Client())

	req, err := a.NewRequest(context.Background(), http.MethodGet, "/ping", nil)
	require.NoError(t, err)

	resp, err := a.Do(req)
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

	a := modeladapter.New(srv.URL, modeladapter.Auth{Key: "sk-test"}, srv.Client())

	var dest respBody
	err := a.PostJSON(context.Background(), "/v1/chat", reqBody{Model: "gpt-4"}, &dest)
	require.NoError(t, err)
	assert.Equal(t, "chatcmpl-123", dest.ID)
}

func TestPostJSON_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	a := modeladapter.New(srv.URL, modeladapter.Auth{}, srv.Client())

	var dest map[string]string
	err := a.PostJSON(context.Background(), "/v1/chat", map[string]string{"model": "gpt-4"}, &dest)
	assert.ErrorContains(t, err, "unexpected status 401")
}

func TestPostJSON_MarshalError(t *testing.T) {
	a := modeladapter.New("https://api.example.com", modeladapter.Auth{}, nil)

	err := a.PostJSON(context.Background(), "/v1/chat", make(chan int), nil)
	assert.ErrorContains(t, err, "marshal payload")
}

func TestPostJSON_NilDest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	a := modeladapter.New(srv.URL, modeladapter.Auth{}, srv.Client())

	err := a.PostJSON(context.Background(), "/v1/chat", map[string]string{"model": "gpt-4"}, nil)
	assert.NoError(t, err)
}
