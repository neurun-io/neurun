package redis

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

const firecrackerOverheadMB int64 = 128

type memoryGate struct {
	mu       sync.Mutex
	reserved int64
}

func (g *memoryGate) Reserve(memoryMB int64) (func(), error) {
	if memoryMB <= 0 || memoryMB > (1<<63-1)-firecrackerOverheadMB {
		return nil, fmt.Errorf("invalid node memory limit: %d MB", memoryMB)
	}
	required := memoryMB + firecrackerOverheadMB
	g.mu.Lock()
	defer g.mu.Unlock()

	available, ok := freeMemoryMB()
	if !ok {
		return nil, fmt.Errorf("cannot determine available host memory")
	}
	remaining := max(int64(0), available-g.reserved)
	if required > remaining {
		return nil, fmt.Errorf("insufficient host memory: need %d MB, available %d MB", required, remaining)
	}
	g.reserved += required
	return func() {
		g.mu.Lock()
		g.reserved -= required
		g.mu.Unlock()
	}, nil
}

func freeMemoryMB() (int64, bool) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[0] != "MemAvailable:" {
			continue
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		return kb / 1024, err == nil
	}
	return 0, false
}
