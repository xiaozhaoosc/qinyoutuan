package singbox

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildShareLinkVmessReality verifies that a vmess+Reality inbound emits its
// reality client params (security/pbk/sid) into the vmess:// JSON payload —
// without them a client has no public_key/short_id and the Reality handshake
// always fails, which is exactly how a vmess+Reality direct node breaks.
func TestBuildShareLinkVmessReality(t *testing.T) {
	p := LinkParams{Type: "vmess", Tag: "n", Host: "1.2.3.4", Port: 20086,
		UUID: "8988083f-1cae-4285-8008-f99eeda971fa", TLS: true, SNI: "www.microsoft.com",
		Network: "ws", Path: "/ws", PublicKey: "PBK123", ShortID: "SID123"}

	link := BuildShareLink(p)
	if link == "" || !strings.HasPrefix(link, "vmess://") {
		t.Fatalf("bad link: %q", link)
	}
	dec, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(link, "vmess://"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(dec, &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	if m["security"] != "reality" {
		t.Fatalf("security should be reality, got %v", m["security"])
	}
	if m["pbk"] != "PBK123" || m["sid"] != "SID123" {
		t.Fatalf("pbk/sid wrong: %+v", m)
	}

	// 无 Reality 时保持普通 tls，不泄漏 security/pbk/sid
	p.PublicKey = ""
	p.ShortID = ""
	link2 := BuildShareLink(p)
	dec2, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(link2, "vmess://"))
	if err != nil {
		t.Fatalf("decode2: %v", err)
	}
	var m2 map[string]interface{}
	if err := json.Unmarshal(dec2, &m2); err != nil {
		t.Fatalf("json2: %v", err)
	}
	if m2["security"] == "reality" || m2["pbk"] != nil || m2["sid"] != nil {
		t.Fatalf("plain-tls link leaked reality params: %+v", m2)
	}
}
