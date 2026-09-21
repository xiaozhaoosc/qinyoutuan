package store

import (
	"strings"
	"testing"

	"qingzhou/internal/singbox"
)

func TestArgoVariantLinks(t *testing.T) {
	base := singbox.LinkParams{Type: "vmess", Tag: "node-A", Host: "1.2.3.4", Port: 12345,
		Network: "ws", Path: "/uuid-vm", WSHost: "x.serv00.net", TLS: true, SNI: "x"}
	const argoHost = "my.trycloudflare.com"

	vs := argoVariantLinks(base, argoHost)
	if len(vs) != 13 {
		t.Fatalf("want 13 variants, got %d", len(vs))
	}
	seen := map[int]bool{}
	for _, v := range vs {
		if v.Host != argoHost {
			t.Fatalf("dial host not overridden: %q", v.Host)
		}
		if v.WSHost != argoHost {
			t.Fatalf("ws host not overridden: %q", v.WSHost)
		}
		if v.Network != "ws" {
			t.Fatalf("network not forced to ws: %q", v.Network)
		}
		if v.Port < 1 {
			t.Fatalf("bad port %d", v.Port)
		}
		if v.TLS && v.SNI != argoHost {
			t.Fatalf("tls variant sni should be the tunnel host, got %q", v.SNI)
		}
		if !v.TLS && v.SNI != "" {
			t.Fatalf("plain variant should have empty sni, got %q", v.SNI)
		}
		if !strings.HasPrefix(v.Tag, "node-A-argo-") {
			t.Fatalf("tag should carry argo suffix: %q", v.Tag)
		}
		if seen[v.Port] {
			t.Fatalf("duplicate port %d", v.Port)
		}
		seen[v.Port] = true
	}
	// No known host → no variants (temporary tunnel not yet up).
	if got := argoVariantLinks(base, ""); got != nil {
		t.Fatalf("empty host must yield nil, got %d links", len(got))
	}
}

func TestArgoSpecMapping(t *testing.T) {
	ib := &SbInbound{Type: "vmess", ListenPort: 2053, ArgoMode: "temporary", ArgoAuth: "", ArgoDomain: ""}
	spec := ib.argoSpec()
	if spec.Mode != "temporary" || spec.TargetPort != 2053 {
		t.Fatalf("argoSpec mapping wrong: %+v", spec)
	}

	fixed := &SbInbound{ListenPort: 8443, ArgoMode: "fixed", ArgoAuth: "tok", ArgoDomain: "t.example.com"}
	f := fixed.argoSpec()
	if f.Mode != "fixed" || f.Auth != "tok" || f.Domain != "t.example.com" || f.TargetPort != 8443 {
		t.Fatalf("fixed argoSpec mapping wrong: %+v", f)
	}
}