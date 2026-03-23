package globalid

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	machineIDBits = 10
	sequenceBits  = 12

	maxMachineID = -1 ^ (-1 << machineIDBits) // 1023 (2^10 - 1)
	maxSequence  = -1 ^ (-1 << sequenceBits)  // 4095 (2^12 - 1)

	machineIDShift = sequenceBits                 // 12
	timestampShift = sequenceBits + machineIDBits // 22

	// Custom epoch (Jan 1, 2026 00:00:00 UTC) in milliseconds
	customEpoch = int64(1767225600000)
)

var (
	ErrClockBackwards    = errors.New("clock moved backwards")
	ErrInvalidMachineID  = errors.New("machine ID must be between 0 and 1023")
	ErrSequenceExhausted = errors.New("sequence exhausted for current millisecond")
)

type ID struct {
	val int64
}

// Generator creates globally unique, roughly monotonic IDs
type Generator struct {
	mu            sync.Mutex
	machineID     int64
	sequence      int64
	lastTimestamp int64

	// If true, wait when sequence exhausted instead of erroring
	waitForTime  bool
	clockChecker ClockChecker
}

// ClockChecker allows for custom time sources
type ClockChecker interface {
	Now() time.Time
}

// StandardClock uses the system clock
type StandardClock struct{}

func (c StandardClock) Now() time.Time {
	return time.Now()
}

// Config holds generator configuration options
type Config struct {
	ClockChecker  ClockChecker // Custom clock source
	MachineID     int64        // Explicitly set machine ID
	AutoMachineID bool         // Auto-generate machine ID from MAC address
	WaitForTime   bool         // Wait instead of error on sequence exhaustion
}

// NewGenerator creates a new ID generator with the given configuration
func NewGenerator(config Config) (*Generator, error) {
	machineID := config.MachineID
	if config.AutoMachineID {
		var err error
		machineID, err = generateMachineID()
		if err != nil {
			return nil, fmt.Errorf("failed to auto-generate machine ID: %w", err)
		}
	}

	if machineID < 0 || machineID > maxMachineID {
		return nil, ErrInvalidMachineID
	}

	clock := config.ClockChecker
	if clock == nil {
		clock = StandardClock{}
	}

	return &Generator{
		sequence:      0,
		lastTimestamp: -1,
		clockChecker:  clock,
		machineID:     machineID,
		waitForTime:   config.WaitForTime,
	}, nil
}

// Generate creates a new globally unique ID
func (g *Generator) Generate() (*ID, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	timestamp := g.currentTimestamp()

	// Check for clock moving backwards
	if timestamp < g.lastTimestamp {
		if g.waitForTime {
			// Wait until we catch up to the last timestamp
			sleepTime := time.Duration(g.lastTimestamp-timestamp) * time.Millisecond
			time.Sleep(sleepTime)
			timestamp = g.currentTimestamp()
		} else {
			return nil, fmt.Errorf("%w: last=%d, now=%d", ErrClockBackwards, g.lastTimestamp, timestamp)
		}
	}

	// Same millisecond: increment sequence
	if timestamp == g.lastTimestamp {
		g.sequence = (g.sequence + 1) & maxSequence

		// Sequence overflow: we've generated >4096 IDs this millisecond
		if g.sequence == 0 {
			if g.waitForTime {
				timestamp = g.waitNextMillis(timestamp)
			} else {
				return nil, ErrSequenceExhausted
			}
		}
	} else {
		g.sequence = 0
	}

	g.lastTimestamp = timestamp

	// Layout: [1 bit sign][41 bits timestamp][10 bits machine][12 bits sequence]
	id := (timestamp << timestampShift) | (g.machineID << machineIDShift) | g.sequence
	return &ID{val: id}, nil
}

// GenerateBatch creates multiple IDs efficiently
func (g *Generator) GenerateBatch(count int) ([]ID, error) {
	if count <= 0 {
		return nil, errors.New("count must be positive")
	}

	ids := make([]ID, count)
	for i := range count {
		id, err := g.Generate()
		if err != nil {
			return nil, fmt.Errorf("failed to generate ID %d of %d: %w", i+1, count, err)
		}
		ids[i] = *id
	}
	return ids, nil
}

func (g *Generator) currentTimestamp() int64 {
	return g.clockChecker.Now().UnixMilli() - customEpoch
}

func (g *Generator) waitNextMillis(lastTimestamp int64) int64 {
	timestamp := g.currentTimestamp()
	for timestamp <= lastTimestamp {
		time.Sleep(time.Microsecond * 100)
		timestamp = g.currentTimestamp()
	}
	return timestamp
}

// generateMachineID creates a machine ID from the MAC address
func generateMachineID() (int64, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return 0, err
	}

	for _, iFace := range interfaces {
		if len(iFace.HardwareAddr) == 0 {
			continue
		}

		// Using the last 10 bits of the MAC address
		mac := iFace.HardwareAddr
		id := int64(mac[len(mac)-2])<<8 | int64(mac[len(mac)-1])
		return id & maxMachineID, nil
	}

	return 0, errors.New("no network interface with MAC address found")
}

func (id *ID) ID() int64 {
	if id == nil {
		return 0
	}
	return id.val
}

func (id *ID) String() string {
	if id == nil {
		return ""
	}
	return strconv.Itoa(int(id.val))
}

func (id *ID) GoString() string {
	if id == nil {
		return ""
	}
	timestamp, machineID, sequence := id.Parse()
	return fmt.Sprintf("ID{time: %s, machine: %d, seq: %d}", timestamp.Format(time.RFC3339), machineID, sequence)
}

// ParseID extracts the components from a generated ID
func (id *ID) Parse() (time.Time, int64, int64) {
	if id == nil {
		return time.Time{}, 0, 0
	}

	sequence := id.val & maxSequence
	machineID := (id.val >> machineIDShift) & maxMachineID
	timestampMillis := (id.val >> timestampShift) + customEpoch
	timestamp := time.Unix(0, timestampMillis*int64(time.Millisecond))

	return timestamp, machineID, sequence
}
