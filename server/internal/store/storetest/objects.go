// Package storetest provides an in-memory bucket for tests of code that attaches files.
package storetest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/r2"
)

// Objects keeps objects in memory and stands in for the attachments bucket.
type Objects struct {
	mu   sync.Mutex
	data map[string][]byte
	// sizes overrides the reported size of objects made with PutSize.
	sizes map[string]int64
}

// NewObjects returns an empty bucket.
func NewObjects() *Objects { return &Objects{data: map[string][]byte{}, sizes: map[string]int64{}} }

// Put stores an object, as a finished upload would.
func (o *Objects) Put(key string, data []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.data[key] = append([]byte(nil), data...)
}

// PutSize stores an object whose first bytes are head and whose reported size is size, to stand in
// for a large upload without holding its bytes.
func (o *Objects) PutSize(key string, head []byte, size int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.data[key] = append([]byte(nil), head...)
	o.sizes[key] = size
}

// Has reports whether the bucket holds the object.
func (o *Objects) Has(key string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.data[key]
	return ok
}

func (o *Objects) PresignPut(key, contentType string, _ time.Duration) string {
	return "https://objects.test/put/" + key + "?type=" + contentType
}

func (o *Objects) PresignGet(key, contentType, disposition string, _ time.Duration) string {
	return "https://objects.test/get/" + key + "?type=" + contentType + "&disposition=" + disposition
}

func (o *Objects) Head(_ context.Context, key string) (int64, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	data, ok := o.data[key]
	if !ok {
		return 0, fmt.Errorf("inspect %s: %w", key, r2.ErrNotFound)
	}
	if size, ok := o.sizes[key]; ok {
		return size, nil
	}
	return int64(len(data)), nil
}

func (o *Objects) GetPrefix(_ context.Context, key string, limit int64) ([]byte, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	data, ok := o.data[key]
	if !ok {
		return nil, fmt.Errorf("read %s: %w", key, r2.ErrNotFound)
	}
	if int64(len(data)) > limit {
		data = data[:limit]
	}
	return append([]byte(nil), data...), nil
}

func (o *Objects) Delete(_ context.Context, key string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.data, key)
	delete(o.sizes, key)
	return nil
}
