package subconv

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// TestSingboxVmessReality verifies a vmess+Reality link keeps its reality block
// when rendered to sing-box format: the vmess JSON payload carries
// security=reality + pbk + sid (see singbox.BuildShareLink), and the renderer
// must pass security through and read pbk/sid from the JSON map (tlsParam).
func TestSingboxVmessReality(t *testing.T) {
	vm := map[string]any{
		"v": "2", "ps": "n", "add": "1.2.3.4", "port": "20086",
		"id": "8988083f-1cae-4285-8008-f99eeda971fa", "aid": "0", "scy": "auto",
		"net": "ws", "path": "/ws", "host": "www.microsoft.com",
		"tls": "tls", "sni": "www.microsoft.com", "fp": "chrome",
		"security": "reality", "pbk": "PBK123", "sid": "SID123",
	}
	payload, _ := json.Marshal(vm)
	link := "vmess://" + base64.StdEncoding.EncodeToString(payload)

	proxies := ParseLinks([]string{link})
	if len(proxies) != 1 {
		t.Fatalf("parse: %d proxies", len(proxies))
	}
	sb, err := Singbox(proxies, "")
	if err != nil {
		t.Fatalf("singbox: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(sb), &cfg); err != nil {
		t.Fatalf("json: %v", err)
	}
	var vmessOut map[string]any
	for _, ib := range cfg["outbounds"].([]any) {
		m, _ := ib.(map[string]any)
		if str(m["type"]) == "vmess" {
			vmessOut = m
			break
		}
	}
	if vmessOut == nil {
		t.Fatalf("no vmess outbound: %s", sb)
	}
	tls, _ := vmessOut["tls"].(map[string]any)
	if tls == nil || tls["enabled"] != true {
		t.Fatalf("tls block missing: %+v", vmessOut["tls"])
	}
	re, _ := tls["reality"].(map[string]any)
	if re == nil || re["enabled"] != true {
		t.Fatalf("reality block missing: %+v", tls)
	}
	if re["public_key"] != "PBK123" || re["short_id"] != "SID123" {
		t.Fatalf("reality params wrong: %+v", re)
	}
	if tls["server_name"] != "www.microsoft.com" {
		t.Fatalf("server_name wrong: %+v", tls)
	}
}

// TestClashVmessReality verifies the Clash (mihomo) renderer also keeps the
// reality block for a vmess+Reality link, reading security/pbk/sid from the
// vmess JSON map (mihomo's reality-opts).
func TestClashVmessReality(t *testing.T) {
	vm := map[string]any{
		"v": "2", "ps": "n", "add": "1.2.3.4", "port": "20086",
		"id": "8988083f-1cae-4285-8008-f99eeda971fa", "aid": "0", "scy": "auto",
		"net": "ws", "path": "/ws", "host": "www.microsoft.com",
		"tls": "tls", "sni": "www.microsoft.com", "fp": "chrome",
		"security": "reality", "pbk": "PBK123", "sid": "SID123",
	}
	payload, _ := json.Marshal(vm)
	link := "vmess://" + base64.StdEncoding.EncodeToString(payload)

	proxies := ParseLinks([]string{link})
	cl, err := Clash(proxies, "")
	if err != nil {
		t.Fatalf("clash: %v", err)
	}
	for _, want := range []string{"reality-opts", "public-key: PBK123", "short-id: SID123", "tls: true", "servername: www.microsoft.com"} {
		if !strings.Contains(cl, want) {
			t.Fatalf("clash output missing %q:\n%s", want, cl)
		}
	}
}
