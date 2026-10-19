package server

import (
	"net/http"
	"slices"

	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/http/auth"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
)

// s3BucketRequest exposes only the buckets listed for the supplied S3 key.
func (s *Server) s3BucketRequest(w http.ResponseWriter, r *http.Request, id auth.Identity) {
	op := r.PathValue("operation")
	if r.Method != "GET" || (op != "ListBuckets" && op != "GetBucketInfo") {
		httpresponse.Error(w, "s3 credentials cannot access administration", http.StatusForbidden)
		return
	}

	names, err := s.auth.Buckets(r.Context(), id.Credential)
	if err != nil {
		httpresponse.Error(w, "cannot list accessible buckets", http.StatusBadGateway)
		return
	}

	if op == "GetBucketInfo" {
		name := r.URL.Query().Get("id")
		if !slices.Contains(names, name) {
			httpresponse.Error(w, "bucket not accessible", http.StatusForbidden)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"id": name, "globalAliases": []string{name}, "keys": []any{}})

		return
	}

	result := []map[string]any{}
	for _, name := range names {
		result = append(result, map[string]any{"id": name, "globalAliases": []string{name}})
	}

	writeJSON(w, http.StatusOK, result)
}

// objectService delegates credential scoping to the composition layer.
func (s *Server) objectService(id auth.Identity) (*objects.Service, error) {
	return s.objects.ForIdentity(id)
}
