package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransferStreamsBeforeCompletionAndCancelsUpstream(t *testing.T) {
	t.Parallel()
	stopped := make(chan struct{})
	cancelled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Cookie"))
		assert.Empty(t, r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, strings.Repeat("x", 64<<10))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-stopped:
		}
	}))
	defer upstream.Close()
	defer close(stopped)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target, err := url.Parse(upstream.URL)
		require.NoError(t, err)
		handler := newObjectStream(transferPlan{result: objects.Result{Method: http.MethodGet}, target: target}, upstream.Client().Transport, logger)
		handler.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", proxy.URL, nil)
	require.NoError(t, err)
	request.Header.Set("Cookie", "private-session")
	request.Header.Set("Authorization", "private-token")
	response, err := proxy.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close() //nolint:errcheck
	first := make([]byte, 1)
	_, err = io.ReadFull(response.Body, first)
	require.NoError(t, err, "the backend must deliver bytes while the upstream is still open")
	assert.Equal(t, byte('x'), first[0])
	require.NoError(t, response.Body.Close())
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("client disconnect did not cancel upstream")
	}
}

func TestTransferOutlivesAPIServerWriteDeadline(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(150 * time.Millisecond):
			_, _ = io.WriteString(w, "completed")
		case <-r.Context().Done():
		}
	}))
	defer upstream.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target, err := url.Parse(upstream.URL)
		require.NoError(t, err)
		handler := newObjectStream(transferPlan{result: objects.Result{Method: http.MethodGet}, target: target}, upstream.Client().Transport, logger)
		handler.ServeHTTP(w, r)
	}))
	proxy.Config.WriteTimeout = 50 * time.Millisecond
	proxy.Start()
	defer proxy.Close()
	response, err := proxy.Client().Get(proxy.URL)
	require.NoError(t, err)
	defer response.Body.Close() //nolint:errcheck
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, "completed", string(data))
}
