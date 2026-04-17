// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Work-Fort/Hive/internal/domain"

	"gopkg.in/yaml.v3"
)

// ExportResult holds counts of exported entities.
type ExportResult struct {
	Teams       int
	Roles       int
	Permissions int
	Agents      int
	Documents   int
	Tasks       int
}

// Export fetches all entities from the DataSource and writes them
// to the target directory in dependency order.
func Export(ctx context.Context, ds DataSource, dir string) (*ExportResult, error) {
	subdirs := []string{"teams", "roles", "permissions", "agents", "documents", "tasks"}
	for _, sub := range subdirs {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", sub, err)
		}
	}

	var result ExportResult

	// Teams
	teams, err := ds.ListTeams(ctx)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	for _, t := range teams {
		tf := TeamFile{Name: t.Name, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
		if err := writeYAML(filepath.Join(dir, "teams", SanitizeName(t.Name)+".yaml"), tf); err != nil {
			return nil, fmt.Errorf("write team %q: %w", t.Name, err)
		}
		result.Teams++
	}

	// Roles
	roles, err := ds.ListAllRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	// Build ID->name map for parent references
	roleNames := make(map[string]string)
	for _, r := range roles {
		roleNames[r.ID] = r.Name
	}
	for _, r := range roles {
		parentName := ""
		if r.ParentID != "" {
			parentName = roleNames[r.ParentID]
		}
		rf := RoleFile{Name: r.Name, Parent: parentName, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
		if err := writeYAML(filepath.Join(dir, "roles", SanitizeName(r.Name)+".yaml"), rf); err != nil {
			return nil, fmt.Errorf("write role %q: %w", r.Name, err)
		}
		result.Roles++
	}

	// Permissions
	perms, err := ds.ListPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	for _, p := range perms {
		pf := PermissionFile{Name: p.Name}
		if err := writeYAML(filepath.Join(dir, "permissions", SanitizeName(p.Name)+".yaml"), pf); err != nil {
			return nil, fmt.Errorf("write permission %q: %w", p.Name, err)
		}
		result.Permissions++
	}

	// Agents (with roles and permissions)
	agents, err := ds.ListAllAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	// Build team ID->name map
	teamNames := make(map[string]string)
	for _, t := range teams {
		teamNames[t.ID] = t.Name
	}
	for _, a := range agents {
		agentRoles, err := ds.GetAgentRoles(ctx, a.ID)
		if err != nil {
			return nil, fmt.Errorf("get roles for agent %q: %w", a.Name, err)
		}
		agentPerms, err := ds.GetAgentPermissions(ctx, a.ID)
		if err != nil {
			return nil, fmt.Errorf("get permissions for agent %q: %w", a.Name, err)
		}

		var roleEntries []AgentRoleEntry
		for _, ar := range agentRoles {
			roleEntries = append(roleEntries, AgentRoleEntry{
				Role:     roleNames[ar.RoleID],
				Priority: ar.Priority,
			})
		}

		var permEntries []AgentPermissionEntry
		for _, ap := range agentPerms {
			entry := AgentPermissionEntry{Permission: ap.PermissionID}
			if ap.ScopeTeamID != "" {
				entry.ScopeTeam = teamNames[ap.ScopeTeamID]
			}
			permEntries = append(permEntries, entry)
		}

		af := AgentFile{
			Name: a.Name, Team: teamNames[a.TeamID],
			Model: a.Model, Runtime: a.Runtime,
			CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
			Roles: roleEntries, Permissions: permEntries,
		}
		if err := writeYAML(filepath.Join(dir, "agents", SanitizeName(a.Name)+".yaml"), af); err != nil {
			return nil, fmt.Errorf("write agent %q: %w", a.Name, err)
		}
		result.Agents++
	}

	// Documents
	for _, r := range roles {
		docs, err := ds.ListRoleDocuments(ctx, r.ID)
		if err != nil {
			return nil, fmt.Errorf("list documents for role %q: %w", r.Name, err)
		}
		for _, d := range docs {
			if err := writeDocument(dir, "role", r.Name, d); err != nil {
				return nil, fmt.Errorf("write document for role %q: %w", r.Name, err)
			}
			result.Documents++
		}
	}
	for _, a := range agents {
		docs, err := ds.ListAgentMemory(ctx, a.ID)
		if err != nil {
			return nil, fmt.Errorf("list memory for agent %q: %w", a.Name, err)
		}
		for _, d := range docs {
			if err := writeDocument(dir, "agent", a.Name, d); err != nil {
				return nil, fmt.Errorf("write document for agent %q: %w", a.Name, err)
			}
			result.Documents++
		}
	}

	// Tasks
	agentNames := make(map[string]string)
	for _, a := range agents {
		agentNames[a.ID] = a.Name
	}
	for _, t := range teams {
		tasks, err := ds.ListTeamTasks(ctx, t.ID)
		if err != nil {
			return nil, fmt.Errorf("list tasks for team %q: %w", t.Name, err)
		}
		for _, tk := range tasks {
			taskF := TaskFile{
				Title: tk.Title, Team: t.Name,
				Status:    string(tk.Status),
				CreatedAt: tk.CreatedAt, UpdatedAt: tk.UpdatedAt,
				Description: tk.Description,
			}
			if tk.AgentID != "" {
				taskF.Agent = agentNames[tk.AgentID]
			}
			filename := SanitizeName(t.Name) + "--" + SanitizeName(tk.Title) + ".yaml"
			if err := writeYAML(filepath.Join(dir, "tasks", filename), taskF); err != nil {
				return nil, fmt.Errorf("write task %q: %w", tk.Title, err)
			}
			result.Tasks++
		}
	}

	return &result, nil
}

func writeYAML(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func writeDocument(dir, kind, ownerName string, d *domain.Document) error {
	fm := &DocumentFrontMatter{
		Title: d.Title, Kind: string(d.Kind),
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if kind == "role" {
		fm.Role = ownerName
	} else if kind == "agent" {
		fm.Agent = ownerName
	} else {
		return fmt.Errorf("unknown document kind %q", kind)
	}
	data, err := MarshalFrontMatter(fm, d.Content)
	if err != nil {
		return fmt.Errorf("marshal document %q: %w", d.Title, err)
	}
	filename := kind + "-" + SanitizeName(ownerName) + "--" + SanitizeName(d.Title) + ".md"
	return os.WriteFile(filepath.Join(dir, "documents", filename), data, 0644)
}
