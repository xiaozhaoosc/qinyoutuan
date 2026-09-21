package api

// Remote Argo tunnels (stage E): a landing machine whose sing-box is managed by
// the panel over SSH can front one of its vmess-ws inbounds with cloudflared too.
// The reconcile loop below mirrors main.startLocalArgoSync, but for servers in the
// `servers` table: every enabled argo inbound on a reachable, credentialed server
// gets its cloudflared systemd unit ensured via an SSH runner, and the resulting
// tunnel hostname is cached under the inbound tag so the subscription layer emits
// the 13-port CDN links exactly as it does for the panel-host case.

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"qingzhou/internal/sbctl"
	"qingzhou/internal/sbproc"
	"qingzhou/internal/store"
)

// StartArgoSync runs the remote-argo reconcile on a timer. wg (optional) lets the
// caller keep shutdown from closing the DB under an in-flight pass.
func (a *API) StartArgoSync(ctx context.Context, wg *sync.WaitGroup) {
	if wg != nil {
		wg.Add(1)
	}
	unitDir := envOr("QZ_ARGO_UNIT_DIR", "/etc/systemd/system")
	logDir := envOr("QZ_ARGO_LOGFILE_DIR", "/etc/cloudflared")
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		a.syncRemoteArgo(ctx, unitDir, logDir)
		t := time.NewTicker(2 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.syncRemoteArgo(ctx, unitDir, logDir)
			}
		}
	}()
}

// syncRemoteArgo ensures cloudflared on every credentialed remote server that has
// at least one enabled argo inbound. Errors are logged, never fatal: a dead node
// must not take the whole sync down, and next tick retries.
func (a *API) syncRemoteArgo(ctx context.Context, unitDir, logDir string) {
	servers, err := a.st.ListServers()
	if err != nil {
		log.Printf("argo sync: ListServers: %v", err)
		return
	}
	for _, sv := range servers {
		if !serverHasCredentials(sv) {
			continue // no SSH creds → can't touch this node yet
		}
		inbounds, err := a.st.ListSbInboundsByServer(sv.ID)
		if err != nil {
			log.Printf("argo sync: server %d inbounds: %v", sv.ID, err)
			continue
		}
		var targets []*store.SbInbound
		for _, ib := range inbounds {
			if ib.Enabled && ib.ArgoEnabled && ib.ArgoMode != "" {
				targets = append(targets, ib)
			}
		}
		if len(targets) == 0 {
			continue
		}
		rm := a.newRemoteManager(40 * time.Second)
		cfg := sbctl.SSHConfigFor(sv)
		// Run the ensure script as root on the node via `sudo -n bash`. The remote
		// cloudflared binary lives at the standard --with-argo install path.
		run := func(rctx context.Context, cmd string) (string, error) {
			return rm.RunCommand(rctx, cfg, "sudo -n bash <<'QZEOF'\n"+cmd+"\nQZEOF\n")
		}
		for _, ib := range targets {
			spec := sbproc.ArgoSpec{Mode: ib.ArgoMode, Auth: ib.ArgoAuth, Domain: ib.ArgoDomain, TargetPort: ib.ListenPort + sbproc.ArgoOriginPortDelta}
			if _, changed, err := sbproc.EnsureRemoteArgo(ctx, run, spec, ib.Tag, sbproc.CloudflaredBin, unitDir, logDir); err != nil {
				log.Printf("argo sync: server %d inbound %s: %v", sv.ID, ib.Tag, err)
			} else if changed {
				log.Printf("argo sync: updated remote cloudflared unit for inbound %s (server %d)", ib.Tag, sv.ID)
			}
		}
	}
}

// envOr returns the env var value, or def when empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}