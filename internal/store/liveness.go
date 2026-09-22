package store

import (
	"net"
	"strconv"
	"sync"
	"time"
)

// 多 IP 存活兜底：对候选 host 做轻量 TCP 可达性判定，带 TTL 缓存避免每次订阅
// 实时探测（控制频率），并返回「可达在前」的排序供主节点选择。

type livenessEntry struct {
	alive bool
	ts    int64 // 探测完成时间（纳秒）
}

var (
	livenessMu   sync.Mutex
	livenessCache = map[string]livenessEntry{}
)

const (
	// livenessTTL 探测结果缓存时长；期间不重复探测。
	livenessTTL = 30 * time.Second
	// livenessTimeout 单次拨测超时，超过视为不可达。
	livenessTimeout = 3 * time.Second
)

// probeHost 探测 host:port 是否可达（带 livenessTTL 缓存）。失败也短缓存，
// 避免连不上时每秒重试；缓存过期后再探。
func probeHost(host string, port int) bool {
	key := host + ":" + strconv.Itoa(port)
	now := time.Now().UnixNano()

	livenessMu.Lock()
	if e, ok := livenessCache[key]; ok && now-e.ts < int64(livenessTTL) {
		alive := e.alive
		livenessMu.Unlock()
		return alive
	}
	livenessMu.Unlock()

	c := net.Dialer{Timeout: livenessTimeout, KeepAlive: 0}
	conn, err := c.Dial("tcp", key)
	alive := err == nil
	if conn != nil {
		conn.Close()
	}

	livenessMu.Lock()
	livenessCache[key] = livenessEntry{alive: alive, ts: now}
	livenessMu.Unlock()
	return alive
}

// pickAliveHosts 并发探测每个候选 host，返回「可达在前、不可达在后」的排序副本。
// 全部不可达时按原序返回，避免误判导致主节点被全部跳过。
func pickAliveHosts(hosts []string, port int) []string {
	type hp struct {
		h    string
		alive bool
	}
	res := make([]hp, len(hosts))
	var wg sync.WaitGroup
	for i, h := range hosts {
		wg.Add(1)
		go func(i int, h string) {
			defer wg.Done()
			res[i] = hp{h: h, alive: probeHost(h, port)}
		}(i, h)
	}
	wg.Wait()

	alive, dead := make([]string, 0, len(hosts)), make([]string, 0, len(hosts))
	for _, x := range res {
		if x.alive {
			alive = append(alive, x.h)
		} else {
			dead = append(dead, x.h)
		}
	}
	if len(alive) == 0 {
		// 全失败：回退原序，宁可让客户端自己试也不要漏掉可用主节点
		return hosts
	}
	merged := append(alive, dead...)
	if len(merged) != len(hosts) {
		// 防御：任何异常都不改变 host 集合，只调整顺序
		return hosts
	}
	return merged
}