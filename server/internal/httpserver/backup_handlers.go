package httpserver

import "net/http"

// backupStatus reports whether backups run and the last run and last success of each job.
func (s *server) backupStatus(w http.ResponseWriter, r *http.Request) error {
	statuses, err := s.store.GetBackupStatus(r.Context())
	if err != nil {
		return err
	}

	jobs := map[string]any{}
	for job, status := range statuses {
		jobs[string(job)] = map[string]any{
			"last_run":     newBackupRunView(status.LastRun),
			"last_success": newBackupRunView(status.LastSuccess),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": s.backupsEnabled, "jobs": jobs})
	return nil
}
