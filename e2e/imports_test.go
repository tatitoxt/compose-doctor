package e2e

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBinaryCannotReachNetworkOrProcesses enforces the offline guarantee at
// the dependency level: the binary must not link packages that open network
// connections or start processes.
func TestBinaryCannotReachNetworkOrProcesses(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "../cmd/compose-doctor").Output()
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{"net": true, "net/http": true, "os/exec": true, "plugin": true, "crypto/tls": true, "net/rpc": true}
	for _, pkg := range strings.Fields(string(out)) {
		if forbidden[pkg] {
			t.Errorf("compose-doctor depends on %s", pkg)
		}
	}
}
