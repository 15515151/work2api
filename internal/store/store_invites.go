package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const inviteColumns = "code,used_by,used_at,expires_at,created_at,max_uses,used_count,enabled,revision"

func migrateInvites(tx *sql.Tx, version int) error {
	if version < 22 {
		for _, column := range []string{"max_uses INTEGER NOT NULL DEFAULT 1 CHECK(max_uses>=0)", "used_count INTEGER NOT NULL DEFAULT 0 CHECK(used_count>=0)", "enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1))", "revision INTEGER NOT NULL DEFAULT 1"} {
			if _, e := tx.Exec("ALTER TABLE invite_codes ADD COLUMN " + column); e != nil && !strings.Contains(e.Error(), "duplicate column") {
				return e
			}
		}
	}
	if _, e := tx.Exec(`CREATE TABLE IF NOT EXISTS invite_redemptions (
 code TEXT NOT NULL REFERENCES invite_codes(code) ON DELETE CASCADE,
 user_id INTEGER NOT NULL, username TEXT NOT NULL DEFAULT '', used_at REAL NOT NULL,
 PRIMARY KEY(code,user_id));`); e != nil {
		return e
	}
	if version < 22 {
		if _, e := tx.Exec("UPDATE invite_codes SET used_count=1 WHERE used_by IS NOT NULL AND used_count=0"); e != nil {
			return e
		}
		_, e := tx.Exec(`INSERT OR IGNORE INTO invite_redemptions(code,user_id,username,used_at)
 SELECT i.code,i.used_by,COALESCE(u.username,''),COALESCE(i.used_at,i.created_at,0)
 FROM invite_codes i LEFT JOIN users u ON u.id=i.used_by WHERE i.used_by IS NOT NULL`)
		return e
	}
	return nil
}

// Increment allowance and record redemption together; registration shares this
// transaction, so failed user creation cannot spend an invitation slot.
func consumeInvite(tx *sql.Tx, code string, userID int64, now float64) (bool, error) {
	res, e := tx.Exec(`UPDATE invite_codes SET used_count=used_count+1,
 used_by=COALESCE(used_by,?),used_at=COALESCE(used_at,?)
 WHERE code=? AND enabled=1 AND (max_uses=0 OR used_count<max_uses)
 AND (expires_at=0 OR expires_at>?)`, userID, now, code, now)
	if e != nil {
		return false, e
	}
	n, e := res.RowsAffected()
	if e != nil || n != 1 {
		return false, e
	}
	_, e = tx.Exec(`INSERT INTO invite_redemptions(code,user_id,username,used_at)
 VALUES(?,?,COALESCE((SELECT username FROM users WHERE id=?),''),?)`, code, userID, userID, now)
	return e == nil, e
}

func (d *DB) UpdateInviteCode(code string, maxUses int64, expires float64, enabled bool, revision int64) error {
	if maxUses < 0 || revision < 1 {
		return fmt.Errorf("invalid invitation settings")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	res, e := d.db.Exec(`UPDATE invite_codes SET max_uses=?,expires_at=?,enabled=?,revision=revision+1
 WHERE code=? AND revision=? AND (?=0 OR used_count<=?)`, maxUses, expires, enabled, code, revision, maxUses, maxUses)
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return ErrConflict
	}
	return nil
}

func (d *DB) InviteRedemptions(code string, offsets ...int) ([]map[string]any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	offset := 0
	if len(offsets) > 0 {
		offset = offsets[0]
	}
	if offset < 0 {
		return nil, fmt.Errorf("invalid offset")
	}
	rows, e := d.db.Query("SELECT user_id,username,used_at FROM invite_redemptions WHERE code=? ORDER BY used_at DESC,user_id DESC LIMIT 51 OFFSET ?", code, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	return scanRows(rows)
}

func (d *DB) consumeInviteCode(code string, userID int64) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, e := d.db.Begin()
	if e != nil {
		return false, e
	}
	defer tx.Rollback()
	ok, e := consumeInvite(tx, code, userID, float64(time.Now().UnixNano())/1e9)
	if e != nil || !ok {
		return false, e
	}
	return true, tx.Commit()
}
