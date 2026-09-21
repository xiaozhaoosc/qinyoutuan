package sbproc

import (
	"strings"
	"testing"
)

func TestParseTemporaryHostname(t *testing.T) {
	cases := []struct {
		name string
		log  string
		want string
	}{
		{
			"plain quick tunnel",
			"INF 2025-09-01T10:00:00Z Your quick tunnel has been created! Visit it at https://shiny-frog-abc.trycloudflare.com",
			"shiny-frog-abc.trycloudflare.com",
		},
		{
			"host embedded among other words",
			"opening a connection to edge.region1.aws.r2.cloudflarestorage.com ... quick tunnel https://a-b-c123.trycloudflare.com?as_experimental=true",
			"a-b-c123.trycloudflare.com",
		},
		{
			"append-only log picks the LATEST tunnel",
			"https://old-one.trycloudflare.com\nhttps://new-two.trycloudflare.com",
			"new-two.trycloudflare.com",
		},
		{"no host yet", "starting quick tunnel...", ""},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ParseTemporaryHostname(c.log); got != c.want {
				t.Fatalf("ParseTemporaryHostname(%q) = %q; want %q", c.log, got, c.want)
			}
		})
	}
}

func TestArgoArgs(t *testing.T) {
	t.Run("disabled returns error", func(t *testing.T) {
		if _, err := ArgoArgs(ArgoSpec{Mode: ""}); err != ErrArgoDisabled {
			t.Fatalf("want ErrArgoDisabled, got %v", err)
		}
	})
	t.Run("temporary", func(t *testing.T) {
		got, err := ArgoArgs(ArgoSpec{Mode: "temporary", TargetPort: 2096})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"tunnel", "--no-autoupdate", "--url", "http://127.0.0.1:2096"}
		if len(got) != len(want) {
			t.Fatalf("got %v; want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("item %d = %q; want %q (full %v)", i, got[i], want[i], got)
			}
		}
	})
	t.Run("temporary needs port", func(t *testing.T) {
		if _, err := ArgoArgs(ArgoSpec{Mode: "temporary"}); err != ErrArgoBadPort {
			t.Fatalf("want ErrArgoBadPort, got %v", err)
		}
	})
	t.Run("fixed", func(t *testing.T) {
		got, err := ArgoArgs(ArgoSpec{Mode: "fixed", Auth: "tok-abc"})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"tunnel", "--no-autoupdate", "run", "--token", "tok-abc"}
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("got %v; want %v", got, want)
		}
	})
	t.Run("fixed needs token", func(t *testing.T) {
		if _, err := ArgoArgs(ArgoSpec{Mode: "fixed"}); err != ErrArgoBadAuth {
			t.Fatalf("want ErrArgoBadAuth, got %v", err)
		}
	})
	t.Run("unknown mode is disabled", func(t *testing.T) {
		if _, err := ArgoArgs(ArgoSpec{Mode: "BOGUS"}); err != ErrArgoDisabled {
			t.Fatalf("want ErrArgoDisabled, got %v", err)
		}
	})
}

func TestArgoSpecEnabled(t *testing.T) {
	if !(ArgoSpec{Mode: "temporary"}).Enabled() {
		t.Fatal("temporary should be enabled")
	}
	if !(ArgoSpec{Mode: "fixed"}).Enabled() {
		t.Fatal("fixed should be enabled")
	}
	if (ArgoSpec{Mode: ""}).Enabled() {
		t.Fatal("empty mode should be disabled")
	}
	if (ArgoSpec{Mode: "BOGUS"}).Enabled() {
		t.Fatal("unknown mode should be disabled")
	}
}

func TestCloudflaredServiceUnit(t *testing.T) {
	u := CloudflaredServiceUnit("/usr/local/bin/cloudflared", "/etc/cloudflared/tunnel.log",
		ArgoSpec{Mode: "temporary", TargetPort: 2053})
	for _, want := range []string{
		"[Service]",
		"ExecStart=/usr/local/bin/cloudflared tunnel --no-autoupdate --url http://127.0.0.1:2053 --logfile /etc/cloudflared/tunnel.log",
		"Restart=always",
		"NoNewPrivileges=true",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(u, want) {
			t.Fatalf("unit missing %q:\n%s", want, u)
		}
	}
}

func TestQuoteWorld(t *testing.T) {
	got := quoteWorld([]string{"run", "--token", "has space", "plain"})
	if !strings.Contains(got, `--token "has space"`) {
		t.Fatalf("expected token value quoted, got: %q", got)
	}
	if !strings.Contains(got, " plain") {
		t.Fatalf("expected plain arg unquoted, got: %q", got)
	}
}