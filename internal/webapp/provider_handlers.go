package webapp

import "net/http"

func (s *Server) listProviders(writer http.ResponseWriter, _ *http.Request) {
	s.writeJSON(writer, http.StatusOK, s.providers.List())
}
