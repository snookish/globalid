package globalid_test

import (
	"sync"
	"testing"
	"time"

	"github.com/snookish/globalid"
)

type MockClock struct {
	current time.Time
	mu      sync.Mutex
}

func (m *MockClock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

func (m *MockClock) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = m.current.Add(d)
}

func (m *MockClock) Set(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = t
}

func TestBasicGeneration(t *testing.T) {
	gen, err := globalid.NewGenerator(globalid.Config{
		MachineID: 1,
	})
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	id1, err := gen.Generate()
	if err != nil {
		t.Fatalf("Failed to generate ID: %v", err)
	}

	id2, err := gen.Generate()
	if err != nil {
		t.Fatalf("Failed to generate ID: %v", err)
	}

	if id1 == id2 {
		t.Error("Generated duplicate IDs")
	}

	if id2.ID() <= id1.ID() {
		t.Error("IDs are not monotonically increasing")
	}
}

func TestUniquenessConcurrent(t *testing.T) {
	gen, _ := globalid.NewGenerator(globalid.Config{
		MachineID:   1,
		WaitForTime: true,
	})

	const numGoroutines = 100
	const idsPerGoroutine = 1000

	var wg sync.WaitGroup
	idChan := make(chan int64, numGoroutines*idsPerGoroutine)

	for range numGoroutines {
		wg.Go(func() {
			for range idsPerGoroutine {
				id, err := gen.Generate()
				if err != nil {
					t.Errorf("Failed to generate ID: %v", err)
					return
				}
				idChan <- id.ID()
			}
		})
	}

	wg.Wait()
	close(idChan)

	seen := make(map[int64]bool)
	for id := range idChan {
		if seen[id] {
			t.Fatalf("Duplicate ID found: %d", id)
		}
		seen[id] = true
	}

	expectedCount := numGoroutines * idsPerGoroutine
	if len(seen) != expectedCount {
		t.Errorf("Expected %d IDs, got %d", expectedCount, len(seen))
	}
}

func TestClockBackwards(t *testing.T) {
	clock := &MockClock{current: time.Now()}

	gen, _ := globalid.NewGenerator(globalid.Config{
		MachineID:    1,
		ClockChecker: clock,
		WaitForTime:  false,
	})

	_, err := gen.Generate()
	if err != nil {
		t.Fatalf("Failed to generate first ID: %v", err)
	}

	// Move clock backwards
	clock.Advance(-time.Second)

	// Should get an error
	_, err = gen.Generate()
	if err == nil {
		t.Error("Expected error when clock moves backwards")
	}
}

func TestSequenceExhaustion(t *testing.T) {
	clock := &MockClock{current: time.Now()}

	gen, _ := globalid.NewGenerator(globalid.Config{
		MachineID:    1,
		ClockChecker: clock,
		WaitForTime:  false,
	})

	// Generate max sequence number of IDs in same millisecond
	for i := 0; i <= 4096; i++ {
		_, err := gen.Generate()
		if i < 4096 && err != nil {
			t.Fatalf("Unexpected error at sequence %d: %v", i, err)
		}
		if i == 4096 && err == nil {
			t.Error("Expected sequence exhaustion error")
		}
	}
}

func TestParseID(t *testing.T) {
	gen, _ := globalid.NewGenerator(globalid.Config{
		MachineID: 42,
	})

	id, _ := gen.Generate()
	timestamp, machineID, sequence := id.Parse()

	if machineID != 42 {
		t.Errorf("Expected machine ID 42, got %d", machineID)
	}

	// Timestamp should be very recent
	timeDiff := time.Since(timestamp)
	if timeDiff > time.Second || timeDiff < 0 {
		t.Errorf("Timestamp not recent: %v ago", timeDiff)
	}

	// First ID in a millisecond should have sequence 0
	if sequence != 0 && sequence > 4095 {
		t.Errorf("Invalid sequence number: %d", sequence)
	}
}

func TestBatchGeneration(t *testing.T) {
	gen, _ := globalid.NewGenerator(globalid.Config{
		MachineID: 1,
	})

	ids, err := gen.GenerateBatch(100)
	if err != nil {
		t.Fatalf("Failed to generate batch: %v", err)
	}

	if len(ids) != 100 {
		t.Errorf("Expected 100 IDs, got %d", len(ids))
	}

	seen := make(map[int64]bool)
	for i, id := range ids {
		if seen[id.ID()] {
			t.Errorf("Duplicate ID in batch: %d", id)
		}
		seen[id.ID()] = true

		if i > 0 && id.ID() <= ids[i-1].ID() {
			t.Error("Batch IDs not monotonically increasing")
		}
	}
}

func TestMultipleMachines(t *testing.T) {
	const numMachines = 10
	const idsPerMachine = 1000

	var wg sync.WaitGroup
	idChan := make(chan int64, numMachines*idsPerMachine)

	for machineID := range numMachines {
		wg.Add(1)
		go func(mid int) {
			defer wg.Done()

			gen, err := globalid.NewGenerator(globalid.Config{
				MachineID: int64(mid),
			})
			if err != nil {
				t.Errorf("Failed to create generator for machine %d: %v", mid, err)
				return
			}

			for range idsPerMachine {
				id, err := gen.Generate()
				if err != nil {
					t.Errorf("Machine %d failed to generate ID: %v", mid, err)
					return
				}
				idChan <- id.ID()
			}
		}(machineID)
	}

	wg.Wait()
	close(idChan)

	seen := make(map[int64]bool)
	for id := range idChan {
		if seen[id] {
			t.Fatalf("Duplicate ID across machines: %d", id)
		}
		seen[id] = true
	}

	if len(seen) != numMachines*idsPerMachine {
		t.Errorf("Expected %d unique IDs, got %d",
			numMachines*idsPerMachine, len(seen))
	}
}

func BenchmarkGenerate(b *testing.B) {
	gen, _ := globalid.NewGenerator(globalid.Config{
		MachineID: 1,
	})

	for b.Loop() {
		_, _ = gen.Generate()
	}
}

func BenchmarkGenerateConcurrent(b *testing.B) {
	gen, _ := globalid.NewGenerator(globalid.Config{
		MachineID:   1,
		WaitForTime: true,
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = gen.Generate()
		}
	})
}
