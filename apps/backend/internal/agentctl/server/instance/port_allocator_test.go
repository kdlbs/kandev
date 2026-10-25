package instance

import (
	"fmt"
	"sync"
	"testing"
)

func TestPortAllocatorReusesOwnerReservation(t *testing.T) {
	allocator := NewPortAllocator(41001, 41002)

	first, err := allocator.Allocate("owner-a")
	if err != nil {
		t.Fatalf("first Allocate: %v", err)
	}
	second, err := allocator.Allocate("owner-a")
	if err != nil {
		t.Fatalf("second Allocate: %v", err)
	}
	if second != first {
		t.Fatalf("second allocation returned lease %+v, want owner reservation %+v", second, first)
	}
}

func TestPortAllocatorStaleReleaseAfterReuse(t *testing.T) {
	allocator := NewPortAllocator(41001, 41001)

	stale, err := allocator.Allocate("old-owner")
	if err != nil {
		t.Fatalf("Allocate old owner: %v", err)
	}
	allocator.Release(stale)
	successor, err := allocator.Allocate("new-owner")
	if err != nil {
		t.Fatalf("Allocate successor: %v", err)
	}
	if successor.Port != stale.Port {
		t.Fatalf("successor port = %d, want reused port %d", successor.Port, stale.Port)
	}
	if successor.Generation == stale.Generation {
		t.Fatalf("successor reused generation %d", stale.Generation)
	}

	allocator.Release(stale)
	if _, err := allocator.Allocate("third-owner"); err == nil {
		t.Fatal("stale release freed the successor reservation")
	}
}

func TestPortAllocatorStaleMarkUnavailable(t *testing.T) {
	allocator := NewPortAllocator(41001, 41001)

	stale, err := allocator.Allocate("old-owner")
	if err != nil {
		t.Fatalf("Allocate old owner: %v", err)
	}
	allocator.Release(stale)
	successor, err := allocator.Allocate("new-owner")
	if err != nil {
		t.Fatalf("Allocate successor: %v", err)
	}

	allocator.MarkUnavailable(stale)
	allocator.Release(successor)
	reusable, err := allocator.Allocate("third-owner")
	if err != nil {
		t.Fatalf("stale mark-unavailable blocked released successor port: %v", err)
	}
	if reusable.Port != successor.Port {
		t.Fatalf("reused port = %d, want %d", reusable.Port, successor.Port)
	}
}

func TestPortAllocatorStaleMutationsPreserveSameOwnerGeneration(t *testing.T) {
	allocator := NewPortAllocator(41001, 41001)
	old, err := allocator.Allocate("owner-a")
	if err != nil {
		t.Fatalf("Allocate old lease: %v", err)
	}
	allocator.Release(old)
	current, err := allocator.Allocate("owner-a")
	if err != nil {
		t.Fatalf("Allocate current lease: %v", err)
	}
	if current.Generation == old.Generation {
		t.Fatalf("same owner reused generation %d", old.Generation)
	}

	allocator.Release(old)
	allocator.MarkUnavailable(old)
	if _, err := allocator.Allocate("owner-b"); err == nil {
		t.Fatal("stale mutation removed the current same-owner lease")
	}
	if got := allocator.owners["owner-a"]; got != current {
		t.Fatalf("owner index = %+v, want %+v", got, current)
	}
	if got := allocator.allocated[current.Port]; got != current {
		t.Fatalf("port index = %+v, want %+v", got, current)
	}
	if _, blocked := allocator.unavailable[current.Port]; blocked {
		t.Fatalf("stale mark-unavailable blocked port %d", current.Port)
	}
}

func TestPortAllocatorRejectsForgedAndAbsentLeases(t *testing.T) {
	allocator := NewPortAllocator(41001, 41001)
	lease, err := allocator.Allocate("owner-a")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	forgedOwner := lease
	forgedOwner.Owner = "owner-b"
	wrongGeneration := lease
	wrongGeneration.Generation++
	for _, forged := range []PortLease{forgedOwner, wrongGeneration, PortLease{}} {
		allocator.Release(forged)
		allocator.MarkUnavailable(forged)
		if got := allocator.owners[lease.Owner]; got != lease {
			t.Fatalf("forged mutation changed owner index: got %+v, want %+v", got, lease)
		}
		if got := allocator.allocated[lease.Port]; got != lease {
			t.Fatalf("forged mutation changed port index: got %+v, want %+v", got, lease)
		}
		if _, blocked := allocator.unavailable[lease.Port]; blocked {
			t.Fatalf("forged mutation blocked port %d", lease.Port)
		}
	}

	allocator.MarkUnavailable(lease)
	if len(allocator.owners) != 0 || len(allocator.allocated) != 0 {
		t.Fatalf("matching mark-unavailable left indexes: owners=%v allocated=%v", allocator.owners, allocator.allocated)
	}
	if _, blocked := allocator.unavailable[lease.Port]; !blocked {
		t.Fatalf("matching mark-unavailable did not block port %d", lease.Port)
	}
	if _, err := allocator.Allocate("owner-c"); err == nil {
		t.Fatal("matching mark-unavailable allowed the port to be reused")
	}
}

func TestPortAllocatorGenerationAndConcurrentExhaustion(t *testing.T) {
	allocator := NewPortAllocator(41001, 41064)
	const ownerCount = 64
	leases := make([]PortLease, ownerCount)
	var wg sync.WaitGroup
	for i := range ownerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lease, err := allocator.Allocate(fmt.Sprintf("owner-%d", i))
			if err != nil {
				t.Errorf("Allocate owner %d: %v", i, err)
				return
			}
			leases[i] = lease
		}()
	}
	wg.Wait()

	ports := make(map[int]struct{}, ownerCount)
	generations := make(map[uint64]struct{}, ownerCount)
	for _, lease := range leases {
		if lease.Generation == 0 {
			t.Fatal("allocated lease has generation zero")
		}
		if _, exists := ports[lease.Port]; exists {
			t.Fatalf("duplicate allocated port %d", lease.Port)
		}
		if _, exists := generations[lease.Generation]; exists {
			t.Fatalf("duplicate generation %d", lease.Generation)
		}
		ports[lease.Port] = struct{}{}
		generations[lease.Generation] = struct{}{}
	}
	if _, err := allocator.Allocate("exhausted-owner"); err == nil {
		t.Fatal("Allocate succeeded after every port was reserved")
	}

	sameOwnerAllocator := NewPortAllocator(41001, 41001)
	const sameOwnerCallers = 32
	sameOwnerLeases := make([]PortLease, sameOwnerCallers)
	for i := range sameOwnerCallers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lease, err := sameOwnerAllocator.Allocate("shared-owner")
			if err != nil {
				t.Errorf("Allocate shared owner: %v", err)
				return
			}
			sameOwnerLeases[i] = lease
		}()
	}
	wg.Wait()
	for _, lease := range sameOwnerLeases {
		if lease != sameOwnerLeases[0] {
			t.Fatalf("same-owner concurrent allocation = %+v, want %+v", lease, sameOwnerLeases[0])
		}
	}

	wrap := NewPortAllocator(41001, 41001)
	wrap.nextGen = ^uint64(0)
	if _, err := wrap.Allocate("overflow-owner"); err == nil {
		t.Fatal("Allocate succeeded after generation exhaustion")
	}
	if len(wrap.allocated) != 0 || len(wrap.owners) != 0 {
		t.Fatalf("generation exhaustion mutated indexes: allocated=%v owners=%v", wrap.allocated, wrap.owners)
	}
}
