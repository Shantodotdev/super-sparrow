package httpserver

import (
	"net/http"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

func (s *server) listProjects(w http.ResponseWriter, r *http.Request) error {
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		return err
	}

	views := make([]projectView, 0, len(projects))
	for _, project := range projects {
		views = append(views, newProjectView(project))
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": views})
	return nil
}

func (s *server) createProject(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	project, err := s.store.CreateProject(r.Context(), identityFrom(r.Context()).caller(), store.NewProject{Key: body.Key, Name: body.Name})
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusCreated, map[string]any{"project": newProjectView(project)})
	return nil
}

func (s *server) getProject(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusOK, map[string]any{"project": newProjectView(project)})
	return nil
}

func (s *server) renameProject(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}
	if err := s.store.RenameProject(r.Context(), identityFrom(r.Context()).caller(), project.ID, body.Name); err != nil {
		return err
	}

	project, err = s.store.GetProject(r.Context(), project.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": newProjectView(project)})
	return nil
}

func (s *server) deleteProject(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}
	if err := s.store.DeleteProject(r.Context(), identityFrom(r.Context()).caller(), project.ID); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) listMilestones(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}
	milestones, err := s.store.ListMilestones(r.Context(), project.ID)
	if err != nil {
		return err
	}

	views := make([]milestoneView, 0, len(milestones))
	for _, milestone := range milestones {
		views = append(views, newMilestoneView(milestone))
	}
	writeJSON(w, http.StatusOK, map[string]any{"milestones": views})
	return nil
}

func (s *server) createMilestone(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}
	milestone, err := s.store.CreateMilestone(r.Context(), identityFrom(r.Context()).caller(),
		store.NewMilestone{ProjectID: project.ID, Name: body.Name, Status: store.MilestoneStatus(body.Status)})
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusCreated, map[string]any{"milestone": newMilestoneView(milestone)})
	return nil
}

func (s *server) updateMilestone(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name     *string `json:"name"`
		Status   *string `json:"status"`
		Position *int    `json:"position"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	update := store.MilestoneUpdate{Name: optionalField(body.Name), Position: optionalField(body.Position)}
	if body.Status != nil {
		update.Status = store.Change(store.MilestoneStatus(*body.Status))
	}
	if err := s.store.UpdateMilestone(r.Context(), identityFrom(r.Context()).caller(), r.PathValue("id"), update); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) listLabels(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}
	labels, err := s.store.ListLabels(r.Context(), project.ID)
	if err != nil {
		return err
	}

	views := make([]labelView, 0, len(labels))
	for _, label := range labels {
		views = append(views, newLabelView(label))
	}
	writeJSON(w, http.StatusOK, map[string]any{"labels": views})
	return nil
}

func (s *server) createLabel(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}

	project, err := s.projectFromPath(r)
	if err != nil {
		return err
	}
	label, err := s.store.CreateLabel(r.Context(), identityFrom(r.Context()).caller(), project.ID, body.Name)
	if err != nil {
		return err
	}

	writeJSON(w, http.StatusCreated, map[string]any{"label": newLabelView(label)})
	return nil
}
