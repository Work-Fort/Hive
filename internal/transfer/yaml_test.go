package transfer

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestTeamFileRoundTrip(t *testing.T) {
	ts := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	tf := TeamFile{Name: "engineering", CreatedAt: ts, UpdatedAt: ts}

	data, err := yaml.Marshal(tf)
	if err != nil {
		t.Fatal(err)
	}

	var got TeamFile
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "engineering" {
		t.Errorf("name = %q, want engineering", got.Name)
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("created_at = %v, want %v", got.CreatedAt, ts)
	}
}

func TestFrontMatterRoundTrip(t *testing.T) {
	ts := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)
	fm := &DocumentFrontMatter{
		Title:     "Coding Standards",
		Kind:      "role",
		Role:      "developer",
		CreatedAt: ts,
		UpdatedAt: ts,
	}
	body := "Some markdown content\nWith multiple lines"

	data, err := MarshalFrontMatter(fm, body)
	if err != nil {
		t.Fatal(err)
	}

	gotFM, gotBody, err := UnmarshalFrontMatter(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}

	if gotFM.Title != "Coding Standards" {
		t.Errorf("title = %q, want Coding Standards", gotFM.Title)
	}
	if gotFM.Kind != "role" {
		t.Errorf("kind = %q, want role", gotFM.Kind)
	}
	if gotFM.Role != "developer" {
		t.Errorf("role = %q, want developer", gotFM.Role)
	}
	if gotBody != body {
		t.Errorf("body = %q, want %q", gotBody, body)
	}
}

func TestAgentFileWithPermissions(t *testing.T) {
	af := AgentFile{
		Name: "claude",
		Team: "engineering",
		Roles: []AgentRoleEntry{
			{Role: "developer", Priority: 1},
		},
		Permissions: []AgentPermissionEntry{
			{Permission: "read-docs", ScopeTeam: "engineering"},
			{Permission: "write-docs"},
		},
	}

	data, err := yaml.Marshal(af)
	if err != nil {
		t.Fatal(err)
	}

	var got AgentFile
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if len(got.Permissions) != 2 {
		t.Fatalf("got %d permissions, want 2", len(got.Permissions))
	}
	if got.Permissions[0].ScopeTeam != "engineering" {
		t.Errorf("scope_team = %q, want engineering", got.Permissions[0].ScopeTeam)
	}
	if got.Permissions[1].ScopeTeam != "" {
		t.Errorf("global perm scope_team = %q, want empty", got.Permissions[1].ScopeTeam)
	}
}
