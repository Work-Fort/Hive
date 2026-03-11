// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// YAML types matching the spec file formats.

type TeamFile struct {
	Name      string    `yaml:"name"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
}

type RoleFile struct {
	Name      string    `yaml:"name"`
	Parent    string    `yaml:"parent"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
}

type PermissionFile struct {
	Name string `yaml:"name"`
}

type AgentRoleEntry struct {
	Role     string `yaml:"role"`
	Priority int    `yaml:"priority"`
}

type AgentPermissionEntry struct {
	Permission string `yaml:"permission"`
	ScopeTeam  string `yaml:"scope_team,omitempty"`
}

type AgentFile struct {
	Name        string                 `yaml:"name"`
	Team        string                 `yaml:"team"`
	CreatedAt   time.Time              `yaml:"created_at"`
	UpdatedAt   time.Time              `yaml:"updated_at"`
	Roles       []AgentRoleEntry       `yaml:"roles,omitempty"`
	Permissions []AgentPermissionEntry `yaml:"permissions,omitempty"`
}

type DocumentFrontMatter struct {
	Title     string    `yaml:"title"`
	Kind      string    `yaml:"kind"`
	Role      string    `yaml:"role,omitempty"`
	Agent     string    `yaml:"agent,omitempty"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
}

type TaskFile struct {
	Title       string    `yaml:"title"`
	Team        string    `yaml:"team"`
	Agent       string    `yaml:"agent,omitempty"`
	Status      string    `yaml:"status"`
	CreatedAt   time.Time `yaml:"created_at"`
	UpdatedAt   time.Time `yaml:"updated_at"`
	Description string    `yaml:"description,omitempty"`
}

// MarshalFrontMatter writes YAML front-matter + body content.
func MarshalFrontMatter(fm *DocumentFrontMatter, body string) ([]byte, error) {
	header, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("marshal front matter: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(header)
	buf.WriteString("---\n")
	buf.WriteString(body)
	return buf.Bytes(), nil
}

// UnmarshalFrontMatter parses YAML front-matter delimited by "---" and returns
// the front matter and remaining body content.
func UnmarshalFrontMatter(r io.Reader) (*DocumentFrontMatter, string, error) {
	scanner := bufio.NewScanner(r)

	// Expect first line to be "---"
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return nil, "", fmt.Errorf("expected front matter delimiter '---'")
	}

	var fmLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		fmLines = append(fmLines, line)
	}

	var fm DocumentFrontMatter
	if err := yaml.Unmarshal([]byte(strings.Join(fmLines, "\n")), &fm); err != nil {
		return nil, "", fmt.Errorf("unmarshal front matter: %w", err)
	}

	var bodyLines []string
	for scanner.Scan() {
		bodyLines = append(bodyLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	return &fm, strings.Join(bodyLines, "\n"), nil
}
