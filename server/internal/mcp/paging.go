package mcp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/shantodotdev/super-sparrow/server/internal/store"
)

// pageArgs bounds list responses. Cursors are scoped to the tool and its filters.
type pageArgs struct {
	Limit  *int   `json:"limit"`
	Cursor string `json:"cursor"`
}

// Default page sizes, used when a list tool gets no limit. Tools whose rows carry discussions
// default to smaller pages; a cursor continues any list.
const (
	pageSizeRows       = 50
	pageSizeDiscussion = 20
)

// bounds validates the page size and decodes a cursor for exactly this query scope. fallback is
// the page size when the caller sent no limit.
func (p pageArgs) bounds(scope any, fallback int) (int, int, string, error) {
	raw, _ := json.Marshal(scope)
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	limit := fallback
	if p.Limit != nil {
		limit = *p.Limit
	}
	if limit < 1 || limit > 100 {
		return 0, 0, "", &store.ValidationError{Field: "limit", Message: "must be between 1 and 100"}
	}
	offset := 0
	if p.Cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(p.Cursor)
		parts := strings.Split(string(decoded), ":")
		if err != nil || len(parts) != 2 || parts[0] != hash {
			return 0, 0, "", &store.ValidationError{Field: "cursor", Message: "does not belong to this query; restart the list"}
		}
		offset, err = strconv.Atoi(parts[1])
		if err != nil || offset < 0 || offset > 1000000000 {
			return 0, 0, "", &store.ValidationError{Field: "cursor", Message: "is invalid"}
		}
	}
	return limit, offset, hash, nil
}

func nextCursor(hash string, offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%d", hash, offset)))
}

// pageSlice trims a bounded page plus one lookahead row and returns its continuation cursor.
func pageSlice[T any](items []T, limit, offset int, hash string) ([]T, string) {
	if len(items) > limit {
		return items[:limit], nextCursor(hash, offset+limit)
	}
	if items == nil {
		items = []T{}
	}
	return items, ""
}
