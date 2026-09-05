package cache

import (
	"context"
	"testing"
	"time"
)

// storeContract exercises the behaviour every Store implementation must have.
func storeContract(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()

	if _, ok, err := s.Get(ctx, "absent"); err != nil || ok {
		t.Errorf("Get(absent) = ok:%v err:%v, want false/nil", ok, err)
	}

	if err := s.Set(ctx, "k1", []byte(`{"title":"x"}`), time.Minute); err != nil {
		t.Fatal(err)
	}
	val, ok, err := s.Get(ctx, "k1")
	if err != nil || !ok {
		t.Fatalf("Get(k1) = ok:%v err:%v, want hit", ok, err)
	}
	if string(val) != `{"title":"x"}` {
		t.Errorf("value = %q", val)
	}

	// A stored value must not outlive its TTL.
	if err := s.Set(ctx, "k2", []byte("v"), 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	if _, ok, _ := s.Get(ctx, "k2"); ok {
		t.Error("expired entry still served")
	}
}

func TestMemoryStoreContract(t *testing.T) {
	s := NewMemory(100)
	defer s.Close()
	storeContract(t, s)
}

func TestMemoryEvictsAtCapacity(t *testing.T) {
	s := NewMemory(3)
	defer s.Close()
	ctx := context.Background()
	for _, k := range []string{"a", "b", "c", "d", "e"} {
		if err := s.Set(ctx, k, []byte(k), time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if n := s.Len(); n > 3 {
		t.Errorf("Len = %d, want <= 3 (cap must bound memory)", n)
	}
	// The most recent write must survive eviction.
	if _, ok, _ := s.Get(ctx, "e"); !ok {
		t.Error("newest entry was evicted")
	}
}

func TestKeyVariesWithEveryInput(t *testing.T) {
	base := Key("https://x.com", "English", "fp1")
	if base == Key("https://y.com", "English", "fp1") {
		t.Error("key ignores the URL")
	}
	if base == Key("https://x.com", "Hebrew", "fp1") {
		t.Error("key ignores lang; a Hebrew request would serve an English label")
	}
	if base == Key("https://x.com", "English", "fp2") {
		t.Error("key ignores the config fingerprint; stale entries would survive a config change")
	}
	if base != Key("https://x.com", "English", "fp1") {
		t.Error("key is not deterministic")
	}
	if len(base) < 16 || base[:len("linkmeta:v1:")] != "linkmeta:v1:" {
		t.Errorf("key = %q, want a namespaced, versioned key", base)
	}
}
