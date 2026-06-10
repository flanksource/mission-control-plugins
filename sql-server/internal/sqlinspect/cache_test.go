package sqlinspect

import (
	"testing"
	"time"

	"github.com/flanksource/arch-unit/models/uir"
)

func TestCacheKeyEscapesParts(t *testing.T) {
	left := CacheKey("a/b", "c")
	right := CacheKey("a", "b/c")
	if left == right {
		t.Fatalf("cache keys collided: %q", left)
	}
	if left != "a%2Fb/c" {
		t.Fatalf("left key = %q, want a%%2Fb/c", left)
	}
	if right != "a/b%2Fc" {
		t.Fatalf("right key = %q, want a/b%%2Fc", right)
	}
}

func TestCacheLoadCachesUntilRefresh(t *testing.T) {
	cache := NewCache(time.Hour)
	calls := 0
	extract := func() (uir.UIR, error) {
		calls++
		return uir.UIR{Records: nil}, nil
	}

	if _, err := cache.Load("cfg/db", false, extract); err != nil {
		t.Fatalf("load first: %v", err)
	}
	if _, err := cache.Load("cfg/db", false, extract); err != nil {
		t.Fatalf("load second: %v", err)
	}
	if calls != 1 {
		t.Fatalf("extract called %d times, want 1", calls)
	}

	if _, err := cache.Load("cfg/db", true, extract); err != nil {
		t.Fatalf("load refresh: %v", err)
	}
	if calls != 2 {
		t.Fatalf("extract called %d times after refresh, want 2", calls)
	}
}

func TestCacheLoadExpires(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cache := NewCache(time.Hour)
	cache.now = func() time.Time { return now }
	calls := 0
	extract := func() (uir.UIR, error) {
		calls++
		return uir.UIR{}, nil
	}

	if _, err := cache.Load("cfg/db", false, extract); err != nil {
		t.Fatalf("load first: %v", err)
	}
	now = now.Add(2 * time.Hour)
	if _, err := cache.Load("cfg/db", false, extract); err != nil {
		t.Fatalf("load after expiry: %v", err)
	}
	if calls != 2 {
		t.Fatalf("extract called %d times, want 2", calls)
	}
}

func TestExtractNilDBReturnsClearError(t *testing.T) {
	_, err := Extract(nil)
	if err == nil || err.Error() != "nil db" {
		t.Fatalf("got error %v, want nil db", err)
	}
}
