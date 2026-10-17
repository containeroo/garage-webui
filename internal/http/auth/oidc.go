package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
)

// BeginOIDC starts an authorization-code flow with PKCE.
func (s *Service) BeginOIDC(w http.ResponseWriter, r *http.Request) error {
	if s.oauth == nil {
		return errors.New("OIDC is not enabled")
	}

	state, err := random()
	if err != nil {
		return err
	}

	verifier := oauth2.GenerateVerifier()
	hash := sha256.Sum256([]byte(state))
	if err := s.sessions.SaveOIDCState(r.Context(), hash[:], verifier); err != nil {
		return err
	}

	s.cookie(w, "garage_webui_state", state, 600)
	http.Redirect(w, r, s.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), http.StatusFound)
	return nil
}

// Callback validates OIDC state and identity before creating a session.
func (s *Service) Callback(w http.ResponseWriter, r *http.Request) error {
	if s.oauth == nil {
		return errors.New("OIDC is not enabled")
	}

	cookie, err := r.Cookie("garage_webui_state")
	if err != nil || cookie.Value == "" || cookie.Value != r.URL.Query().Get("state") {
		return errors.New("invalid OIDC state")
	}

	s.cookie(w, "garage_webui_state", "", -1)
	hash := sha256.Sum256([]byte(cookie.Value))
	verifier, err := s.sessions.ConsumeOIDCState(r.Context(), hash[:])
	if err != nil {
		return errors.New("expired OIDC state")
	}

	token, err := s.oauth.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
	if err != nil {
		return errors.New("OIDC code exchange failed")
	}

	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return errors.New("OIDC ID token missing")
	}

	id, err := s.verifier.Verify(r.Context(), raw)
	if err != nil {
		return errors.New("OIDC ID token verification failed")
	}

	var claims map[string]any
	if err := id.Claims(&claims); err != nil {
		return errors.New("invalid OIDC claims")
	}

	permitted := false
	externalGroups := groupClaimValues(claims[s.groupClaim])
	for _, name := range externalGroups {
		if name == s.adminGroup && name != "" {
			permitted = true
		}
	}

	username, _ := claims["preferred_username"].(string)
	if username == "" {
		username = id.Subject
	}

	user, err := s.ResolveExternal(r.Context(), "oidc", s.issuer, id.Subject, username)
	if err != nil {
		return fmt.Errorf("resolve OIDC identity: %w", err)
	}

	email, _ := claims["email"].(string)
	displayName, _ := claims["name"].(string)
	if err := s.RefreshExternalProfile(r.Context(), user, Identity{
		Provider: "oidc",
		Issuer:   s.issuer,
		Subject:  id.Subject,
	}, username, email, displayName); err != nil {
		return fmt.Errorf("refresh OIDC profile: %w", err)
	}

	if err := s.SyncOIDCGroups(r.Context(), user, externalGroups); err != nil {
		return fmt.Errorf("synchronize OIDC groups: %w", err)
	}

	return s.session(w, r, user, "", &Identity{
		Provider: "oidc",
		Issuer:   s.issuer,
		Subject:  id.Subject,
	}, permitted)
}

// groupClaimValues accepts the supported OIDC group claim formats.
func groupClaimValues(claim any) []string {
	switch value := claim.(type) {
	case string:
		return []string{value}
	case []any:
		groups := make([]string, 0, len(value))
		for _, item := range value {
			if group, ok := item.(string); ok {
				groups = append(groups, group)
			}
		}
		return groups
	default:
		return nil
	}
}

// random returns a cryptographically random URL-safe value.
func random() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}
