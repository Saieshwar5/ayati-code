package webapp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/agent"
)

func TestHandlerManagesSkillsAndAgentAttachments(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	response := serve(handler, http.MethodPost, "/api/skills",
		`{"name":"Go review","description":"Review guidance","markdown":"Check context cancellation."}`, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create skill status = %d, body = %s", response.Code, response.Body.String())
	}
	var skill agent.Skill
	if err := json.Unmarshal(response.Body.Bytes(), &skill); err != nil {
		t.Fatalf("decode skill: %v", err)
	}
	response = serve(handler, http.MethodGet, "/api/skills", "", false)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Go review") {
		t.Fatalf("list skills status = %d, body = %s", response.Code, response.Body.String())
	}
	createAgent := `{"name":"Reviewer","emoji":"🔍","description":"","provider_id":"fireworks","model":"","max_steps":8,"shell_enabled":true,"instructions":"","skill_ids":["` + skill.ID + `"]}`
	response = serve(handler, http.MethodPost, "/api/agents", createAgent, true)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), skill.ID) {
		t.Fatalf("attach skill status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodPost, "/api/skills/"+skill.ID+"/archive", "", true)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "detach") {
		t.Fatalf("archive attached skill status = %d, body = %s", response.Code, response.Body.String())
	}
}
