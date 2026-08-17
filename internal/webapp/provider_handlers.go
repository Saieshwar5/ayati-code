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
	id, ok := providerID(request.URL.Path, false)
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
	id, ok := providerID(request.URL.Path, true)
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
	id, ok := providerID(request.URL.Path, false)
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

func providerID(path string, test bool) (string, bool) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "/api/providers"), "/"), "/")
	if test {
		return firstProviderPart(parts, len(parts) == 2 && parts[1] == "test")
	}
	return firstProviderPart(parts, len(parts) == 1)
}

func firstProviderPart(parts []string, valid bool) (string, bool) {
	if !valid || strings.TrimSpace(parts[0]) == "" {
		return "", false
	}
	return strings.TrimSpace(parts[0]), true
}
