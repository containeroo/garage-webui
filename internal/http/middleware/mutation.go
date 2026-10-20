package middleware

import "net/http"

// Mutation protects JSON writes using the configured external origin.
func (s *Policies) Mutation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && !s.validMutation(w, r) {
			s.logger.Debug("mutation rejected", "event", "mutation_rejected", "method", r.Method)
			return
		}

		next.ServeHTTP(w, r)
	})
}
