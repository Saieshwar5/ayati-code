package protocol

import (
	"encoding/json"
	"io"
	"sync"

	agentruntime "github.com/Saieshwar5/ayati-code/internal/runtime"
)

type JSONLSink struct {
	mu      sync.Mutex
	encoder *json.Encoder
}

func NewJSONLSink(writer io.Writer) *JSONLSink {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return &JSONLSink{encoder: encoder}
}

func (s *JSONLSink) Emit(event agentruntime.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.encoder.Encode(event)
}
