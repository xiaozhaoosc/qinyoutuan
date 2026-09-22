package store

import (
	"net"
	"reflect"
	"testing"
	"time"
)

// TestPickAliveHosts：可达 host 排前；全部不可达回退原序。
func TestPickAliveHosts(t *testing.T) {
	// 起一个本地 TCP listener 模拟"可达"
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	aliveIP := "127.0.0.1"
	deadIP := "127.0.0.2" // 本机无服务，探测失败

	got := pickAliveHosts([]string{deadIP, aliveIP}, port)
	if len(got) != 2 {
		t.Fatalf("want 2 hosts, got %d: %v", len(got), got)
	}
	// 可达者应排前（无论输入顺序）
	if got[0] != aliveIP {
		t.Fatalf("alive host should be first, got %v", got)
	}

	// 全部不可达 -> 原序
	all := pickAliveHosts([]string{"127.0.0.250", "127.0.0.251"}, 1)
	if !reflect.DeepEqual(all, []string{"127.0.0.250", "127.0.0.251"}) {
		t.Fatalf("all-dead should fall back to original order, got %v", all)
	}
}

// TestProbeHostCache 覆盖缓存命中路径（不能实际等 TTL，仅验证调用不 panic、结果稳定）。
func TestProbeHostCache(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() { for { c, e := ln.Accept(); if e != nil { return }; c.Close() } }()
	port := ln.Addr().(*net.TCPAddr).Port
	if !probeHost("127.0.0.1", port) {
		t.Fatal("listener host should be alive")
	}
	// 连续两次应在缓存命中，结果一致
	if !probeHost("127.0.0.1", port) {
		t.Fatal("cached alive should stay alive")
	}
	_ = time.Now // quiet unused import path
}

// TestMarshalParseHosts：JSON 往返。
func TestMarshalParseHosts(t *testing.T) {
	if s := marshalHosts(nil); s != "" {
		t.Fatalf("empty->empty, got %q", s)
	}
	if hs := parseHosts(""); hs != nil {
		t.Fatalf("empty string->nil, got %v", hs)
	}
	hs := []string{"1.2.3.4", "5.6.7.8"}
	back := parseHosts(marshalHosts(hs))
	if !reflect.DeepEqual(back, hs) {
		t.Fatalf("roundtrip mismatch: %v vs %v", back, hs)
	}
	if got := parseHosts("not-json"); got != nil {
		t.Fatalf("garbage->nil, got %v", got)
	}
}