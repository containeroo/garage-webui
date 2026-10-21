package auth

import (
	"net/http"
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
