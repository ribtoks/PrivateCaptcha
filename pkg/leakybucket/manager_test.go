package leakybucket

import (
	"net/netip"
	"sync"
	"testing"
	"time"
)

func TestManagerAdd(t *testing.T) {
	const maxBuckets = 8
	const cap = 5
	const key = 123

	manager := NewManager[int32, ConstLeakyBucket[int32]](maxBuckets, cap, 1*time.Second)
	tnow := time.Now().Truncate(1 * time.Second)

	for i := 0; i < cap; i++ {
		result := manager.Add(key, 1, tnow)
		if result.CurrLevel != uint32(i+1) {
			t.Errorf("Unexpected level: %v", result.CurrLevel)
		}
		if result.Added != 1 {
			t.Errorf("Failed to add to bucket")
		}
	}
}

func TestManagerAddParallel(t *testing.T) {
	const maxBuckets = 8
	const cap = 5
	const key = 123

	manager := NewManager[int32, ConstLeakyBucket[int32]](maxBuckets, cap, 1*time.Second)
	tnow := time.Now().Truncate(1 * time.Second)

	var wg sync.WaitGroup

	for i := 0; i < cap; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			result := manager.Add(key, 1, tnow)
			if result.Added != 1 {
				t.Errorf("Failed to add to bucket")
			}
		}()
	}

	wg.Wait()

	result := manager.Add(key, 1, tnow)
	if result.CurrLevel != cap {
		t.Errorf("Unexpected level after full: %v", result.CurrLevel)
	}
	if result.Added != 0 {
		t.Errorf("Was able to add to the bucket after")
	}
}

func TestManagerAddDefault(t *testing.T) {
	const maxBuckets = 8
	const cap = 5
	const key = 123

	manager := NewManager[int32, ConstLeakyBucket[int32]](maxBuckets, cap, 1*time.Second)
	tnow := time.Now().Truncate(1 * time.Second)

	for i := 0; i < cap; i++ {
		result := manager.Add(key, 1, tnow)
		if result.CurrLevel != uint32(i+1) {
			t.Errorf("Unexpected level: %v", result.CurrLevel)
		}
		if result.Added != 1 {
			t.Errorf("Failed to add to bucket")
		}
	}

	result := manager.Add(key, 1, tnow)
	if result.CurrLevel != cap {
		t.Errorf("Unexpected level after full: %v", result.CurrLevel)
	}
	if result.Added != 0 {
		t.Errorf("Managed to add to full bucket")
	}
}

func TestManagerIPAddrAddDefault(t *testing.T) {
	const maxBuckets = 8
	const cap = 5
	manager := NewManager[netip.Addr, ConstLeakyBucket[netip.Addr]](maxBuckets, cap, 1*time.Second)

	tnow := time.Now().Truncate(1 * time.Second)

	key := netip.Addr{}
	for i := 0; i < cap; i++ {
		result := manager.Add(key, 1, tnow)
		if result.CurrLevel != uint32(i+1) {
			t.Errorf("Unexpected level: %v", result.CurrLevel)
		}
		if result.Added != 1 {
			t.Errorf("Failed to add to bucket")
		}
	}

	result := manager.Add(key, 1, tnow)
	if result.CurrLevel != cap {
		t.Errorf("Unexpected level after full: %v", result.CurrLevel)
	}
	if result.Added != 0 {
		t.Errorf("Managed to add to full bucket")
	}
}
