package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Role controls what a user or API token may do.
type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	return r == RoleViewer || r == RoleOperator || r == RoleAdmin
}

// Allows reports whether r grants at least the permissions of need.
func (r Role) Allows(need Role) bool {
	return r.rank() >= need.rank()
}

func (r Role) rank() int {
	switch r {
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	}
	return 0
}

// Min returns the less privileged of two roles.
func Min(a, b Role) Role {
	if a.rank() < b.rank() {
		return a
	}
	return b
}

// User is a rabmin login.
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Role         Role      `json:"role"`
	Disabled     bool      `json:"disabled"`
	CreatedAt    time.Time `json:"createdAt"`
	PasswordHash string    `json:"-"`
}

const userCols = `id, username, password_hash, role, disabled, created_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	var created int64
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Disabled, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.CreatedAt = time.Unix(created, 0).UTC()
	return &u, nil
}

// CountUsers returns the number of users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CountAdmins returns the number of enabled admins.
func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled = 0`).Scan(&n)
	return n, err
}

// CreateUser inserts a user.
func (s *Store) CreateUser(ctx context.Context, username, passwordHash string, role Role) (*User, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)`,
		username, passwordHash, role, time.Now().Unix())
	if isUnique(err) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetUser(ctx, id)
}

// CreateFirstUser inserts a user only when no users exist, atomically, and
// returns ErrConflict otherwise.
func (s *Store) CreateFirstUser(ctx context.Context, username, passwordHash string, role Role) (*User, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (username, password_hash, role, created_at)
		SELECT ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM users)`, username, passwordHash, role, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrConflict
	}
	id, _ := res.LastInsertId()
	return s.GetUser(ctx, id)
}

// GetUser loads a user by id.
func (s *Store) GetUser(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

// GetUserByUsername loads a user by username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE username = ?`, username))
}

// ListUsers returns all users.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// UpdateUser saves role, disabled flag and password hash.
func (s *Store) UpdateUser(ctx context.Context, u *User) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET role = ?, disabled = ?, password_hash = ? WHERE id = ?`,
		u.Role, u.Disabled, u.PasswordHash, u.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if u.Disabled {
		_, err = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, u.ID)
	}
	return err
}

// DeleteUser removes a user along with their sessions and tokens.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateSession stores a hashed session token.
func (s *Store) CreateSession(ctx context.Context, tokenHash string, userID int64, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		tokenHash, userID, time.Now().Unix(), expires.Unix())
	return err
}

// SessionUser returns the user owning a live session.
func (s *Store) SessionUser(ctx context.Context, tokenHash string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT u.id, u.username, u.password_hash, u.role, u.disabled, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ? AND u.disabled = 0`, tokenHash, time.Now().Unix()))
}

// DeleteSession removes a session.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// APIToken is a long-lived bearer token for automation.
type APIToken struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"userId"`
	Username   string     `json:"username"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Role       Role       `json:"role"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

// CreateToken stores a hashed API token.
func (s *Store) CreateToken(ctx context.Context, userID int64, name, prefix, tokenHash string, role Role, expires *time.Time) (*APIToken, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO api_tokens (user_id, name, prefix, token_hash, role, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, name, prefix, tokenHash, role, time.Now().Unix(), unixOrNil(expires))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	toks, err := s.listTokens(ctx, `WHERE t.id = ?`, id)
	if err != nil {
		return nil, err
	}
	return &toks[0], nil
}

// ListTokens returns tokens, all of them when userID is 0.
func (s *Store) ListTokens(ctx context.Context, userID int64) ([]APIToken, error) {
	if userID == 0 {
		return s.listTokens(ctx, ``)
	}
	return s.listTokens(ctx, `WHERE t.user_id = ?`, userID)
}

func (s *Store) listTokens(ctx context.Context, where string, args ...any) ([]APIToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.user_id, u.username, t.name, t.prefix, t.role, t.created_at, t.expires_at, t.last_used_at
		FROM api_tokens t JOIN users u ON u.id = t.user_id `+where+` ORDER BY t.created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		var t APIToken
		var created int64
		var exp, used sql.NullInt64
		if err := rows.Scan(&t.ID, &t.UserID, &t.Username, &t.Name, &t.Prefix, &t.Role, &created, &exp, &used); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(created, 0).UTC()
		t.ExpiresAt = timePtr(exp)
		t.LastUsedAt = timePtr(used)
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteToken removes a token. When userID is non-zero the token must belong
// to that user.
func (s *Store) DeleteToken(ctx context.Context, id, userID int64) error {
	q, args := `DELETE FROM api_tokens WHERE id = ?`, []any{id}
	if userID != 0 {
		q += ` AND user_id = ?`
		args = append(args, userID)
	}
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TokenUser resolves an API token to its user and the token's own role, and
// records its use.
func (s *Store) TokenUser(ctx context.Context, tokenHash string) (*User, Role, error) {
	var tokID int64
	var role Role
	var exp sql.NullInt64
	var userID int64
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, role, expires_at FROM api_tokens WHERE token_hash = ?`, tokenHash).
		Scan(&tokID, &userID, &role, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if exp.Valid && exp.Int64 < time.Now().Unix() {
		return nil, "", ErrNotFound
	}
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	if u.Disabled {
		return nil, "", ErrNotFound
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, time.Now().Unix(), tokID)
	return u, Min(role, u.Role), nil
}
