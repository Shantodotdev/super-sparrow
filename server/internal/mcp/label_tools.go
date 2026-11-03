package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// labelTools let agents discover, create, edit, and apply a project's labels.
func (s *Server) labelTools() []tool {
	labelNames := array("Names of existing labels in the project. The list replaces the current labels; an empty list removes them all.", text("A label name."))
	color := text("A six-digit hex color, such as #f2a9ae.")
	return []tool{
		{
			name:        "list_labels",
			title:       "List labels",
			description: "List a project's labels with their colors, in name order.",
			schema:      object([]string{"project_key"}, map[string]any{"project_key": text("Project key from list_projects.")}),
			readOnly:    true,
			run:         bind(s.listLabels),
		},
		{
			name:  "create_label",
			title: "Create a label",
			description: "Create a label in a project so issues and subissues can use it. Names are unique within a project. " +
				"Without a color the label gets the default blue.",
			schema: object([]string{"project_key", "name"}, map[string]any{
				"project_key": text("Project key from list_projects."), "name": text("The label's name, up to 50 characters."), "color": color,
			}),
			run: bind(s.createLabel),
		},
		{
			name:        "update_label",
			title:       "Edit a label",
			description: "Rename or recolor a label. Issues and subissues that use it keep it.",
			schema: object([]string{"project_key", "name"}, map[string]any{
				"project_key": text("Project key from list_projects."), "name": text("The label's current name."),
				"new_name": text("The new name."), "color": color,
			}),
			run: bind(s.updateLabel),
		},
		{
			name:  "set_labels",
			title: "Set labels on an issue or subissue",
			description: "Replace the labels of an issue, or of one of its subissues when subissue_key is given. " +
				"Labels must already exist in the issue's project; create missing ones with create_label.",
			schema: object([]string{"issue_key", "labels"}, map[string]any{
				"issue_key": issueKey, "subissue_key": text("A subissue key from list_subissues, such as WP-142.1."), "labels": labelNames,
			}),
			run: bind(s.setLabels),
		},
	}
}

// labelOutput is a label as agents see it: its name is how tools refer to it.
type labelOutput struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (s *Server) listLabels(ctx context.Context, _ store.Caller, args struct {
	ProjectKey string `json:"project_key"`
}) (any, error) {
	project, err := s.project(ctx, args.ProjectKey)
	if err != nil {
		return nil, err
	}
	labels, err := s.store.ListLabels(ctx, project.ID)
	if err != nil {
		return nil, err
	}

	outputs := []labelOutput{}
	for _, label := range labels {
		outputs = append(outputs, labelOutput{Name: label.Name, Color: label.Color})
	}
	return map[string]any{"project_key": project.Key, "labels": outputs}, nil
}

func (s *Server) createLabel(ctx context.Context, caller store.Caller, args struct {
	ProjectKey string  `json:"project_key"`
	Name       string  `json:"name"`
	Color      *string `json:"color"`
}) (any, error) {
	project, err := s.project(ctx, args.ProjectKey)
	if err != nil {
		return nil, err
	}

	var label store.Label
	if args.Color == nil {
		label, err = s.store.CreateLabel(ctx, caller, project.ID, args.Name)
	} else {
		label, err = s.store.CreateLabelWithColor(ctx, caller, project.ID, args.Name, *args.Color)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"project_key": project.Key, "label": labelOutput{Name: label.Name, Color: label.Color}}, nil
}

func (s *Server) updateLabel(ctx context.Context, caller store.Caller, args struct {
	ProjectKey string  `json:"project_key"`
	Name       string  `json:"name"`
	NewName    *string `json:"new_name"`
	Color      *string `json:"color"`
}) (any, error) {
	if args.NewName == nil && args.Color == nil {
		return nil, &store.ValidationError{Field: "fields", Message: "supply new_name or color"}
	}
	project, err := s.project(ctx, args.ProjectKey)
	if err != nil {
		return nil, err
	}
	ids, err := s.labelIDs(ctx, project.ID, project.Key, "name", []string{args.Name})
	if err != nil {
		return nil, err
	}

	update := store.LabelUpdate{}
	if args.NewName != nil {
		update.Name = store.Change(*args.NewName)
	}
	if args.Color != nil {
		update.Color = store.Change(*args.Color)
	}
	label, err := s.store.UpdateLabel(ctx, caller, project.ID, ids[0], update)
	if err != nil {
		return nil, err
	}
	return map[string]any{"project_key": project.Key, "label": labelOutput{Name: label.Name, Color: label.Color}}, nil
}

func (s *Server) setLabels(ctx context.Context, caller store.Caller, args struct {
	IssueKey    string   `json:"issue_key"`
	SubissueKey string   `json:"subissue_key"`
	Labels      []string `json:"labels"`
}) (any, error) {
	issue, err := s.issue(ctx, args.IssueKey)
	if err != nil {
		return nil, err
	}
	ids, err := s.labelIDs(ctx, issue.ProjectID, issue.ProjectKey, "labels", args.Labels)
	if err != nil {
		return nil, err
	}

	if args.SubissueKey == "" {
		if err := s.store.SetIssueLabels(ctx, caller, issue.ID, ids); err != nil {
			return nil, err
		}
		issue, err = s.store.GetIssue(ctx, issue.ID)
		if err != nil {
			return nil, err
		}
		return s.newIssueOutput(ctx, issue)
	}

	sub, err := s.subissue(ctx, issue, args.SubissueKey)
	if err != nil {
		return nil, err
	}
	sub, err = s.store.UpdateSubissue(ctx, caller, issue.ID, sub.ID, store.SubissueUpdate{LabelIDs: store.Change(ids)})
	if err != nil {
		return nil, err
	}
	return s.subissueOutput(ctx, issue, sub)
}

// project finds a project by its key, refusing unknown keys with a message that names the key.
func (s *Server) project(ctx context.Context, key string) (store.Project, error) {
	if strings.TrimSpace(key) == "" {
		return store.Project{}, &store.ValidationError{Field: "project_key", Message: "is required"}
	}
	project, err := s.store.GetProjectByKey(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		return store.Project{}, store.Refuse(store.ErrNotFound, "there is no project %s", strings.ToUpper(key))
	}
	return project, err
}

// labelIDs resolves label names, ignoring case, to the IDs of the project's labels. An unknown
// name is reported against field.
func (s *Server) labelIDs(ctx context.Context, projectID, projectKey, field string, names []string) ([]string, error) {
	labels, err := s.store.ListLabels(ctx, projectID)
	if err != nil {
		return nil, err
	}
	byName := map[string]string{}
	for _, label := range labels {
		byName[strings.ToLower(label.Name)] = label.ID
	}

	ids := []string{}
	for _, name := range names {
		id, ok := byName[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			return nil, &store.ValidationError{Field: field, Message: fmt.Sprintf("project %s has no label %q", projectKey, name)}
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// labelNameList returns the names of the given label IDs in name order.
func (s *Server) labelNameList(ctx context.Context, projectID string, ids []string) ([]string, error) {
	names := []string{}
	if len(ids) == 0 {
		return names, nil
	}
	labels, err := s.store.ListLabels(ctx, projectID)
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	for _, label := range labels {
		if wanted[label.ID] {
			names = append(names, label.Name)
		}
	}
	return names, nil
}
