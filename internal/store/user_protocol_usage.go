package store

import (
	"time"
)

// ProtocolUsage is a user's cumulative traffic through a single protocol
// (e.g. "vless"/"hysteria2"). Backed by user_protocol_usage: a best-effort,
// additive table separate from billing, fed from sing-box's per-inbound stats.
type ProtocolUsage struct {
	UserID    int64  `json:"user_id"`
	Protocol  string `json:"protocol"`
	ServerID  int64  `json:"server_id"`
	Up        int64  `json:"up"`
	Down      int64  `json:"down"`
	UpdatedAt int64  `json:"updated_at"`
}

// AddProtocolUsage accumulates per-protocol deltas. byProto maps protocol →
// (stats identity → delta). Identification reuses the billing resolver so bytes
// land on the right user_id, but this is deliberately standalone: failures here
// are best-effort and never affect the primary meter (AddUsageBatchesByServer).
// server_id is stored as 0 (panel machine) since local sing-box stats are the
// collected source.
func (s *Store) AddProtocolUsage(byProto map[string]map[string]UsageDelta) error {
	if len(byProto) == 0 {
		return nil
	}
	// Canonicalize every identity and pre-resolve account-level targets in one
	// pass (mirrors addUsageBatches), so the per-identity loop only pays for rows.
	all := map[string]UsageDelta{}
	for _, deltas := range byProto {
		for name, d := range deltas {
			n, _ := canonicalStatsIdentity(name)
			cur := all[n]
			cur.Up += d.Up
			cur.Down += d.Down
			all[n] = cur
		}
	}
	if len(all) == 0 {
		return nil
	}
	acctTargets, _ := s.accountTargets(all)
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for proto, deltas := range byProto {
		for rawName, d := range deltas {
			name, _ := canonicalStatsIdentity(rawName)
			if name == "" || (d.Up == 0 && d.Down == 0) {
				continue
			}
			_, userID, _, _, rerr := s.resolveStatsIdentity(tx, name, acctTargets)
			if rerr != nil || userID <= 0 {
				continue // unknown / just-removed identity — skip, best-effort
			}
			_, werr := tx.Exec(`INSERT INTO user_protocol_usage (user_id, protocol, server_id, up, down, updated_at)
				VALUES (?,?,0,?,?,?)
				ON CONFLICT(user_id, protocol, server_id) DO UPDATE SET
					up=user_protocol_usage.up+excluded.up,
					down=user_protocol_usage.down+excluded.down,
					updated_at=excluded.updated_at`, userID, proto, d.Up, d.Down, now)
			if werr != nil {
				return werr
			}
		}
	}
	return tx.Commit()
}

// ListProtocolUsage returns cumulative protocol usage, optionally filtered to one
// user (userID<=0 = all users).
func (s *Store) ListProtocolUsage(userID int64) ([]ProtocolUsage, error) {
	q := `SELECT user_id, protocol, server_id, up, down, updated_at FROM user_protocol_usage`
	var args []any
	if userID > 0 {
		q += ` WHERE user_id=?`
		args = append(args, userID)
	}
	q += ` ORDER BY user_id, protocol`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProtocolUsage
	for rows.Next() {
		var p ProtocolUsage
		if err := rows.Scan(&p.UserID, &p.Protocol, &p.ServerID, &p.Up, &p.Down, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}