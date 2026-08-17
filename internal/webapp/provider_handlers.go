package webapp

import (
	"net/http"
	"strings"

	modelprovider "github.com/Saieshwar5/ayati-code/internal/provider"
)

func (s *Server) listProviders(writer http.ResponseWriter, _ *http.Request) {
	s.writeJSON(writer, http.StatusOK, s.providers.List())
}

func (s *Server) configureProvider(writer http.ResponseWriter, request *http.Request) {
	id, ok := providerPath(request.URL.Path, "")
	if !ok {
		http.NotFound(writer, request)
		return
	}
	if s.connections == nil {
		s.writeError(writer, http.StatusNotImplemented, "provider configuration is unavailable")
		return
	}
	var input modelprovider.ConnectionInput
	if !s.decode(writer, request, &input) {
		return
	}
	definition, err := s.connections.Configure(id, input)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(writer, http.StatusOK, definition)
}

func (s *Server) testProvider(writer http.ResponseWriter, request *http.Request) {
	id, ok := providerPath(request.URL.Path, "test")
	if !ok {
		http.NotFound(writer, request)
		return
	}
	if s.connections == nil {
		s.writeError(writer, http.StatusNotImplemented, "provider configuration is unavailable")
		return
	}
	var input modelprovider.ConnectionInput
	if !s.decode(writer, request, &input) {
		return
	}
	if err := s.connections.Test(request.Context(), id, input); err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(writer, http.StatusOK, map[string]bool{"verified": true})
}

func (s *Server) removeProvider(writer http.ResponseWriter, request *http.Request) {
	id, ok := providerPath(request.URL.Path, "")
	if !ok {
		http.NotFound(writer, request)
		return
	}
	if s.connections == nil {
		s.writeError(writer, http.StatusNotImplemented, "provider configuration is unavailable")
		return
	}
	if err := s.connections.Remove(id); err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) listProviderModels(writer http.ResponseWriter, request *http.Request) {
	id, ok := providerPath(request.URL.Path, "models")
	if !ok {
		http.NotFound(writer, request)
		return
	}
	if s.connections == nil {
		s.writeError(writer, http.StatusNotImplemented, "provider model discovery is unavailable")
		return
	}
	models, err := s.connections.Models(request.Context(), id)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if models == nil {
		models = []modelprovider.Model{}
	}
	s.writeJSON(writer, http.StatusOK, models)
}

func providerPath(path, action string) (string, bool) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "/api/providers"), "/"), "/")
	if action == "" {
		return firstProviderPart(parts, len(parts) == 1)
	}
	return firstProviderPart(parts, len(parts) == 2 && parts[1] == action)
}

func firstProviderPart(parts []string, valid bool) (string, bool) {
	if !valid || strings.TrimSpace(parts[0]) == "" {
		return "", false
	}
	return strings.TrimSpace(parts[0]), true
}
