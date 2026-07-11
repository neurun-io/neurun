//go:build linux

package main

import (
	"log"

	"github.com/dagflows/worker/internal/agent"
)

func main() {
	if err := agent.Run(); err != nil {
		log.Fatal(err)
	}
}
