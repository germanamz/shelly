package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/germanamz/shelly/pkg/chatty/chat"
	"github.com/germanamz/shelly/pkg/chatty/message"
	"github.com/germanamz/shelly/pkg/chatty/role"
	"github.com/germanamz/shelly/pkg/providers/model"
	"github.com/germanamz/shelly/pkg/providers/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Completer interface tests ---

// Compile-time interface check: a mock satisfies Completer.
var _ provider.Completer = (*mockCompleter)(nil)

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

// Compile-time interface check: Provider itself satisfies Completer.
var _ provider.Completer = (*provider.Provider)(nil)

// --- Provider struct (base) tests ---

func TestProvider_StubComplete(t *testing.T) {
	var p provider.Provider

	_, err := p.Complete(context.Background(), chat.New())
	assert.EqualError(t, err, "provider: Complete not implemented")
}

func TestNewProvider_DefaultClient(t *testing.T) {
	p := provider.NewProvider("https://api.example.com", provider.Auth{}, model.Model{}, nil)
	assert.Nil(t, p.Client)
}

func TestNewProvider_ModelEmbedding(t *testing.T) {
	m := model.Model{Name: "gpt-4", Temperature: 0.7, MaxTokens: 1024}
	p := provider.NewProvider("https://api.example.com", provider.Auth{}, m, nil)

	assert.Equal(t, "gpt-4", p.Name)
	assert.InDelta(t, 0.7, p.Temperature, 1e-9)
	assert.Equal(t, 1024, p.MaxTokens)
}

func TestNewRequest_BearerAuth(t *testing.T) {
	p := provider.NewProvider("https://api.example.com", provider.Auth{Key: "sk-test"}, model.Model{}, nil)

	req, err := p.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/v1/chat", req.URL.String())
	assert.Equal(t, "Bearer sk-test", req.Header.Get("Authorization"))
}

func TestNewRequest_CustomHeader(t *testing.T) {
	auth := provider.Auth{Key: "sk-test", Header: "x-api-key"}
	p := provider.NewProvider("https://api.example.com", auth, model.Model{}, nil)

	req, err := p.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Equal(t, "sk-test", req.Header.Get("x-api-key"))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestNewRequest_CustomHeaderWithScheme(t *testing.T) {
	auth := provider.Auth{Key: "sk-test", Header: "x-api-key", Scheme: "Token"}
	p := provider.NewProvider("https://api.example.com", auth, model.Model{}, nil)

	req, err := p.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Equal(t, "Token sk-test", req.Header.Get("x-api-key"))
}

func TestNewRequest_NoAuth(t *testing.T) {
	p := provider.NewProvider("https://api.example.com", provider.Auth{}, model.Model{}, nil)

	req, err := p.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
	require.NoError(t, err)
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestNewRequest_ExtraHeaders(t *testing.T) {
	p := provider.NewProvider("https://api.example.com", provider.Auth{}, model.Model{}, nil)
	p.Headers = map[string]string{
		"anthropic-version": "2024-01-01",
		"x-custom":          "value",
	}

	req, err := p.NewRequest(context.Background(), http.MethodGet, "/v1/chat", nil)
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

	p := provider.NewProvider(srv.URL, provider.Auth{}, model.Model{}, srv.Client())

	req, err := p.NewRequest(context.Background(), http.MethodGet, "/ping", nil)
	require.NoError(t, err)

	resp, err := p.Do(req)
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

	p := provider.NewProvider(srv.URL, provider.Auth{Key: "sk-test"}, model.Model{}, srv.Client())

	var dest respBody
	err := p.PostJSON(context.Background(), "/v1/chat", reqBody{Model: "gpt-4"}, &dest)
	require.NoError(t, err)
	assert.Equal(t, "chatcmpl-123", dest.ID)
}

func TestPostJSON_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	p := provider.NewProvider(srv.URL, provider.Auth{}, model.Model{}, srv.Client())

	var dest map[string]string
	err := p.PostJSON(context.Background(), "/v1/chat", map[string]string{"model": "gpt-4"}, &dest)
	assert.ErrorContains(t, err, "unexpected status 401")
}

func TestPostJSON_MarshalError(t *testing.T) {
	p := provider.NewProvider("https://api.example.com", provider.Auth{}, model.Model{}, nil)

	err := p.PostJSON(context.Background(), "/v1/chat", make(chan int), nil)
	assert.ErrorContains(t, err, "marshal payload")
}

func TestPostJSON_NilDest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	p := provider.NewProvider(srv.URL, provider.Auth{}, model.Model{}, srv.Client())

	err := p.PostJSON(context.Background(), "/v1/chat", map[string]string{"model": "gpt-4"}, nil)
	assert.NoError(t, err)
}
