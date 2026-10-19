package auth

import (
	"net/http"
	"net/url"
)

// cookie writes an HTTP-only authentication cookie.
func (s *Service) cookie(w http.ResponseWriter, name, value string, age int) {
	path := s.prefix + "/"
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   age,
	})
}

// SameOrigin checks a request's Origin against the configured public URL.
func SameOrigin(r *http.Request, publicURL string) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || origin.Host == "" {
		return false
	}

	base, err := url.Parse(publicURL)

	return err == nil && origin.Scheme == base.Scheme && origin.Host == base.Host
}
