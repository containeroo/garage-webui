package server

import (
	"github.com/gi8lino/garage-webui/internal/http/auth"
	"net/http"

	httpresponse "github.com/gi8lino/garage-webui/internal/http/response"
)

// listGroupMappings returns mappings from OIDC claims to local groups.
func (s *Server) listGroupMappings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	mappings, err := s.auth.GroupMappings(r.Context())
	if err != nil {
		s.userResult(w, err)
		return
	}

	writeJSON(w, http.StatusOK, mappings)
}

// createGroupMapping adds a group mapping.
func (s *Server) createGroupMapping(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	var mapping auth.GroupMapping
	if !decodeUserRequest(w, r, &mapping) {
		return
	}

	s.userResult(w, s.auth.SetGroupMapping(r.Context(), mapping, false))
}

// deleteGroupMapping removes a group mapping.
func (s *Server) deleteGroupMapping(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	var mapping auth.GroupMapping
	if !decodeUserRequest(w, r, &mapping) {
		return
	}

	s.userResult(w, s.auth.SetGroupMapping(r.Context(), mapping, true))
}

// listGroups returns local groups.
func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	groups, err := s.auth.Groups(r.Context())
	if err != nil {
		s.userResult(w, err)
		return
	}

	writeJSON(w, http.StatusOK, groups)
}

// createGroup creates a local group.
func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	var input struct {
		Name string `json:"name"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	s.userResult(w, s.auth.CreateGroup(r.Context(), input.Name))
}

// deleteGroup removes a local group.
func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	s.userResult(w, s.auth.DeleteGroup(r.Context(), r.PathValue("group")))
}

// updateUserGroups replaces local group memberships.
func (s *Server) updateUserGroups(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.administrator(w, r)
	if !ok {
		return
	}

	var input struct {
		Groups []string `json:"groups"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	if input.Groups == nil {
		httpresponse.Error(w, "groups is required", http.StatusBadRequest)
		return
	}

	s.userResult(w, s.auth.SetAccountGroups(r.Context(), actor, r.PathValue("username"), input.Groups))
}
