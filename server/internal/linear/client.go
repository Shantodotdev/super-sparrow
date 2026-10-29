// Package linear imports a project from Linear: its milestones, issues, and comments, and the
// review rounds written in comments.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// APIURL is Linear's GraphQL endpoint.
const APIURL = "https://api.linear.app/graphql"

const requestTimeout = time.Minute

// Client reads from Linear's GraphQL API with a personal API key.
type Client struct {
	endpoint string
	apiKey   string
	http     *http.Client
}

// NewClient returns a client for endpoint, which is APIURL outside tests.
func NewClient(endpoint, apiKey string) *Client {
	return &Client{endpoint: endpoint, apiKey: apiKey, http: &http.Client{Timeout: requestTimeout}}
}

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// query runs a GraphQL query and decodes its data into dst.
func (c *Client) query(ctx context.Context, query string, variables map[string]any, dst any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return fmt.Errorf("encode Linear query: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build Linear request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", c.apiKey)

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("call Linear: %w", err)
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("read Linear response: %w", err)
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("the Linear API answered %d: %s", response.StatusCode, strings.TrimSpace(string(data[:min(len(data), 300)])))
	}
	if len(envelope.Errors) > 0 {
		messages := make([]string, 0, len(envelope.Errors))
		for _, e := range envelope.Errors {
			messages = append(messages, e.Message)
		}
		return fmt.Errorf("the Linear API refused the query: %s", strings.Join(messages, "; "))
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the Linear API answered %d", response.StatusCode)
	}

	if err := json.Unmarshal(envelope.Data, dst); err != nil {
		return fmt.Errorf("read Linear data: %w", err)
	}
	return nil
}

// Project is a Linear project with the teams it belongs to.
type Project struct {
	ID    string
	Name  string
	Teams []string
}

// FindProject returns the project whose name or ID is nameOrID.
func (c *Client) FindProject(ctx context.Context, nameOrID string) (Project, error) {
	const query = `query Projects($after: String) {
		projects(first: 50, after: $after, includeArchived: true) {
			nodes { id name teams { nodes { key } } }
			pageInfo { hasNextPage endCursor }
		}
	}`

	var matches []Project
	after := ""
	for {
		var data struct {
			Projects struct {
				Nodes []struct {
					ID    string `json:"id"`
					Name  string `json:"name"`
					Teams struct {
						Nodes []struct {
							Key string `json:"key"`
						} `json:"nodes"`
					} `json:"teams"`
				} `json:"nodes"`
				PageInfo pageInfo `json:"pageInfo"`
			} `json:"projects"`
		}
		if err := c.query(ctx, query, variablesAfter(after), &data); err != nil {
			return Project{}, err
		}

		for _, node := range data.Projects.Nodes {
			if node.ID == nameOrID || strings.EqualFold(node.Name, nameOrID) {
				project := Project{ID: node.ID, Name: node.Name}
				for _, team := range node.Teams.Nodes {
					project.Teams = append(project.Teams, team.Key)
				}
				matches = append(matches, project)
			}
		}
		if !data.Projects.PageInfo.HasNextPage {
			break
		}
		after = data.Projects.PageInfo.EndCursor
	}

	switch len(matches) {
	case 0:
		return Project{}, fmt.Errorf("there is no Linear project named %q", nameOrID)
	case 1:
		return matches[0], nil
	}
	return Project{}, fmt.Errorf("there are %d Linear projects named %q; pass the project's ID instead", len(matches), nameOrID)
}

// Milestone is a Linear project milestone.
type Milestone struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	SortOrder float64 `json:"sortOrder"`
}

// Milestones returns a project's milestones in their order.
func (c *Client) Milestones(ctx context.Context, projectID string) ([]Milestone, error) {
	const query = `query Milestones($id: String!, $after: String) {
		project(id: $id) {
			projectMilestones(first: 50, after: $after) {
				nodes { id name sortOrder }
				pageInfo { hasNextPage endCursor }
			}
		}
	}`

	var milestones []Milestone
	after := ""
	for {
		var data struct {
			Project struct {
				ProjectMilestones struct {
					Nodes    []Milestone `json:"nodes"`
					PageInfo pageInfo    `json:"pageInfo"`
				} `json:"projectMilestones"`
			} `json:"project"`
		}
		variables := variablesAfter(after)
		variables["id"] = projectID
		if err := c.query(ctx, query, variables, &data); err != nil {
			return nil, err
		}

		milestones = append(milestones, data.Project.ProjectMilestones.Nodes...)
		if !data.Project.ProjectMilestones.PageInfo.HasNextPage {
			return milestones, nil
		}
		after = data.Project.ProjectMilestones.PageInfo.EndCursor
	}
}

// Issue is a Linear issue.
type Issue struct {
	ID          string     `json:"id"`
	Identifier  string     `json:"identifier"`
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	BranchName  string     `json:"branchName"`
	CompletedAt *time.Time `json:"completedAt"`
	CanceledAt  *time.Time `json:"canceledAt"`
	State       struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"state"`
	Team struct {
		Key string `json:"key"`
	} `json:"team"`
	Parent *struct {
		ID string `json:"id"`
	} `json:"parent"`
	ProjectMilestone *struct {
		ID string `json:"id"`
	} `json:"projectMilestone"`
	Assignee *struct {
		Email string `json:"email"`
	} `json:"assignee"`
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
}

// Issues returns every issue of a project, archived ones included.
func (c *Client) Issues(ctx context.Context, projectID string) ([]Issue, error) {
	const query = `query Issues($id: String!, $after: String) {
		project(id: $id) {
			issues(first: 50, after: $after, includeArchived: true) {
				nodes {
					id identifier number title description branchName completedAt canceledAt
					state { type name }
					team { key }
					parent { id }
					projectMilestone { id }
					assignee { email }
					labels { nodes { name } }
				}
				pageInfo { hasNextPage endCursor }
			}
		}
	}`

	var issues []Issue
	after := ""
	for {
		var data struct {
			Project struct {
				Issues struct {
					Nodes    []Issue  `json:"nodes"`
					PageInfo pageInfo `json:"pageInfo"`
				} `json:"issues"`
			} `json:"project"`
		}
		variables := variablesAfter(after)
		variables["id"] = projectID
		if err := c.query(ctx, query, variables, &data); err != nil {
			return nil, err
		}

		issues = append(issues, data.Project.Issues.Nodes...)
		if !data.Project.Issues.PageInfo.HasNextPage {
			return issues, nil
		}
		after = data.Project.Issues.PageInfo.EndCursor
	}
}

// Comment is a comment on a Linear issue.
type Comment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	User      *struct {
		Name string `json:"name"`
	} `json:"user"`
}

// Comments returns an issue's comments, oldest first.
func (c *Client) Comments(ctx context.Context, issueID string) ([]Comment, error) {
	const query = `query Comments($id: String!, $after: String) {
		issue(id: $id) {
			comments(first: 50, after: $after, orderBy: createdAt) {
				nodes { id body createdAt user { name } }
				pageInfo { hasNextPage endCursor }
			}
		}
	}`

	var comments []Comment
	after := ""
	for {
		var data struct {
			Issue struct {
				Comments struct {
					Nodes    []Comment `json:"nodes"`
					PageInfo pageInfo  `json:"pageInfo"`
				} `json:"comments"`
			} `json:"issue"`
		}
		variables := variablesAfter(after)
		variables["id"] = issueID
		if err := c.query(ctx, query, variables, &data); err != nil {
			return nil, err
		}

		comments = append(comments, data.Issue.Comments.Nodes...)
		if !data.Issue.Comments.PageInfo.HasNextPage {
			return comments, nil
		}
		after = data.Issue.Comments.PageInfo.EndCursor
	}
}

func variablesAfter(after string) map[string]any {
	if after == "" {
		return map[string]any{"after": nil}
	}
	return map[string]any{"after": after}
}
