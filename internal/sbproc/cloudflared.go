package sbproc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// This file implements the Cloudflare Argo / Quick Tunnel exposure driver. It
// translates an argo-enabled inbound (see store.SbInbound.Argo*) into the exact
// cloudflared invocation and systemd unit the node should run, and knows how to
// read the ephemeral trycloudflare.com hostname back out of cloudflared's logs.
//
// The panel follows the same pattern it uses for sing-box: it does NOT keep a
// hand-spawned child in its own process, it writes a systemd unit and lets
// systemd keep cloudflared alive (Restart=always). This is strictly stronger
// than sing-box-yg's "pgrep then relaunch" loop — a crash is restarted by PID1,
// not by the next panel tick — and it means "is it up" is one
// `systemctl is-active cloudflared` call away on local and remote nodes alike.
//
// Two tunnel flavors, mirroring cloudflared's own CLI:
//   - temporary:  `cloudflared tunnel --url http://127.0.0.1:<port> --no-autoupdate`
//     asks Cloudflare for a fresh quick-tunnel hostname (`*.trycloudflare.com`).
//     No credentials, but the hostname changes every restart.
//   - fixed:      `cloudflared tunnel --no-autoupdate run --token <auth>`
//     connects to a tunnel the operator already created; its public hostname is
//     fixed ahead of time (stored in ArgoSpec.Domain), so no log parsing needed.

// ArgoSpec describes how one argo-exposed inbound should be fronted by
// cloudflared. It is derived from the inbound's Argo* fields plus the local
// loopback port of the vmess WS inbound it wraps.
type ArgoSpec struct {
	// Mode is "" (disabled/not argo), "temporary", or "fixed".
	Mode string
	// Auth is the fixed-tunnel token (only used when Mode == "fixed").
	Auth string
	// Domain is the fixed-tunnel public hostname (only used when Mode == "fixed").
	Domain string
	// TargetPort is the loopback port of the vmess WS inbound to front. Used by
	// the quick tunnel (`--url http://127.0.0.1:<TargetPort>`).
	TargetPort int
}

// Enabled reports whether the spec wants a tunnel at all.
func (s ArgoSpec) Enabled() bool { return s.Mode == "temporary" || s.Mode == "fixed" }

// Arg error values returned by ArgoArgs when the spec is unusable.
var (
	ErrArgoDisabled  = fmt.Errorf("argo not enabled")
	ErrArgoBadPort   = fmt.Errorf("argo quick tunnel needs a TargetPort>0")
	ErrArgoBadAuth   = fmt.Errorf("argo fixed tunnel needs a token")
	ErrArgoBadDomain = fmt.Errorf("argo fixed tunnel needs a domain")
)

// ArgoArgs returns the cloudflared CLI argument list (excluding the binary
// path) for the spec, or an error when the spec cannot produce a working
// tunnel. Deliberately the single place the CLI is decided, so every caller —
// instance provisioning, systemd unit render, remote push — gets the same args.
func ArgoArgs(s ArgoSpec) ([]string, error) {
	switch s.Mode {
	case "temporary":
		if s.TargetPort <= 0 {
			return nil, ErrArgoBadPort
		}
		// Quick tunnel = the TOP-LEVEL `tunnel --url` form. `tunnel run` requires
		// a named tunnel/token and rejects --url (verified against cloudflared
		// 2026.9 on a live box — a unit test only checks our own expectation).
		return []string{
			"tunnel", "--no-autoupdate",
			"--url", fmt.Sprintf("http://127.0.0.1:%d", s.TargetPort),
		}, nil
	case "fixed":
		if s.Auth == "" {
			return nil, ErrArgoBadAuth
		}
		return []string{"tunnel", "--no-autoupdate", "run", "--token", s.Auth}, nil
	default:
		return nil, ErrArgoDisabled
	}
}

var tempHostRE = regexp.MustCompile(`[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.trycloudflare\.com`)

// ParseTemporaryHostname extracts the quick-tunnel hostname
// (`something.trycloudflare.com`) from cloudflared's log. Quick tunnels print
// their URL once on startup; the logfile is append-only and grows across
// restarts (each restart creates a NEW quick tunnel), so the LATEST line is the
// live one — take the last match, not the first, or a stale hostname from a
// previous tunnel keeps winning. Returns "" when the log has no such host yet
// (still connecting), so callers can retry.
func ParseTemporaryHostname(log string) string {
	all := tempHostRE.FindAllString(log, -1)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1]
}

// CloudflaredBin is the default install location used by install-singbox.sh's
// --with-argo path (stage B). Listed first so FindCloudflaredBin prefers it.
var CloudflaredBin = "/usr/local/bin/cloudflared"

// FindCloudflaredBin locates the cloudflared binary: the ship-with script path,
// then common places, then PATH. Returns "" if not found (the Argo feature is
// off / not installed).
func FindCloudflaredBin() string {
	for _, p := range []string{
		CloudflaredBin,
		"/usr/bin/cloudflared",
		"/usr/local/sbin/cloudflared",
		"/opt/qinyoutuan/cloudflared",
	} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p
	}
	return ""
}

// quoteWorld joined args as a single systemd ExecStart line. Only values that
// could trip command tokenization are quoted; a bare flag is left plain. Both
// a tunnel token and a quick-tunnel URL are quoted for safety.
func quoteWorld(args []string) string {
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"'\\") {
			args[i] = `"` + strings.ReplaceAll(strings.ReplaceAll(a, `\`, `\\`), `"`, `\"`) + `"`
		}
	}
	return strings.Join(args, " ")
}

// CloudflaredServiceUnit renders the systemd unit that keeps cloudflared alive.
// bin is the cloudflared binary path; logfile, when non-empty, is written to a
// --logfile so the panel (or ops) can read the trycloudflare.com hostname out of
// a stable place rather than the journal. Mirrors the panel's sing-box unit: a
// modest service with Restart=always and no removed sandbox capabilities — a
// tunnel needs nothing privileged.
func CloudflaredServiceUnit(bin, logfile string, s ArgoSpec) string {
	args, _ := ArgoArgs(s) // unit is only rendered for an enabled spec
	if logfile != "" {
		args = append(args, "--logfile", logfile)
	}
	var desc string
	if s.Mode == "temporary" {
		desc = "cloudflared quick tunnel"
	} else {
		desc = "cloudflared tunnel " + s.Domain
	}
	return fmt.Sprintf(`[Unit]
Description=%s
After=network.target nss-lookup.target

[Service]
ExecStart=%s %s
Restart=always
RestartSec=5s
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
`, desc, bin, quoteWorld(args))
}

// ---- local tunnel reconciliation ----

// argoHosts caches the last-known public hostname of each argo tunnel, keyed by
// inbound tag. Fixed tunnels are deterministic (the stored Domain); temporary
// tunnels are re-parsed from the running cloudflared's logfile on each ensure.
// store.BuildSelfBuiltLinks reads it when emitting the 13-port CDN links, so a
// freshly started quick tunnel's hostname flows into the next subscription.
var argoHosts sync.Map // tag -> string

// SetArgoHost records an argo tunnel's public hostname for an inbound tag.
func SetArgoHost(tag, host string) {
	if host == "" {
		ClearArgoHost(tag)
		return
	}
	argoHosts.Store(tag, host)
}

// ClearArgoHost forgets a tunnel's hostname (tunnel disabled/removed).
func ClearArgoHost(tag string) { argoHosts.Delete(tag) }

// ArgoHost returns the last known public hostname for the inbound tag, "" when
// none is cached yet (tunnel not up, or disabled).
func ArgoHost(tag string) string {
	if v, ok := argoHosts.Load(tag); ok {
		return v.(string)
	}
	return ""
}

// argoSpecHost returns the public hostname the spec currently serves. Fixed
// tunnels always serve their stored Domain; temporary tunnels are parsed from
// cloudflared's logfile ("" until the quick tunnel prints its URL).
func argoSpecHost(s ArgoSpec, logfile string) string {
	switch s.Mode {
	case "fixed":
		return s.Domain
	case "temporary":
		if logfile != "" {
			if b, err := os.ReadFile(logfile); err == nil {
				return ParseTemporaryHostname(string(b))
			}
		}
	}
	return ""
}

// runSystemctl runs a systemctl subcommand with the given args.
func runSystemctl(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

// EnsureLocalArgo reconciles the local cloudflared systemd unit so it serves
// the desired tunnel for an inbound, and returns the hostname that tunnel is
// reachable at (fixed → spec.Domain; temporary → parsed from logfile, "" until
// the tunnel is up). changed reports whether the unit was rewritten — and so
// whether the tunnel was cycled. A no-op when nothing about the unit changed, so
// the reconcile loop can call this on a timer without restarting the tunnel on
// every tick.
func EnsureLocalArgo(ctx context.Context, spec ArgoSpec, tag, bin, unitPath, logfile string) (string, bool, error) {
	if !spec.Enabled() {
		ClearArgoHost(tag)
		return "", false, nil
	}
	unit := CloudflaredServiceUnit(bin, logfile, spec)
	changed := false
	if cur, err := os.ReadFile(unitPath); err != nil || string(cur) != unit {
		if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
			return "", false, err
		}
		if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
			return "", false, err
		}
		name := filepath.Base(unitPath)
		for _, c := range [][]string{{"daemon-reload"}, {"enable", "--now", name}, {"restart", name}} {
			if _, err := runSystemctl(ctx, c...); err != nil {
				return "", true, err
			}
		}
		changed = true
	}
	host := argoSpecHost(spec, logfile)
	if host != "" {
		SetArgoHost(tag, host)
	}
	return host, changed, nil
}