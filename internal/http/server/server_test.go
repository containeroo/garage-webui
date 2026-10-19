package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/containeroo/garage-webui/internal/assembly"
	"github.com/containeroo/garage-webui/internal/http/auth"
	"github.com/containeroo/garage-webui/internal/postgres"
	"github.com/containeroo/garage-webui/web"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGarageCredentialFlow(t *testing.T) {
	dsn := os.Getenv("GARAGE_WEBUI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GARAGE_WEBUI_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	schema := pgx.Identifier{fmt.Sprintf("credential_test_%d", time.Now().UnixNano())}.Sanitize()
	_, err = pool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	defer pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") // nolint:errcheck
	config, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	defer db.Close()
	store := postgres.New(db)
	require.NoError(t, store.Migrate(ctx, slog.Default()))
	revoked := false
	scope := []string{"ListBuckets", "ListAdminTokens", "CreateAdminToken", "UpdateAdminToken", "DeleteAdminToken"}
	calls := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if revoked || r.Header.Get("Authorization") != "Bearer private-admin" {
			w.WriteHeader(401)
			return
		}
		op := strings.TrimPrefix(r.URL.Path, "/v2/")
		calls = append(calls, op)
		w.Header().Set("Content-Type", "application/json")
		if op == "GetCurrentAdminTokenInfo" {
			json.NewEncoder(w).Encode(map[string]any{"name": "alice", "scope": scope}) // nolint:errcheck
			return
		}
		if op == "CreateAdminToken" {
			w.Write([]byte(`{"id":"new","name":"bob","scope":["ListBuckets"],"secretToken":"shown-once"}`)) // nolint:errcheck
			return
		}
		w.Write([]byte(`[]`)) // nolint:errcheck
	}))
	defer upstream.Close()
	s3server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NotContains(t, r.Header.Get("Authorization"), "private-admin")
		if !strings.HasPrefix(r.URL.Query().Get("X-Amz-Credential"), "GKbucket/") {
			w.WriteHeader(403)
			return
		}
		w.Write([]byte(`<ListAllMyBucketsResult><Buckets><Bucket><Name>allowed</Name></Bucket></Buckets></ListAllMyBucketsResult>`)) // nolint:errcheck
	}))
	defer s3server.Close()
	authentication, err := auth.New(store, auth.Config{PublicURL: "http://console.example", GarageURL: upstream.URL, S3URL: s3server.URL, Region: "garage", SessionSecret: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))})
	require.NoError(t, err)
	handler := New(authentication, assembly.New(authentication, store, "http://console.example"), store, s3server.URL, "http://console.example", "", slog.Default(), false, web.Assets, nil).Handler()
	call := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:9000"
		r.Header.Set("Origin", "http://console.example")
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	assert.Equal(t, 401, call("GET", "/api/garage/ListBuckets", "", nil).Code)
	assert.Equal(t, 400, call("POST", "/api/login", `{"username":"old","password":"old"}`, nil).Code)
	login := call("POST", "/api/login", `{"kind":"admin","token":"private-admin"}`, nil)
	require.Equal(t, 204, login.Code)
	cookie := login.Result().Cookies()[0]
	state := call("GET", "/api/session", "", cookie)
	assert.Contains(t, state.Body.String(), `"user":"alice"`)
	assert.NotContains(t, state.Body.String(), "private-admin")
	assert.Equal(t, 200, call("GET", "/api/garage/ListBuckets", "", cookie).Code)
	assert.Equal(t, 403, call("POST", "/api/garage/CreateBucket", `{}`, cookie).Code)
	assert.Equal(t, 200, call("POST", "/api/garage/CreateAdminToken", `{"name":"bob","scope":["ListBuckets"]}`, cookie).Code)
	assert.Contains(t, call("POST", "/api/garage/CreateAdminToken", `{}`, cookie).Body.String(), "shown-once")
	forged := httptest.NewRequest("POST", "/api/garage/CreateAdminToken", strings.NewReader(`{}`))
	forged.AddCookie(cookie)
	forged.Header.Set("Origin", "https://attacker.example")
	forged.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, forged)
	assert.Equal(t, 403, w.Code)
	scope = []string{"ListBuckets"}
	assert.Equal(t, 403, call("GET", "/api/garage/ListAdminTokens", "", cookie).Code)
	revoked = true
	assert.Equal(t, 401, call("GET", "/api/garage/ListBuckets", "", cookie).Code)
	revoked = false
	assert.Equal(t, 204, call("POST", "/api/logout", `{}`, cookie).Code)
	assert.Equal(t, 401, call("GET", "/api/garage/ListBuckets", "", cookie).Code)
	login = call("POST", "/api/login", `{"kind":"s3","accessKeyId":"GKbucket","secretAccessKey":"s3-secret"}`, nil)
	require.Equal(t, 204, login.Code)
	cookie = login.Result().Cookies()[0]
	assert.Equal(t, 200, call("GET", "/api/garage/ListBuckets", "", cookie).Code)
	assert.Contains(t, call("GET", "/api/garage/ListBuckets", "", cookie).Body.String(), "allowed")
	for _, path := range []string{"/api/garage/ListAdminTokens", "/api/garage/ListKeys", "/api/garage/GetClusterStatus", "/api/garage/GetBucketInfo?id=other"} {
		assert.Equal(t, 403, call("GET", path, "", cookie).Code)
	}
	assert.Equal(t, 200, call("GET", "/api/garage/GetBucketInfo?id=allowed", "", cookie).Code)
	assert.Equal(t, 400, call("POST", "/api/objects/presign", `{"bucketId":"other","action":"list"}`, cookie).Code)
	signed := call("POST", "/api/objects/presign", `{"bucketId":"allowed","action":"list"}`, cookie)
	assert.Equal(t, 200, signed.Code)
	assert.Contains(t, signed.Body.String(), "GKbucket")
	assert.NotContains(t, signed.Body.String(), "s3-secret")
	for _, path := range []string{"/api/users", "/api/groups", "/api/registration", "/auth/login"} {
		r := call("GET", path, "", cookie)
		assert.Equal(t, 404, r.Code)
	}
}
