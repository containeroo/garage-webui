package server

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	kit "github.com/containeroo/appkit/auth"
	appkithttpserver "github.com/containeroo/appkit/httpserver"
	applicationgarage "github.com/gi8lino/garage-webui/internal/application/garage"
	"github.com/gi8lino/garage-webui/internal/application/objects"
	"github.com/gi8lino/garage-webui/internal/flags"
	"github.com/gi8lino/garage-webui/internal/garage"
	"github.com/gi8lino/garage-webui/internal/garage/credentials"
	"github.com/gi8lino/garage-webui/internal/http/auth"
	"github.com/gi8lino/garage-webui/internal/postgres"
	"github.com/gi8lino/garage-webui/internal/s3"
	"github.com/gi8lino/garage-webui/web"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAuthConfig selects authentication settings from the integration configuration.
func testAuthConfig(cfg flags.Config) auth.Config {
	return auth.Config{
		PublicURL:      cfg.Server.PublicURL,
		RoutePrefix:    cfg.Server.RoutePrefix,
		Authentication: cfg.Authentication,
		Proxy:          cfg.Proxy,
		OIDC:           cfg.OIDC,
	}
}

// TestLocalFlow exercises authenticated Garage requests against a test database.
func TestLocalFlow(t *testing.T) {
	dsn := os.Getenv("GARAGE_WEBUI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GARAGE_WEBUI_TEST_DATABASE_URL to run the database integration test")
	}

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{fmt.Sprintf("webui_test_%d", time.Now().UnixNano())}.Sanitize()
	_, err = admin.Exec(ctx, `CREATE SCHEMA `+schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`) })
	poolConfig, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, poolConfig)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	mode := kit.ModeLocal
	var cfg flags.Config
	cfg.Authentication.ModeOverride = &mode
	cfg.Server.PublicURL = "http://example.com"
	_, err = db.Exec(ctx, `
  CREATE TABLE webui_users (
    username text PRIMARY KEY,
    password_hash text NOT NULL
  );

  CREATE TABLE webui_sessions (
    token_hash bytea PRIMARY KEY,
    username text NOT NULL,
    expires_at timestamptz NOT NULL,
    mode text NOT NULL DEFAULT ''
  );

  INSERT INTO
    webui_sessions
  VALUES
    (
      'legacy',
      'subject',
      now() + interval '1 hour',
      'oidc'
    )
`)
	require.NoError(t, err)
	require.NoError(t, postgres.New(db).Migrate(ctx, slog.New(slog.NewTextHandler(io.Discard, nil))))
	authentication, err := auth.New(ctx, postgres.New(db), testAuthConfig(cfg))
	require.NoError(t, err)
	var legacyCount int
	require.NoError(t, db.QueryRow(ctx, `
  SELECT
    count(*)
  FROM
    webui_sessions
  WHERE
    mode = 'oidc'
`).Scan(&legacyCount))
	assert.Zero(t, legacyCount, "legacy external sessions must not become local account sessions")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer private-token", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/v2/ListBuckets":
			_, _ = w.Write([]byte(`[]`))
		case "/v2/GetBucketInfo":
			assert.Equal(t, "bucket-id", r.URL.Query().Get("id"))
			_, _ = w.Write([]byte(`{"id":"bucket-id","globalAliases":["backups"]}`))
		case "/v2/CreateKey", "/v2/GetKeyInfo":
			_, _ = w.Write([]byte(`{"accessKeyId":"GKmanaged","secretAccessKey":"S3-private-secret"}`))
		case "/v2/AllowBucketKey", "/v2/DenyBucketKey":
			var grant struct {
				BucketID    string          `json:"bucketId"`
				KeyID       string          `json:"accessKeyId"`
				Permissions map[string]bool `json:"permissions"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&grant))
			assert.Equal(t, "bucket-id", grant.BucketID)
			assert.Equal(t, "GKmanaged", grant.KeyID)
			if r.URL.Path == "/v2/AllowBucketKey" {
				assert.True(t, grant.Permissions["write"])
				assert.False(t, grant.Permissions["owner"])
				assert.False(t, grant.Permissions["read"])
			} else {
				assert.True(t, grant.Permissions["owner"])
				assert.True(t, grant.Permissions["read"])
				assert.False(t, grant.Permissions["write"])
			}
			_, _ = w.Write([]byte(`{}`))
		case "/v2/UpdateBucket":
			assert.Equal(t, "bucket-id", r.URL.Query().Get("id"))
			var updated struct {
				CORSRules []json.RawMessage `json:"corsRules"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&updated))
			require.Len(t, updated.CORSRules, 1)
			assert.Contains(t, string(updated.CORSRules[0]), cfg.Server.PublicURL)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	client, err := garage.New(upstream.URL, "private-token")
	require.NoError(t, err)
	handler := New(applicationgarage.New(client), authentication, nil, "", cfg.Server.PublicURL, "", nil, false, web.Assets, testThemes).Handler()
	loginPage := httptest.NewRecorder()
	handler.ServeHTTP(loginPage, httptest.NewRequest(http.MethodGet, "/login", nil))
	assert.Equal(t, http.StatusOK, loginPage.Code)
	assert.Contains(t, loginPage.Body.String(), `<base href="/">`)
	initial := httptest.NewRecorder()
	handler.ServeHTTP(initial, httptest.NewRequest("GET", "/api/session", nil))
	assert.JSONEq(t, `{"mode":"local","oidcIssuer":"","user":"","role":"","setupRequired":true,"localLogin":true,"uploadsEnabled":false}`, initial.Body.String())
	setupRequest := func(origin, password string) *http.Request {
		req := httptest.NewRequest("POST", "/api/setup", bytes.NewBufferString(fmt.Sprintf(`{"username":"admin","password":%q}`, password)))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		return req
	}
	badOrigin := httptest.NewRecorder()
	handler.ServeHTTP(badOrigin, setupRequest("https://attacker.example", "a-secure-password"))
	assert.Equal(t, http.StatusForbidden, badOrigin.Code)
	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, setupRequest(cfg.Server.PublicURL, "short"))
	assert.Equal(t, http.StatusBadRequest, invalid.Code)
	longPassword := httptest.NewRecorder()
	handler.ServeHTTP(longPassword, setupRequest(cfg.Server.PublicURL, strings.Repeat("x", 73)))
	assert.Equal(t, http.StatusBadRequest, longPassword.Code)
	var wg sync.WaitGroup
	results := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, setupRequest(cfg.Server.PublicURL, "a-secure-password"))
			results <- response
		}()
	}

	wg.Wait()
	close(results)
	statuses := map[int]int{}
	for response := range results {
		statuses[response.Code]++
		if response.Code == http.StatusCreated {
			require.NotEmpty(t, response.Result().Cookies())
			request := httptest.NewRequest("GET", "/api/session", nil)
			request.AddCookie(response.Result().Cookies()[0])
			user, err := authentication.User(request)
			require.NoError(t, err)
			assert.Equal(t, "admin", user)
		}
	}

	assert.Equal(t, 1, statuses[http.StatusCreated])
	assert.Equal(t, 1, statuses[http.StatusConflict])
	registration, err := authentication.Registration(ctx)
	require.NoError(t, err)
	assert.False(t, registration, "local setup must disable automatic external registration")
	var userCount int
	require.NoError(t, db.QueryRow(ctx, `
  SELECT
    count(*)
  FROM
    webui_users
`).Scan(&userCount))
	assert.Equal(t, 1, userCount)
	completed := httptest.NewRecorder()
	handler.ServeHTTP(completed, httptest.NewRequest("GET", "/api/session", nil))
	assert.JSONEq(t, `{"mode":"local","oidcIssuer":"","user":"","role":"","setupRequired":false,"localLogin":true,"uploadsEnabled":false}`, completed.Body.String())
	restarted, err := auth.New(ctx, postgres.New(db), testAuthConfig(cfg))
	require.NoError(t, err)
	needed, err := restarted.SetupRequired(ctx)
	require.NoError(t, err)
	assert.False(t, needed)
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest("GET", "/api/garage/ListBuckets", nil))
	assert.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	login := httptest.NewRequest("POST", "/api/login", bytes.NewBufferString(`{"username":"admin","password":"a-secure-password"}`))
	login.Header.Set("Origin", cfg.Server.PublicURL)
	login.Header.Set("Content-Type", "application/json")
	loggedIn := httptest.NewRecorder()
	handler.ServeHTTP(loggedIn, login)
	require.Equal(t, http.StatusOK, loggedIn.Code)
	require.NotEmpty(t, loggedIn.Result().Cookies())
	cookie := loggedIn.Result().Cookies()[0]
	assert.True(t, cookie.HttpOnly)
	api := httptest.NewRequest("GET", "/api/garage/ListBuckets", nil)
	api.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, api)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `[]`, response.Body.String())

	// The Admin API provisions CORS; browser uploads use only the public S3 origin.
	uploader, err := s3.NewManager(credentials.New(client, postgres.New(db), cfg.Server.PublicURL), "http://localhost:3900", "garage", cfg.Server.PublicURL)
	require.NoError(t, err)
	uploadHandler := New(applicationgarage.New(client), authentication, objects.New(client, uploader), uploader.Origin(), cfg.Server.PublicURL, "", slog.Default(), false, web.Assets, testThemes).Handler()
	uploadRequest := func(origin, body string, authorized bool) *http.Request {
		req := httptest.NewRequest("POST", "/api/uploads/presign", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		if authorized {
			req.AddCookie(cookie)
		}

		return req
	}
	validUpload := `{"bucketId":"bucket-id","key":"backups/archive.zip","size":1024}`
	noSession := httptest.NewRecorder()
	uploadHandler.ServeHTTP(noSession, uploadRequest(cfg.Server.PublicURL, validUpload, false))
	assert.Equal(t, http.StatusUnauthorized, noSession.Code)
	badUploadOrigin := httptest.NewRecorder()
	uploadHandler.ServeHTTP(badUploadOrigin, uploadRequest("https://evil.example", validUpload, true))
	assert.Equal(t, http.StatusForbidden, badUploadOrigin.Code)
	badSize := httptest.NewRecorder()
	uploadHandler.ServeHTTP(badSize, uploadRequest(cfg.Server.PublicURL, `{"bucketId":"bucket-id","key":"a","size":999999999999}`, true))
	assert.Equal(t, http.StatusBadRequest, badSize.Code)
	signed := httptest.NewRecorder()
	uploadHandler.ServeHTTP(signed, uploadRequest(cfg.Server.PublicURL, validUpload, true))
	require.Equal(t, http.StatusOK, signed.Code)
	var signedResponse struct {
		URL string `json:"url"`
	}
	require.NoError(t, json.Unmarshal(signed.Body.Bytes(), &signedResponse))
	address, err := url.Parse(signedResponse.URL)
	require.NoError(t, err)
	assert.Equal(t, "/backups/backups/archive.zip", address.Path)
	assert.NotEmpty(t, address.Query().Get("X-Amz-Signature"))
	assert.NotContains(t, signed.Body.String(), "S3-private-secret")
	assert.Contains(t, signed.Header().Get("Content-Security-Policy"), "http://localhost:3900")

	mutation := httptest.NewRequest("POST", "/api/garage/CreateBucket", bytes.NewBufferString(`{}`))
	mutation.AddCookie(cookie)
	mutation.Header.Set("Origin", "https://attacker.example")
	mutation.Header.Set("Content-Type", "application/json")
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, mutation)
	assert.Equal(t, http.StatusForbidden, blocked.Code)

	proxyMode := kit.ModeProxy
	cfg.Authentication.ModeOverride = &proxyMode
	cfg.Proxy.TrustedCIDRs = []string{"127.0.0.0/8"}
	cfg.Proxy.UsernameHeaders = []string{"X-Forwarded-User"}
	cfg.Proxy.GroupHeaders = []string{"X-Forwarded-Groups"}
	cfg.Proxy.AdminGroup = "garage-admins"
	require.NoError(t, authentication.SetRegistration(ctx, true))
	proxyAuth, err := auth.New(ctx, postgres.New(db), testAuthConfig(cfg))
	require.NoError(t, err)
	needed, err = proxyAuth.SetupRequired(ctx)
	require.NoError(t, err)
	assert.False(t, needed)
	assert.ErrorIs(t, proxyAuth.Setup(httptest.NewRecorder(), setupRequest(cfg.Server.PublicURL, "a-secure-password"), "admin", "a-secure-password"), auth.ErrSetupComplete)
	proxyRequest := httptest.NewRequest("GET", "/api/session", nil)
	proxyRequest.RemoteAddr = "127.0.0.1:1000"
	proxyRequest.Header.Set("X-Forwarded-User", "proxy-user")
	proxyRequest.Header.Set("X-Forwarded-Groups", "garage-admins")
	user, err := proxyAuth.User(proxyRequest)
	require.NoError(t, err)
	assert.Equal(t, "proxy-user", user)
	proxyRequest.RemoteAddr = "192.0.2.1:1000"
	_, err = proxyAuth.User(proxyRequest)
	assert.Error(t, err)
	proxyRequest.RemoteAddr = "127.0.0.1:1000"
	proxyRequest.Header.Set("X-Forwarded-Groups", "other")
	user, role, err := proxyAuth.Authenticate(proxyRequest)
	require.NoError(t, err)
	assert.Equal(t, "proxy-user", user)
	assert.Equal(t, "viewer", role)

	t.Run("user management", func(t *testing.T) {
		call := func(method, path, body string, authenticated bool) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Origin", cfg.Server.PublicURL)
			req.Header.Set("Content-Type", "application/json")
			if authenticated {
				req.AddCookie(cookie)
			}

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			return response
		}
		assert.Equal(t, http.StatusUnauthorized, call("GET", "/api/users", "", false).Code)
		assert.Equal(t, http.StatusUnauthorized, call("GET", "/api/themes", "", false).Code)
		assert.Contains(t, call("GET", "/api/themes", "", true).Body.String(), `"Nord"`)
		assert.Equal(t, http.StatusBadRequest, call("PUT", "/api/profile/theme", `{"theme":"Unknown"}`, true).Code)
		assert.Equal(t, http.StatusNoContent, call("POST", "/api/users", `{"username":"alice","password":"another-secure-password"}`, true).Code)
		assert.Equal(t, http.StatusConflict, call("POST", "/api/users", `{"username":"alice","password":""}`, true).Code)
		aliceLogin := call("POST", "/api/login", `{"username":"alice","password":"another-secure-password"}`, false)
		require.Equal(t, http.StatusOK, aliceLogin.Code)
		require.NotEmpty(t, aliceLogin.Result().Cookies())
		aliceSession := httptest.NewRequest("GET", "/api/session", nil)
		aliceSession.AddCookie(aliceLogin.Result().Cookies()[0])
		aliceUsers := httptest.NewRequest("GET", "/api/users", nil)
		aliceUsers.AddCookie(aliceLogin.Result().Cookies()[0])
		aliceUsersResponse := httptest.NewRecorder()
		handler.ServeHTTP(aliceUsersResponse, aliceUsers)
		assert.Equal(t, http.StatusForbidden, aliceUsersResponse.Code)
		aliceWrite := httptest.NewRequest("POST", "/api/garage/CreateBucket", strings.NewReader(`{}`))
		aliceWrite.AddCookie(aliceLogin.Result().Cookies()[0])
		aliceWrite.Header.Set("Origin", cfg.Server.PublicURL)
		aliceWrite.Header.Set("Content-Type", "application/json")
		aliceWriteResponse := httptest.NewRecorder()
		handler.ServeHTTP(aliceWriteResponse, aliceWrite)
		assert.Equal(t, http.StatusForbidden, aliceWriteResponse.Code)
		assert.Equal(t, http.StatusNoContent, call("PUT", "/api/users/alice/profile", `{"role":"editor","email":"alice@example.com","displayName":"Alice"}`, true).Code)
		assert.Error(t, authentication.ChangeOwnPassword(ctx, "alice", "incorrect-password", "replacement-password"))
		require.NoError(t, authentication.ChangeOwnPassword(ctx, "alice", "another-secure-password", "replacement-password"))
		_, err = authentication.User(aliceSession)
		assert.Error(t, err, "changing the local password must revoke local sessions")
		require.NoError(t, authentication.UpdateOwnTheme(ctx, "alice", "Nord"))
		assert.Equal(t, http.StatusNoContent, call("PUT", "/api/profile/theme", `{"theme":"Dark"}`, true).Code)
		adminProfile, err := authentication.Profile(ctx, "admin")
		require.NoError(t, err)
		assert.Equal(t, "Dark", adminProfile.Theme)
		require.NoError(t, authentication.UpdateOwnProfile(ctx, "alice", "updated@example.com", "Alice Updated"))
		profile, err := authentication.Profile(ctx, "alice")
		require.NoError(t, err)
		assert.Equal(t, "Alice Updated", profile.DisplayName)
		assert.Equal(t, "Nord", profile.Theme)
		assert.True(t, profile.LocalCredentialEnabled)
		assert.Equal(t, http.StatusNoContent, call("PUT", "/api/users/alice/credential", `{"enabled":false}`, true).Code)
		profile, err = authentication.Profile(ctx, "alice")
		require.NoError(t, err)
		assert.False(t, profile.LocalCredentialEnabled)
		assert.Error(t, authentication.Login(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/login", nil), "alice", "replacement-password"))
		assert.Equal(t, http.StatusNoContent, call("PUT", "/api/users/alice/credential", `{"enabled":true}`, true).Code)
		require.NoError(t, authentication.Login(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/login", nil), "alice", "replacement-password"))
		assert.Equal(t, http.StatusNoContent, call("POST", "/api/groups", `{"name":"operators"}`, true).Code)
		assert.Equal(t, http.StatusConflict, call("POST", "/api/groups", `{"name":"operators"}`, true).Code)
		assert.Equal(t, http.StatusNoContent, call("PUT", "/api/users/alice/groups", `{"groups":["operators"]}`, true).Code)
		assert.Equal(t, http.StatusBadRequest, call("PUT", "/api/users/alice/groups", `{"groups":["missing"]}`, true).Code)
		var membership int
		require.NoError(t, db.QueryRow(ctx, `
  SELECT
    count(*)
  FROM
    webui_group_members
  WHERE
    username = 'alice'
    AND group_name = 'operators'
`).Scan(&membership))
		assert.Equal(t, 1, membership, "invalid updates must preserve existing membership")
		assert.Equal(t, http.StatusNoContent, call("DELETE", "/api/groups/operators", `{}`, true).Code)
		assert.Equal(t, http.StatusConflict, call("PUT", "/api/users/admin/profile", `{"role":"viewer","email":"","displayName":""}`, true).Code)
		assert.Equal(t, http.StatusConflict, call("PUT", "/api/users/admin", `{"enabled":false,"password":""}`, true).Code)
		assert.Equal(t, http.StatusConflict, call("DELETE", "/api/users/admin", `{}`, true).Code)
		identity := `{"provider":"proxy","issuer":"","subject":"external-alice"}`
		assert.Equal(t, http.StatusNoContent, call("POST", "/api/users/alice/identities", identity, true).Code)
		assert.Equal(t, http.StatusConflict, call("POST", "/api/users/alice/identities", identity, true).Code)
		proxyRequest.Header.Set("X-Forwarded-Groups", "garage-admins")
		proxyRequest.Header.Set("X-Forwarded-User", "external-alice")
		user, err := proxyAuth.User(proxyRequest)
		require.NoError(t, err)
		assert.Equal(t, "alice", user)
		assert.Equal(t, http.StatusNoContent, call("PUT", "/api/users/alice", `{"enabled":false,"password":""}`, true).Code)
		_, err = proxyAuth.User(proxyRequest)
		assert.Error(t, err)
		_, err = authentication.User(aliceSession)
		assert.Error(t, err)
		assert.Equal(t, http.StatusNoContent, call("PUT", "/api/users/alice", `{"enabled":true,"password":""}`, true).Code)
		_, err = authentication.User(aliceSession)
		assert.Error(t, err, "re-enabling an account must not restore revoked sessions")
		assert.Equal(t, http.StatusNoContent, call("DELETE", "/api/users/alice/identities", identity, true).Code)
		_, err = proxyAuth.User(proxyRequest)
		assert.Error(t, err, "unlinked identities must not automatically re-register")
		assert.Equal(t, http.StatusNoContent, call("POST", "/api/users/alice/identities", identity, true).Code)
		user, err = proxyAuth.User(proxyRequest)
		require.NoError(t, err)
		assert.Equal(t, "alice", user)
		assert.Equal(t, http.StatusNoContent, call("POST", "/api/users/alice/identities/proxy/relink", `{"oldSubject":"external-alice","newSubject":"renamed-alice"}`, true).Code)
		_, err = proxyAuth.User(proxyRequest)
		assert.Error(t, err, "the old proxy identity must remain retired")
		proxyRequest.Header.Set("X-Forwarded-User", "renamed-alice")
		user, err = proxyAuth.User(proxyRequest)
		require.NoError(t, err)
		assert.Equal(t, "alice", user)
		assert.Equal(t, http.StatusNoContent, call("DELETE", "/api/users/alice", `{}`, true).Code)
		_, err = proxyAuth.User(proxyRequest)
		assert.Error(t, err, "deleted identities must not automatically re-register")
		assert.Equal(t, http.StatusNoContent, call("PUT", "/api/registration", `{"enabled":false}`, true).Code)
		proxyRequest.Header.Set("X-Forwarded-User", "unknown")
		_, err = proxyAuth.User(proxyRequest)
		assert.Error(t, err)
		assert.Equal(t, http.StatusNoContent, call("PUT", "/api/registration", `{"enabled":true}`, true).Code)
		proxyRequest.Header.Set("X-Forwarded-User", "admin")
		_, err = proxyAuth.User(proxyRequest)
		assert.Error(t, err, "an external identity must never implicitly bind an existing account")
	})

	t.Run("oidc", func(t *testing.T) { testOIDC(t, ctx, db, cfg) })
}

// testOIDC exercises discovery, PKCE, state and signed group claims.
func testOIDC(t *testing.T, ctx context.Context, db *pgxpool.Pool, cfg flags.Config) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	encode := base64.RawURLEncoding.EncodeToString
	var issuer string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test", "use": "sig", "alg": "RS256", "n": encode(private.N.Bytes()), "e": "AQAB"}}})
		case "/token":
			require.NoError(t, r.ParseForm())
			assert.NotEmpty(t, r.Form.Get("code_verifier"))
			header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test"})
			claims, _ := json.Marshal(map[string]any{"iss": issuer, "sub": "subject", "aud": "client", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "groups": []string{"admins"}, "email": "provider@example.com", "name": "Provider Name"})
			input := encode(header) + "." + encode(claims)
			digest := sha256.Sum256([]byte(input))
			signature, signErr := rsa.SignPKCS1v15(rand.Reader, private, crypto.SHA256, digest[:])
			require.NoError(t, signErr)
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test", "token_type": "Bearer", "id_token": input + "." + encode(signature)})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(provider.Close)
	issuer = provider.URL
	mode := kit.ModeOIDC
	cfg.Authentication.ModeOverride = &mode
	cfg.OIDC.Issuer, cfg.OIDC.ClientID, cfg.OIDC.ClientSecret = issuer, "client", "client-secret"
	cfg.OIDC.GroupClaim, cfg.OIDC.AdminGroup = "groups", "admins"
	cfg.Authentication.LocalLogin = true
	service, err := auth.New(ctx, postgres.New(db), testAuthConfig(cfg))
	require.NoError(t, err)
	recovery := httptest.NewRecorder()
	require.NoError(t, service.Login(recovery, httptest.NewRequest("POST", "/api/login", nil), "admin", "a-secure-password"))
	recoveryRequest := httptest.NewRequest("GET", "/api/session", nil)
	recoveryRequest.AddCookie(recovery.Result().Cookies()[0])
	recoveryUser, err := service.User(recoveryRequest)
	require.NoError(t, err)
	assert.Equal(t, "admin", recoveryUser)
	require.NoError(t, service.CreateGroup(ctx, "oidc-operators"))
	require.NoError(t, service.SetGroupMapping(ctx, auth.GroupMapping{
		ExternalGroup: "admins",
		Group:         "oidc-operators",
	}, false))
	require.NoError(t, service.CreateAccount(ctx, "oidc-account", ""))
	require.NoError(t, service.SetRegistration(ctx, false))
	pendingStart := httptest.NewRecorder()
	require.NoError(t, service.BeginOIDC(pendingStart, httptest.NewRequest("GET", "/auth/login", nil)))
	pendingLocation, err := url.Parse(pendingStart.Header().Get("Location"))
	require.NoError(t, err)
	pendingCallback := httptest.NewRequest("GET", "/auth/callback?code=test&state="+url.QueryEscape(pendingLocation.Query().Get("state")), nil)
	pendingCallback.AddCookie(pendingStart.Result().Cookies()[0])
	assert.Error(t, service.Callback(httptest.NewRecorder(), pendingCallback))
	pending, err := service.PendingIdentities(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "subject", pending[0].Subject)
	require.NoError(t, service.RejectPendingIdentity(ctx, pending[0].Identity))
	pending, err = service.PendingIdentities(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "rejected", pending[0].Status)
	require.NoError(t, service.ReopenPendingIdentity(ctx, pending[0].Identity))
	require.NoError(t, service.LinkIdentity(ctx, "oidc-account", pending[0].Identity))
	pending, err = service.PendingIdentities(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending)
	start := httptest.NewRecorder()
	require.NoError(t, service.BeginOIDC(start, httptest.NewRequest("GET", "/auth/login", nil)))
	location, err := url.Parse(start.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "S256", location.Query().Get("code_challenge_method"))
	state := location.Query().Get("state")
	require.NotEmpty(t, state)
	callback := httptest.NewRequest("GET", "/auth/callback?code=test&state="+url.QueryEscape(state), nil)
	callback.AddCookie(start.Result().Cookies()[0])
	result := httptest.NewRecorder()
	require.NoError(t, service.Callback(result, callback))
	var sessionCookie *http.Cookie
	for _, cookie := range result.Result().Cookies() {
		if cookie.Name == "garage_webui_session" {
			sessionCookie = cookie
		}
	}

	require.NotNil(t, sessionCookie)
	request := httptest.NewRequest("GET", "/api/session", nil)
	request.AddCookie(sessionCookie)
	user, err := service.User(request)
	require.NoError(t, err)
	assert.Equal(t, "oidc-account", user)
	_, role, err := service.Authenticate(request)
	require.NoError(t, err)
	assert.Equal(t, "admin", role, "verified OIDC admin group grants session-scoped elevation")
	localRole, err := service.Role(ctx, "oidc-account")
	require.NoError(t, err)
	assert.Equal(t, "viewer", localRole, "external elevation must not change the local role")
	require.NoError(t, service.SetAccountProfile(ctx, "admin", "oidc-account", "viewer", "local@example.com", "Local Name"))
	require.NoError(t, service.RevertProfile(ctx, "admin", "oidc-account", true, true))
	var profileEmail, profileName string
	require.NoError(t, db.QueryRow(ctx, `
  SELECT
    email,
    display_name
  FROM
    webui_users
  WHERE
    username = 'oidc-account'
`).Scan(&profileEmail, &profileName))
	assert.Equal(t, "provider@example.com", profileEmail)
	assert.Equal(t, "Provider Name", profileName)
	var mapped bool
	require.NoError(t, db.QueryRow(ctx, `
  SELECT
    EXISTS (
      SELECT
        1
      FROM
        webui_oidc_group_members
      WHERE
        username = 'oidc-account'
        AND group_name = 'oidc-operators'
    )
`).Scan(&mapped))
	assert.True(t, mapped)
	assert.Error(t, service.Callback(httptest.NewRecorder(), callback), "state must be single use")
	require.NoError(t, service.UnlinkIdentity(ctx, "oidc-account", auth.Identity{
		Provider: "oidc",
		Issuer:   issuer,
		Subject:  "subject",
	}))
	require.NoError(t, db.QueryRow(ctx, `
  SELECT
    EXISTS (
      SELECT
        1
      FROM
        webui_oidc_group_members
      WHERE
        username = 'oidc-account'
    )
`).Scan(&mapped))
	assert.False(t, mapped, "unlinking an identity removes its external memberships")
	_, err = service.User(request)
	assert.Error(t, err, "unlinking an identity revokes its sessions")
}

// TestFrontendRoutePrefix verifies embedded Vite assets use the requested mount path.
func TestFrontendRoutePrefix(t *testing.T) {
	for _, tc := range []struct {
		prefix string
		base   string
	}{
		{"", `<base href="/">`},
		{"/garage", `<base href="/garage/">`},
		{"/storage/admin", `<base href="/storage/admin/">`},
	} {
		t.Run(tc.base, func(t *testing.T) {
			handler := New(nil, nil, nil, "", "http://example.com", tc.prefix, nil, false, web.Assets, testThemes).Handler()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Contains(t, response.Body.String(), tc.base)
			assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
		})
	}
}

// TestFrontendPrefixRejectsUnknownAPI ensures SPA serving does not swallow API 404s.
func TestFrontendPrefixRejectsUnknownAPI(t *testing.T) {
	handler := New(nil, nil, nil, "", "http://example.com", "/garage", nil, false, web.Assets, testThemes).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/unknown", nil))
	assert.Equal(t, http.StatusNotFound, response.Code)
}

// TestDisabledLocalLoginPage rejects the recovery route when it is unavailable.
func TestDisabledLocalLoginPage(t *testing.T) {
	handler := New(nil, nil, nil, "", "http://example.com", "", nil, false, web.Assets, testThemes).Handler()
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	request.Header.Set("Accept", "text/html")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)
}

// TestRecoveryLoginPage checks recovery route availability for proxy deployments.
func TestRecoveryLoginPage(t *testing.T) {
	mode := kit.ModeProxy
	for _, enabled := range []bool{false, true} {
		cfg := auth.Config{PublicURL: "http://example.com"}
		cfg.Authentication.ModeOverride = &mode
		cfg.Authentication.LocalLogin = enabled
		cfg.Proxy.TrustedCIDRs = []string{"127.0.0.0/8"}
		cfg.Proxy.UsernameHeaders = []string{"X-Forwarded-User"}
		cfg.Proxy.AdminGroup = "admins"
		authentication, err := auth.New(context.Background(), postgres.New(nil), cfg)
		require.NoError(t, err)
		application := New(nil, authentication, nil, "", cfg.PublicURL, "/garage", nil, false, web.Assets, testThemes).Handler()
		handler := appkithttpserver.New(appkithttpserver.Config{
			Application: application,
			RoutePrefix: "/garage",
		}).Application
		request := httptest.NewRequest(http.MethodGet, "/garage/login", nil)
		request.Header.Set("Accept", "text/html")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if enabled {
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Contains(t, response.Body.String(), `<base href="/garage/">`)
		} else {
			assert.Equal(t, http.StatusNotFound, response.Code)
		}
	}
}

// TestAppKitRoutePrefix exercises the same mounting layer used by Kumbuka.
func TestAppKitRoutePrefix(t *testing.T) {
	application := New(nil, nil, nil, "", "http://example.com", "/garage", nil, false, web.Assets, testThemes).Handler()
	handlers := appkithttpserver.New(appkithttpserver.Config{
		Application: application,
		RoutePrefix: "/garage",
	})
	response := httptest.NewRecorder()
	handlers.Application.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/garage/", nil))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `<base href="/garage/">`)

	assets := regexp.MustCompile(`(?:src|href)="(\./assets/[^"]+)"`).FindAllStringSubmatch(response.Body.String(), -1)
	require.NotEmpty(t, assets)
	for _, asset := range assets {
		response := httptest.NewRecorder()
		path := "/garage/" + strings.TrimPrefix(asset[1], "./")
		handlers.Application.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusOK, response.Code, "asset %s", path)
		assert.NotEmpty(t, response.Body.Bytes(), "asset %s", path)
	}
}

// TestFrontendNotFound serves a custom page without changing API or asset errors.
func TestFrontendNotFound(t *testing.T) {
	application := New(nil, nil, nil, "", "http://example.com", "/garage", nil, false, web.Assets, testThemes).Handler()
	handlers := appkithttpserver.New(appkithttpserver.Config{
		Application: application,
		RoutePrefix: "/garage",
	})

	request := httptest.NewRequest(http.MethodGet, "/garage/missing", nil)
	request.Header.Set("Accept", "text/html")
	response := httptest.NewRecorder()
	handlers.Application.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Contains(t, response.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, response.Body.String(), "<h1>Page not found</h1>")
	assert.Contains(t, response.Body.String(), `<base href="/garage/">`)
	assert.Contains(t, response.Body.String(), `href="./"`)

	response = httptest.NewRecorder()
	handlers.Application.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/garage/404.css", nil))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Header().Get("Content-Type"), "text/css")

	for _, path := range []string{"/garage/api/missing", "/garage/assets/missing.js"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if strings.Contains(path, "/api/") {
			request.Header.Set("Accept", "text/html")
		} else {
			request.Header.Set("Accept", "*/*")
		}

		response := httptest.NewRecorder()
		handlers.Application.ServeHTTP(response, request)
		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.NotContains(t, response.Body.String(), "<h1>Page not found</h1>")
	}
}
