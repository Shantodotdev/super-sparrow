package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// RunAgentTool commits a tool's writes, events, and optional retry receipt together. Agents run
// tools, and so do people through their signed-in CLI. A repeated
// key returns the original result for this actor; using it with different arguments is refused.
// fn must use the scoped store exclusively and must not retain it after returning.
func (s *Store) RunAgentTool(ctx context.Context, by Caller, key, hash string, fn func(*Store) (any, error)) (json.RawMessage, error) {
	if len(key) > 200 {
		return nil, invalid("idempotency_key", "must contain at most 200 bytes")
	}
	var result json.RawMessage
	err := s.write(ctx, by, func(w *writeTx) error {
		// A person runs the tools through their signed-in CLI, never with a browser cookie.
		if w.caller.Kind != ActorAgent && (w.caller.Kind != ActorHuman || w.client != ClientCLI) {
			return forbidden("only an agent or a signed-in CLI can run agent tools")
		}
		// Reuse the committed result before running any mutation.
		if key != "" {
			var savedHash, savedResult string
			err := w.tx.QueryRowContext(ctx, `SELECT request_hash, result FROM mcp_retry_receipts WHERE actor_id = ? AND request_key = ?`, by.ActorID, key).Scan(&savedHash, &savedResult)
			switch {
			case err == nil:
				if hash != savedHash {
					return Refuse(ErrConflict, "idempotency_key was already used for a different request")
				}
				result = json.RawMessage(savedResult)
				return nil
			case !errors.Is(err, sql.ErrNoRows):
				return fmt.Errorf("read tool receipt: %w", err)
			}
		}
		// Keep reads and writes on the same transaction until the result is encoded.
		scoped := &Store{db: transactionDatabase{w.tx}, now: s.now, secrets: s.secrets, transaction: w}
		output, err := fn(scoped)
		if err != nil {
			return err
		}
		result, err = json.Marshal(output)
		if err != nil {
			return fmt.Errorf("encode tool receipt: %w", err)
		}
		if key != "" {
			if _, err := w.exec(`INSERT INTO mcp_retry_receipts (actor_id, request_key, request_hash, result, created_at) VALUES (?, ?, ?, ?, ?)`, by.ActorID, key, hash, string(result), w.timestamp()); err != nil {
				return fmt.Errorf("save tool receipt: %w", err)
			}
		}
		return nil
	})
	return result, err
}

// ListAgentFindings returns a bounded page, including resolved findings and duplicates when
// requested. Offset cursors reflect current data; callers should restart after changing filters.
func (s *Store) ListAgentFindings(ctx context.Context, issueID, roundID string, statuses []FindingStatus, excludeDuplicates bool, limit, offset int) ([]Finding, error) {
	if limit < 1 || limit > 101 || offset < 0 {
		return nil, invalid("page", "is invalid")
	}
	where, args, err := findingFilter(`r.issue_id = ?`, []any{issueID}, statuses, excludeDuplicates)
	if err != nil {
		return nil, err
	}
	if roundID != "" {
		where += ` AND r.id = ?`
		args = append(args, roundID)
	}
	args = append(args, limit, offset)
	return listFindings(ctx, s.db, where+` ORDER BY CASE f.severity WHEN 'blocking' THEN 0 WHEN 'should_fix' THEN 1 ELSE 2 END, r.number, f.number LIMIT ? OFFSET ?`, args...)
}

// ListAgentComments returns a bounded discussion page in creation order.
func (s *Store) ListAgentComments(ctx context.Context, issueID, findingID string, limit, offset int) ([]Comment, error) {
	if (issueID == "") == (findingID == "") {
		return nil, invalid("target", "must name exactly one issue or finding")
	}
	if limit < 1 || limit > 101 || offset < 0 {
		return nil, invalid("page", "is invalid")
	}
	column, target := "issue_id", issueID
	if findingID != "" {
		column, target = "finding_id", findingID
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+commentColumns+` FROM comments WHERE `+column+` = ? ORDER BY created_at, id LIMIT ? OFFSET ?`, target, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list agent comments: %w", err)
	}
	defer rows.Close()
	comments := []Comment{}
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}

// ListAgentProjects returns a bounded project page in key order.
func (s *Store) ListAgentProjects(ctx context.Context, limit, offset int) ([]Project, error) {
	return agentRows(ctx, s.db, `SELECT `+projectColumns+` FROM projects ORDER BY key LIMIT ? OFFSET ?`, scanProject, limit, offset)
}

// ListAgentSubissues returns a bounded subissue page in number order.
func (s *Store) ListAgentSubissues(ctx context.Context, issueID string, limit, offset int) ([]Subissue, error) {
	return agentRows(ctx, s.db, subissueSelect+` WHERE issue_id = ? ORDER BY number LIMIT ? OFFSET ?`, scanSubissue, issueID, limit, offset)
}

// ListAgentRounds returns a bounded review page in number order.
func (s *Store) ListAgentRounds(ctx context.Context, issueID string, limit, offset int) ([]ReviewRound, error) {
	return agentRows(ctx, s.db, `SELECT `+roundColumns+` FROM review_rounds WHERE issue_id = ? ORDER BY number LIMIT ? OFFSET ?`, scanRound, issueID, limit, offset)
}

// agentRows scans a bounded query without exposing database access outside the store.
func agentRows[T any](ctx context.Context, q querier, query string, scan func(scanner) (T, error), args ...any) ([]T, error) {
	limit, ok := args[len(args)-2].(int)
	offset, offsetOK := args[len(args)-1].(int)
	if !ok || !offsetOK || limit < 1 || limit > 101 || offset < 0 {
		return nil, invalid("page", "is invalid")
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read agent page: %w", err)
	}
	defer rows.Close()
	outputs := []T{}
	for rows.Next() {
		output, err := scan(rows)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read agent page: %w", err)
	}
	return outputs, nil
}
