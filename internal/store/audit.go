package store

import (
	"context"
	"time"
)

// AuditEntry records one change made through rabmin.
type AuditEntry struct {
	ID     int64     `json:"id"`
	Time   time.Time `json:"time"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Target string    `json:"target"`
	Detail string    `json:"detail,omitempty"`
	OK     bool      `json:"ok"`
	Error  string    `json:"error,omitempty"`
	Remote string    `json:"remote,omitempty"`
}

// AddAudit appends an audit entry.
func (s *Store) AddAudit(ctx context.Context, e AuditEntry) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit (ts, actor, action, target, detail, ok, error, remote) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Time.UnixMilli(), e.Actor, e.Action, e.Target, e.Detail, e.OK, e.Error, e.Remote)
	return err
}

// AuditQuery filters the audit log.
type AuditQuery struct {
	Actor  string
	Action string
	Target string
	Before int64
	Limit  int
}

// ListAudit returns entries newest first.
func (s *Store) ListAudit(ctx context.Context, q AuditQuery) ([]AuditEntry, error) {
	where, args := `WHERE 1=1`, []any{}
	if q.Actor != "" {
		where += ` AND actor = ?`
		args = append(args, q.Actor)
	}
	if q.Action != "" {
		where += ` AND action LIKE ?`
		args = append(args, q.Action+"%")
	}
	if q.Target != "" {
		where += ` AND target LIKE ?`
		args = append(args, "%"+q.Target+"%")
	}
	if q.Before > 0 {
		where += ` AND id < ?`
		args = append(args, q.Before)
	}
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	args = append(args, q.Limit)
	rows, err := s.db.QueryContext(ctx, `SELECT id, ts, actor, action, target, detail, ok, error, remote FROM audit `+where+` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.Actor, &e.Action, &e.Target, &e.Detail, &e.OK, &e.Error, &e.Remote); err != nil {
			return nil, err
		}
		e.Time = time.UnixMilli(ts).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}
