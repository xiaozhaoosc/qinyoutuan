package sbproc

import (
	"os"
	"testing"
)

func TestArgoHostCache(t *testing.T) {
	ClearArgoHost("it")
	if got := ArgoHost("it"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	SetArgoHost("it", "a.trycloudflare.com")
	if got := ArgoHost("it"); got != "a.trycloudflare.com" {
		t.Fatalf("expected host, got %q", got)
	}
	SetArgoHost("it", "") // empty clears
	if got := ArgoHost("it"); got != "" {
		t.Fatalf("empty set should clear, got %q", got)
	}
}

func TestArgoSpecHost(t *testing.T) {
	t.Run("fixed uses domain", func(t *testing.T) {
		if got := argoSpecHost(ArgoSpec{Mode: "fixed", Domain: "t.example.com"}, ""); got != "t.example.com" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("temporary parses logfile", func(t *testing.T) {
		f := t.TempDir() + "/tunnel.log"
		if err := writeTestTunnelLog(f); err != nil {
			t.Fatal(err)
		}
		got := argoSpecHost(ArgoSpec{Mode: "temporary", TargetPort: 2053}, f)
		if got != "green-cat.trycloudflare.com" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("temporary without logfile", func(t *testing.T) {
		if got := argoSpecHost(ArgoSpec{Mode: "temporary"}, ""); got != "" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("disabled", func(t *testing.T) {
		if got := argoSpecHost(ArgoSpec{Mode: ""}, ""); got != "" {
			t.Fatalf("got %q", got)
		}
	})
}

func writeTestTunnelLog(path string) error {
	return os.WriteFile(path, []byte("Your quick tunnel has been created! Visit it at https://green-cat.trycloudflare.com\n"), 0o644)
}