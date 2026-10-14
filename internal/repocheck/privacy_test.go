// Package repocheck holds checks over the repository itself.
package repocheck

import (
	"bufio"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This repo is public. Test fixtures, examples and docs must use synthetic
// data (see "Code standards" in ROADMAP.md). These checks catch the
// mechanical leaks: real IP addresses and email addresses. Names and other
// personal details can't be detected automatically; review for those.

var (
	ipv4RE  = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	emailRE = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)
)

// Addresses reserved for documentation (RFC 5737) and loopback can't
// identify anyone.
var allowedIPs = []netip.Prefix{
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("0.0.0.0/32"),
}

func allowedEmail(e string) bool {
	return strings.HasSuffix(e, "@example.com") || strings.HasSuffix(e, ".example.com")
}

func TestNoRealAddressesInRepo(t *testing.T) {
	root := repoRoot(t)
	out, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	for _, rel := range strings.Fields(string(out)) {
		if rel == "go.sum" { // module hashes, not data
			continue
		}
		f, err := os.Open(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for n := 1; sc.Scan(); n++ {
			line := sc.Text()
			for _, ip := range ipv4RE.FindAllString(line, -1) {
				if a, err := netip.ParseAddr(ip); err == nil && !allowedIP(a) {
					t.Errorf("%s:%d: real-looking IP address %s (use 192.0.2.x, 198.51.100.x or 203.0.113.x)", rel, n, ip)
				}
			}
			for _, e := range emailRE.FindAllString(line, -1) {
				if !allowedEmail(e) {
					t.Errorf("%s:%d: email address %s (use an @example.com address)", rel, n, e)
				}
			}
		}
		f.Close()
	}
}

func allowedIP(a netip.Addr) bool {
	for _, p := range allowedIPs {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skipf("not in a git checkout: %v", err)
	}
	return strings.TrimSpace(string(out))
}
