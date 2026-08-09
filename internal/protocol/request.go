package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	agentruntime "github.com/Saieshwar5/ayati-runtime/internal/runtime"
)

const maxRequestBytes = 4 << 20

func DecodeRequest(reader io.Reader) (agentruntime.Request, error) {
	var request agentruntime.Request
	if err := decodeLimited(reader, maxRequestBytes, &request); err != nil {
		return agentruntime.Request{}, fmt.Errorf("decode run request: %w", err)
	}
	request.Prompt = strings.TrimSpace(request.Prompt)
	if request.Version == 0 {
		request.Version = agentruntime.ProtocolVersion
	}
	if request.Version != agentruntime.ProtocolVersion {
		return agentruntime.Request{}, fmt.Errorf("unsupported request version %d", request.Version)
	}
	if request.RunID == "" {
		id, err := newRunID()
		if err != nil {
			return agentruntime.Request{}, err
		}
		request.RunID = id
	}
	return request, nil
}

func newRunID() (string, error) {
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	return hex.EncodeToString(value), nil
}
