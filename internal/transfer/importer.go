// SPDX-License-Identifier: GPL-3.0-or-later
package transfer

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Work-Fort/Hive/internal/domain"
	"gopkg.in/yaml.v3"
)

// ImportOptions configures import behavior.
type ImportOptions struct {
	Upsert bool // Update existing entities instead of failing
	DryRun bool // Validate only, don't create
}

// ImportResult holds counts of import actions.
type ImportResult struct {
	Teams       int
	Roles       int
	Permissions int
	Agents      int
	Documents   int
	Tasks       int
	Updated     int
}

// Import reads entity files from the directory and creates them in the
// DataSource in dependency order.
func Import(ctx context.Context, ds DataSource, dir string, opts ImportOptions) (*ImportResult, error) {
	// Phase 1: Parse all files
	teams, err := parseDir[TeamFile](filepath.Join(dir, "teams"), ".yaml")
	if err != nil {
		return nil, fmt.Errorf("parse teams: %w", err)
	}
	roles, err := parseDir[RoleFile](filepath.Join(dir, "roles"), ".yaml")
	if err != nil {
		return nil, fmt.Errorf("parse roles: %w", err)
	}
	permissions, err := parseDir[PermissionFile](filepath.Join(dir, "permissions"), ".yaml")
	if err != nil {
		return nil, fmt.Errorf("parse permissions: %w", err)
	}
	agents, err := parseDir[AgentFile](filepath.Join(dir, "agents"), ".yaml")
	if err != nil {
		return nil, fmt.Errorf("parse agents: %w", err)
	}
	documents, err := parseDocuments(filepath.Join(dir, "documents"))
	if err != nil {
		return nil, fmt.Errorf("parse documents: %w", err)
	}
	tasks, err := parseDir[TaskFile](filepath.Join(dir, "tasks"), ".yaml")
	if err != nil {
		return nil, fmt.Errorf("parse tasks: %w", err)
	}

	var result ImportResult

	// Phase 2: Import teams
	teamIDs := make(map[string]string) // name -> ID
	for _, tf := range teams {
		id, updated, err := importTeam(ctx, ds, tf, opts)
		if err != nil {
			return nil, fmt.Errorf("import team %q: %w", tf.Name, err)
		}
		teamIDs[tf.Name] = id
		if updated {
			result.Updated++
		} else {
			result.Teams++
		}
	}

	// Phase 3: Import roles (topological order)
	roleIDs := make(map[string]string) // name -> ID
	sortedRoles := topoSortRoles(roles)
	for _, rf := range sortedRoles {
		parentID := ""
		if rf.Parent != "" {
			var ok bool
			parentID, ok = roleIDs[rf.Parent]
			if !ok {
				existing, err := ds.LookupRoleByName(ctx, rf.Parent)
				if err != nil {
					return nil, fmt.Errorf("resolve parent role %q: %w", rf.Parent, err)
				}
				parentID = existing.ID
				roleIDs[rf.Parent] = parentID
			}
		}
		id, updated, err := importRole(ctx, ds, rf, parentID, opts)
		if err != nil {
			return nil, fmt.Errorf("import role %q: %w", rf.Name, err)
		}
		roleIDs[rf.Name] = id
		if updated {
			result.Updated++
		} else {
			result.Roles++
		}
	}

	// Phase 4: Import permissions
	for _, pf := range permissions {
		if !opts.DryRun {
			if err := ds.EnsurePermission(ctx, pf.Name); err != nil {
				return nil, fmt.Errorf("import permission %q: %w", pf.Name, err)
			}
		}
		result.Permissions++
	}

	// Phase 5: Import agents (with roles and permissions)
	agentIDs := make(map[string]string) // name -> ID
	for _, af := range agents {
		tID, ok := teamIDs[af.Team]
		if !ok {
			existing, err := ds.LookupTeamByName(ctx, af.Team)
			if err != nil {
				return nil, fmt.Errorf("resolve team %q for agent %q: %w", af.Team, af.Name, err)
			}
			tID = existing.ID
			teamIDs[af.Team] = tID
		}

		id, updated, err := importAgent(ctx, ds, af, tID, roleIDs, teamIDs, opts)
		if err != nil {
			return nil, fmt.Errorf("import agent %q: %w", af.Name, err)
		}
		agentIDs[af.Name] = id
		if updated {
			result.Updated++
		} else {
			result.Agents++
		}
	}

	// Phase 6: Import documents
	for _, doc := range documents {
		ownerID := ""
		if doc.FM.Role != "" {
			id, ok := roleIDs[doc.FM.Role]
			if !ok {
				existing, err := ds.LookupRoleByName(ctx, doc.FM.Role)
				if err != nil {
					return nil, fmt.Errorf("resolve role %q for document %q: %w", doc.FM.Role, doc.FM.Title, err)
				}
				id = existing.ID
			}
			ownerID = id
		} else {
			id, ok := agentIDs[doc.FM.Agent]
			if !ok {
				existing, err := ds.LookupAgentByName(ctx, doc.FM.Agent)
				if err != nil {
					return nil, fmt.Errorf("resolve agent %q for document %q: %w", doc.FM.Agent, doc.FM.Title, err)
				}
				id = existing.ID
			}
			ownerID = id
		}

		updated, err := importDocument(ctx, ds, doc, ownerID, opts)
		if err != nil {
			return nil, fmt.Errorf("import document %q: %w", doc.FM.Title, err)
		}
		if updated {
			result.Updated++
		} else {
			result.Documents++
		}
	}

	// Phase 7: Import tasks
	for _, tf := range tasks {
		tID, ok := teamIDs[tf.Team]
		if !ok {
			existing, err := ds.LookupTeamByName(ctx, tf.Team)
			if err != nil {
				return nil, fmt.Errorf("resolve team %q for task %q: %w", tf.Team, tf.Title, err)
			}
			tID = existing.ID
		}

		agentID := ""
		if tf.Agent != "" {
			aID, ok := agentIDs[tf.Agent]
			if !ok {
				existing, err := ds.LookupAgentByName(ctx, tf.Agent)
				if err != nil {
					return nil, fmt.Errorf("resolve agent %q for task %q: %w", tf.Agent, tf.Title, err)
				}
				aID = existing.ID
			}
			agentID = aID
		}

		updated, err := importTask(ctx, ds, tf, tID, agentID, opts)
		if err != nil {
			return nil, fmt.Errorf("import task %q: %w", tf.Title, err)
		}
		if updated {
			result.Updated++
		} else {
			result.Tasks++
		}
	}

	return &result, nil
}

// --- helpers ---

type parsedDocument struct {
	FM   DocumentFrontMatter
	Body string
}

func parseDir[T any](dir, ext string) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil // empty directory is fine
	}
	if err != nil {
		return nil, err
	}
	var items []T
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var item T
		if err := yaml.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		items = append(items, item)
	}
	return items, nil
}

func parseDocuments(dir string) ([]parsedDocument, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var docs []parsedDocument
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		fm, body, err := UnmarshalFrontMatter(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		docs = append(docs, parsedDocument{FM: *fm, Body: body})
	}
	return docs, nil
}

func topoSortRoles(roles []RoleFile) []RoleFile {
	byName := make(map[string]RoleFile)
	for _, r := range roles {
		byName[r.Name] = r
	}
	var sorted []RoleFile
	visited := make(map[string]bool)
	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		r, ok := byName[name]
		if !ok {
			return // external parent, already in system
		}
		if r.Parent != "" {
			visit(r.Parent)
		}
		sorted = append(sorted, r)
	}
	for _, r := range roles {
		visit(r.Name)
	}
	return sorted
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func importTeam(ctx context.Context, ds DataSource, tf TeamFile, opts ImportOptions) (string, bool, error) {
	existing, err := ds.LookupTeamByName(ctx, tf.Name)
	if err == nil {
		if !opts.Upsert {
			return "", false, fmt.Errorf("team %q already exists", tf.Name)
		}
		if !opts.DryRun {
			if err := ds.UpdateTeam(ctx, existing.ID, tf.Name); err != nil {
				return "", false, err
			}
		}
		return existing.ID, true, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return "", false, err
	}
	id := newID()
	if !opts.DryRun {
		t := &domain.Team{ID: id, Name: tf.Name, CreatedAt: tf.CreatedAt, UpdatedAt: tf.UpdatedAt}
		if err := ds.CreateTeam(ctx, t); err != nil {
			return "", false, err
		}
	}
	return id, false, nil
}

func importRole(ctx context.Context, ds DataSource, rf RoleFile, parentID string, opts ImportOptions) (string, bool, error) {
	existing, err := ds.LookupRoleByName(ctx, rf.Name)
	if err == nil {
		if !opts.Upsert {
			return "", false, fmt.Errorf("role %q already exists", rf.Name)
		}
		if !opts.DryRun {
			if err := ds.UpdateRole(ctx, existing.ID, rf.Name, parentID); err != nil {
				return "", false, err
			}
		}
		return existing.ID, true, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return "", false, err
	}
	id := newID()
	if !opts.DryRun {
		r := &domain.Role{ID: id, Name: rf.Name, ParentID: parentID, CreatedAt: rf.CreatedAt, UpdatedAt: rf.UpdatedAt}
		if err := ds.CreateRole(ctx, r); err != nil {
			return "", false, err
		}
	}
	return id, false, nil
}

func importAgent(ctx context.Context, ds DataSource, af AgentFile, teamID string, roleIDs, teamIDs map[string]string, opts ImportOptions) (string, bool, error) {
	existing, err := ds.LookupAgentByName(ctx, af.Name)
	isNew := errors.Is(err, domain.ErrNotFound)
	if err != nil && !isNew {
		return "", false, err
	}
	if !isNew && !opts.Upsert {
		return "", false, fmt.Errorf("agent %q already exists", af.Name)
	}

	var id string
	updated := false
	if isNew {
		id = newID()
		if !opts.DryRun {
			a := &domain.Agent{ID: id, Name: af.Name, TeamID: teamID, CreatedAt: af.CreatedAt, UpdatedAt: af.UpdatedAt}
			if err := ds.CreateAgent(ctx, a); err != nil {
				return "", false, err
			}
		}
	} else {
		id = existing.ID
		updated = true
		if !opts.DryRun {
			if err := ds.UpdateAgent(ctx, id, af.Name, teamID); err != nil {
				return "", false, err
			}
		}
	}

	if opts.DryRun {
		return id, updated, nil
	}

	// Set roles
	if len(af.Roles) > 0 {
		var domainRoles []domain.AgentRole
		for _, ar := range af.Roles {
			rID, ok := roleIDs[ar.Role]
			if !ok {
				r, err := ds.LookupRoleByName(ctx, ar.Role)
				if err != nil {
					return "", false, fmt.Errorf("resolve role %q: %w", ar.Role, err)
				}
				rID = r.ID
			}
			domainRoles = append(domainRoles, domain.AgentRole{AgentID: id, RoleID: rID, Priority: ar.Priority})
		}
		if err := ds.SetAgentRoles(ctx, id, domainRoles); err != nil {
			return "", false, fmt.Errorf("set roles: %w", err)
		}
	}

	// Set permissions
	if len(af.Permissions) > 0 {
		var domainPerms []domain.AgentPermission
		for _, ap := range af.Permissions {
			scopeTeamID := ""
			if ap.ScopeTeam != "" {
				tID, ok := teamIDs[ap.ScopeTeam]
				if !ok {
					t, err := ds.LookupTeamByName(ctx, ap.ScopeTeam)
					if err != nil {
						return "", false, fmt.Errorf("resolve scope team %q: %w", ap.ScopeTeam, err)
					}
					tID = t.ID
				}
				scopeTeamID = tID
			}
			domainPerms = append(domainPerms, domain.AgentPermission{
				AgentID: id, PermissionID: ap.Permission, ScopeTeamID: scopeTeamID,
			})
		}
		if err := ds.SetAgentPermissions(ctx, id, domainPerms); err != nil {
			return "", false, fmt.Errorf("set permissions: %w", err)
		}
	}

	return id, updated, nil
}

func importDocument(ctx context.Context, ds DataSource, doc parsedDocument, ownerID string, opts ImportOptions) (bool, error) {
	existing, err := ds.LookupDocumentByOwnerAndTitle(ctx, ownerID, doc.FM.Title)
	if err == nil {
		if !opts.Upsert {
			return false, fmt.Errorf("document %q already exists", doc.FM.Title)
		}
		if !opts.DryRun {
			if err := ds.UpdateDocument(ctx, existing.ID, doc.FM.Title, doc.Body); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return false, err
	}
	if !opts.DryRun {
		d := &domain.Document{
			ID: newID(), Kind: domain.DocumentKind(doc.FM.Kind),
			Title: doc.FM.Title, Content: doc.Body,
			CreatedAt: doc.FM.CreatedAt, UpdatedAt: doc.FM.UpdatedAt,
		}
		if doc.FM.Role != "" {
			d.RoleID = ownerID
		} else {
			d.AgentID = ownerID
		}
		if err := ds.CreateDocument(ctx, d); err != nil {
			return false, err
		}
	}
	return false, nil
}

func importTask(ctx context.Context, ds DataSource, tf TaskFile, teamID, agentID string, opts ImportOptions) (bool, error) {
	existing, err := ds.LookupTaskByTeamAndTitle(ctx, teamID, tf.Title)
	if err == nil {
		if !opts.Upsert {
			return false, fmt.Errorf("task %q already exists", tf.Title)
		}
		if !opts.DryRun {
			t := &domain.Task{
				Title: tf.Title, Description: tf.Description,
				Status: domain.TaskStatus(tf.Status), AgentID: agentID,
			}
			if err := ds.UpdateTask(ctx, existing.ID, t); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return false, err
	}
	if !opts.DryRun {
		t := &domain.Task{
			ID: newID(), TeamID: teamID, AgentID: agentID,
			Title: tf.Title, Description: tf.Description,
			Status: domain.TaskStatus(tf.Status),
			CreatedAt: tf.CreatedAt, UpdatedAt: tf.UpdatedAt,
		}
		if err := ds.CreateTask(ctx, t); err != nil {
			return false, err
		}
	}
	return false, nil
}
