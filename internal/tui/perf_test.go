package tui

import "testing"

// TestDevicePagesContract checks what every Performance page must provide,
// so a new device kind can't ship without help text or with a clashing key.
func TestDevicePagesContract(t *testing.T) {
	m := sampled(t, &fakeSampler{})
	seen := map[string]bool{}
	for _, p := range m.pages(true) {
		k := p.key()
		if seen[k] {
			t.Errorf("duplicate page key %q", k)
		}
		seen[k] = true
		if p.name() == "" || p.helpTitle() == "" {
			t.Errorf("%s: missing name or help title", k)
		}
		if len(p.terms()) == 0 {
			t.Errorf("%s: no ? explanations", k)
		}
		if len(p.detail(80, 30)) == 0 {
			t.Errorf("%s: empty detail page", k)
		}
	}
	if len(seen) != 7 { // cpu, memory, disk, Wi-Fi, hidden VPN, 2 GPUs
		t.Errorf("want 7 pages including hidden, got %d", len(seen))
	}
}
