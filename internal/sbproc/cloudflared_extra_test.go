package sbproc

import (
	"context"
	"os"
	"strings"
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

func TestParseArgoMarkers(t *testing.T) {
	changed, host := parseArgoMarkers("up\nARGO_CHANGED=1\nARGO_HOST=abc.trycloudflare.com\n")
	if !changed || host != "abc.trycloudflare.com" {
		t.Fatalf("got changed=%v host=%q", changed, host)
	}
	changed, host = parseArgoMarkers("ARGO_CHANGED=0\nARGO_HOST=\n")
	if changed || host != "" {
		t.Fatalf("got changed=%v host=%q", changed, host)
	}
}

func TestRemoteEnsureScript(t *testing.T) {
	spec := ArgoSpec{Mode: "temporary", TargetPort: 20086}
	s, err := RemoteEnsureScript(spec, "argo-remote", "/usr/local/bin/cloudflared", "/etc/systemd/system", "/etc/cloudflared")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"cloudflared-argo-remote.service",
		"tunnel --no-autoupdate --url http://127.0.0.1:20086",
		"echo ARGO_CHANGED",
		"ARGO_HOST=",
		"trycloudflare\\.com",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("script missing %q:\n%s", want, s)
		}
	}

	fixed, err := RemoteEnsureScript(ArgoSpec{Mode: "fixed", Auth: "tok", Domain: "t.example.com"}, "f", "/usr/local/bin/cloudflared", "/etc/systemd/system", "/etc/cloudflared")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fixed, "ARGO_HOST=t.example.com") {
		t.Fatalf("fixed script should embed the domain host\n%s", fixed)
	}
}

func TestEnsureRemoteArgo(t *testing.T) {
	const tag = "remote-it"
	ClearArgoHost(tag)
	if got := ArgoHost(tag); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	// Fake SSH runner: echoes a CHANGED + HOST back, simulating a remote box.
	run := func(_ context.Context, cmd string) (string, error) {
		if !strings.Contains(cmd, "ARGO_HOST=") {
			t.Fatalf("runner got unexpected command")
		}
		return "ARGO_CHANGED=1\nARGO_HOST=node-x.trycloudflare.com\n", nil
	}
	spec := ArgoSpec{Mode: "temporary", TargetPort: 2053}
	host, changed, err := EnsureRemoteArgo(context.Background(), run, spec, tag, "/usr/local/bin/cloudflared", "/etc/systemd/system", "/etc/cloudflared")
	if err != nil {
		t.Fatal(err)
	}
	if host != "node-x.trycloudflare.com" || !changed {
		t.Fatalf("got host=%q changed=%v", host, changed)
	}
	if got := ArgoHost(tag); got != "node-x.trycloudflare.com" {
		t.Fatalf("host should be cached, got %q", got)
	}
	// Disabled clears the cache and skips the run.
	called := false
	run = func(context.Context, string) (string, error) { called = true; return "", nil }
	if _, _, err := EnsureRemoteArgo(context.Background(), run, ArgoSpec{Mode: ""}, tag, "/usr/local/bin/cloudflared", "/etc/systemd/system", "/etc/cloudflared"); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("disabled spec must not run anything")
	}
	if got := ArgoHost(tag); got != "" {
		t.Fatalf("disabled should clear cache, got %q", got)
	}
}