package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisStoreContract(t *testing.T) {
	mr := miniredis.RunT(t)
	s, err := NewRedis("redis://"+mr.Addr(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	if _, ok, err := s.Get(ctx, "absent"); err != nil || ok {
		t.Errorf("Get(absent) = ok:%v err:%v, want false/nil", ok, err)
	}
	if err := s.Set(ctx, "k1", []byte(`{"title":"x"}`), time.Minute); err != nil {
		t.Fatal(err)
	}
	val, ok, err := s.Get(ctx, "k1")
	if err != nil || !ok || string(val) != `{"title":"x"}` {
		t.Fatalf("Get(k1) = %q ok:%v err:%v", val, ok, err)
	}

	// TTL must reach Redis, not just be tracked locally.
	if err := s.Set(ctx, "k2", []byte("v"), 30*time.Second); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(31 * time.Second)
	if _, ok, _ := s.Get(ctx, "k2"); ok {
		t.Error("expired key still served; TTL was not applied server-side")
	}
}

func TestRedisUnreachableReportsError(t *testing.T) {
	mr := miniredis.RunT(t)
	s, err := NewRedis("redis://"+mr.Addr(), 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	mr.Close() // server goes away mid-flight, as in a real outage

	if _, ok, err := s.Get(context.Background(), "k"); err == nil || ok {
		t.Error("want an error (treated as a miss by the caller), got a silent hit/miss")
	}
}

func TestRedisRejectsBadURL(t *testing.T) {
	if _, err := NewRedis("not-a-redis-url", time.Second); err == nil {
		t.Error("want an error for an unparseable REDIS_URL")
	}
}
