package store

import (
	"strings"
	"testing"

	"qingzhou/internal/sbproc"
	"qingzhou/internal/singbox"
)

func TestArgoVariantLinks(t *testing.T) {
	base := singbox.LinkParams{Type: "vmess", Tag: "node-A", Host: "1.2.3.4", Port: 12345,
		Network: "ws", Path: "/uuid-vm", WSHost: "x.serv00.net", TLS: true, SNI: "x"}
	const argoHost = "my.trycloudflare.com"

	// fixed 隧道：13 个 CDN 端口全下发
	vs := argoVariantLinks(base, argoHost, "fixed")
	if len(vs) != 13 {
		t.Fatalf("fixed: want 13 variants, got %d", len(vs))
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

	// temporary 隧道：quick tunnel 只服务 443，只下发 443 一个节点
	tmp := argoVariantLinks(base, argoHost, "temporary")
	if len(tmp) != 1 {
		t.Fatalf("temporary: want 1 variant, got %d", len(tmp))
	}
	if tmp[0].Port != 443 || !tmp[0].TLS {
		t.Fatalf("temporary variant should be the single 443 tls node, got %+v", tmp[0])
	}

	// No known host → no variants (temporary tunnel not yet up).
	if got := argoVariantLinks(base, "", "temporary"); got != nil {
		t.Fatalf("empty host must yield nil, got %d links", len(got))
	}
}

func TestArgoSpecMapping(t *testing.T) {
	ib := &SbInbound{Type: "vmess", ListenPort: 2053, ArgoMode: "temporary", ArgoAuth: "", ArgoDomain: ""}
	spec := ib.argoSpec()
	if spec.Mode != "temporary" || spec.TargetPort != 2053+sbproc.ArgoOriginPortDelta {
		t.Fatalf("argoSpec mapping wrong: %+v", spec)
	}

	fixed := &SbInbound{ListenPort: 8443, ArgoMode: "fixed", ArgoAuth: "tok", ArgoDomain: "t.example.com"}
	f := fixed.argoSpec()
	if f.Mode != "fixed" || f.Auth != "tok" || f.Domain != "t.example.com" || f.TargetPort != 8443+sbproc.ArgoOriginPortDelta {
		t.Fatalf("fixed argoSpec mapping wrong: %+v", f)
	}
}

func TestArgoOriginInbound(t *testing.T) {
	users := []singbox.User{{Name: "u1", UUID: "abc"}}

	// 本机 argo vmess 入站 → 派生无 TLS 明文 ws 回源入站
	ib := &SbInbound{ServerID: 0, Type: "vmess", Tag: "argo-test", ListenPort: 20086,
		ArgoEnabled: true, ArgoMode: "temporary",
		Options: `{"transport":{"type":"ws","path":"/ws"}}`}
	origin := argoOriginInbound(ib, users, 0)
	if origin == nil {
		t.Fatal("argo inbound should derive an origin inbound")
	}
	if origin.Type != "vmess" || origin.Base["listen_port"] != 20086+sbproc.ArgoOriginPortDelta {
		t.Fatalf("origin port wrong: %+v", origin.Base)
	}
	if origin.Base["listen"] != "127.0.0.1" {
		t.Fatalf("origin must listen on loopback only, got %q", origin.Base["listen"])
	}
	if _, hasTLS := origin.Base["tls"]; hasTLS {
		t.Fatal("origin inbound must be plaintext (no tls block)")
	}
	tr, ok := origin.Base["transport"].(map[string]interface{})
	if !ok || tr["type"] != "ws" || tr["path"] != "/ws" {
		t.Fatalf("origin transport wrong: %+v", origin.Base["transport"])
	}

	// 其它 server 的入站、非 vmess、未启用 argo → 不派生
	if got := argoOriginInbound(&SbInbound{ServerID: 1, Type: "vmess", Tag: "x", ListenPort: 20086, ArgoEnabled: true, ArgoMode: "temporary"}, users, 0); got != nil {
		t.Fatal("remote-server inbound must not derive an origin here")
	}
	if got := argoOriginInbound(&SbInbound{ServerID: 0, Type: "vless", Tag: "x", ListenPort: 20086, ArgoEnabled: true, ArgoMode: "temporary"}, users, 0); got != nil {
		t.Fatal("non-vmess inbound must not derive an origin")
	}
	if got := argoOriginInbound(&SbInbound{ServerID: 0, Type: "vmess", Tag: "x", ListenPort: 20086}, users, 0); got != nil {
		t.Fatal("non-argo inbound must not derive an origin")
	}
}
