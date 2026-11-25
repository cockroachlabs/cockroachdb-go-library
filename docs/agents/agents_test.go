// Package agents holds the consumer-facing AGENTS.md shipped with the module.
// This test asserts the file is present at its tagged path so `go get` consumers
// always receive it in the module zip served by the proxy.
package agents

import (
	"os"
	"testing"
)

func TestAgentsMarkdownShipsWithModule(t *testing.T) {
	info, err := os.Stat("AGENTS.md")
	if err != nil {
		t.Fatalf("AGENTS.md must exist at docs/agents/AGENTS.md so it ships with go get: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("AGENTS.md is empty")
	}
}
