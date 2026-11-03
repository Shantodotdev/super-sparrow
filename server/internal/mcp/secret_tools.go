package mcp

import (
	"context"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

func (s *Server) secretTools() []tool {
	return []tool{
		{
			name:  "list_secret_names",
			title: "List secret names",
			description: "List the key names of a project's secrets, never their values: your owner's dev (with whether each key is a team default or their override), " +
				"the team dev defaults, staging, and production, plus required keys that have no value. Read dev values with the sparrow CLI.",
			schema:   object([]string{"project_key"}, map[string]any{"project_key": text("Project key from list_projects.")}),
			readOnly: true,
			run:      bind(s.listSecretNames),
		},
	}
}

// secretNameOutput is a key of a person's dev and where its value comes from.
type secretNameOutput struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

func (s *Server) listSecretNames(ctx context.Context, caller store.Caller, args struct {
	ProjectKey string `json:"project_key"`
}) (any, error) {
	project, err := s.project(ctx, args.ProjectKey)
	if err != nil {
		return nil, err
	}

	overview, err := s.store.SecretsOverview(ctx, caller, project.ID)
	if err != nil {
		return nil, err
	}
	if !overview.Enabled {
		return map[string]any{"enabled": false}, nil
	}

	dev := overview.Configs[store.ConfigDev]
	devNames := make([]secretNameOutput, 0, len(dev.Secrets))
	for _, entry := range dev.Secrets {
		devNames = append(devNames, secretNameOutput{Name: entry.Name, Source: string(entry.Source)})
	}
	names := func(config store.SecretConfig) []string {
		listing := overview.Configs[config]
		result := make([]string, 0, len(listing.Secrets))
		for _, entry := range listing.Secrets {
			result = append(result, entry.Name)
		}
		return result
	}

	return map[string]any{
		"enabled": true, "dev": devNames, "dev_defaults": names(store.ConfigDevDefaults),
		"staging": names(store.ConfigStaging), "production": names(store.ConfigProduction),
		"missing": map[string][]string{
			"dev": dev.Missing, "staging": overview.Configs[store.ConfigStaging].Missing,
			"production": overview.Configs[store.ConfigProduction].Missing,
		},
	}, nil
}
