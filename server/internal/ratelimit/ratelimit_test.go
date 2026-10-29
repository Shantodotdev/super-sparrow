package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterRefusesOverTheLimitUntilTheWindowPasses(t *testing.T) {
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	limiter := New(2, time.Minute, func() time.Time { return now })

	for i := range 2 {
		if ok, _ := limiter.Allow("a"); !ok {
			t.Fatalf("hit %d refused", i+1)
		}
		now = now.Add(10 * time.Second)
	}

	ok, retry := limiter.Allow("a")
	if ok {
		t.Fatal("third hit allowed")
	}
	if retry != 40*time.Second {
		t.Errorf("retry after = %v, want 40s", retry)
	}

	if ok, _ := limiter.Allow("b"); !ok {
		t.Fatal("another key was limited")
	}

	now = now.Add(41 * time.Second)
	if ok, _ := limiter.Allow("a"); !ok {
		t.Fatal("hit refused after the oldest one left the window")
	}
}

func TestLimiterForgetsQuietKeys(t *testing.T) {
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	limiter := New(1, time.Minute, func() time.Time { return now })

	limiter.Allow("quiet")
	now = now.Add(2 * time.Minute)
	for range sweepEvery {
		limiter.Allow("busy")
	}

	if _, kept := limiter.hits["quiet"]; kept {
		t.Error("a key with no recent hits was kept")
	}
}

func TestCheckAndRecordCountOnlyWhatIsRecorded(t *testing.T) {
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	limiter := New(2, time.Minute, func() time.Time { return now })

	for range 5 {
		if ok, _ := limiter.Check("a"); !ok {
			t.Fatal("checks alone used up the limit")
		}
	}

	limiter.Record("a")
	now = now.Add(15 * time.Second)
	limiter.Record("a")
	if ok, retry := limiter.Check("a"); ok || retry != 45*time.Second {
		t.Fatalf("Check after two records = %v, %v", ok, retry)
	}

	now = now.Add(46 * time.Second)
	if ok, _ := limiter.Check("a"); !ok {
		t.Fatal("still limited after the first record left the window")
	}
}
