package redis

import (
	"bufio"
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type hostCapacityGate struct {
	minFreeMemoryMB int64
}

func newHostCapacityGate(minFreeMemoryMB int64) hostCapacityGate {
	return hostCapacityGate{minFreeMemoryMB: minFreeMemoryMB}
}

func (g hostCapacityGate) Wait(ctx context.Context) error {
	if g.minFreeMemoryMB <= 0 {
		return nil
	}
	for {
		free, ok := freeMemoryMB()
		if !ok || free >= g.minFreeMemoryMB {
			return nil
		}
		log.Printf("host capacity gate waiting free_memory_mb=%d min_free_memory_mb=%d", free, g.minFreeMemoryMB)
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func freeMemoryMB() (int64, bool) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return kb / 1024, true
	}
	return 0, false
}
