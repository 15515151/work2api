package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Snapshot creates a consistent independent database, never a copy of live WAL files.
func (d *DB) Snapshot(ctx context.Context, path string) error {
	if _, err := d.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return err
	}
	copy, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer copy.Close()
	var result string
	if err := copy.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("snapshot integrity: %s", result)
	}
	return nil
}

func (d *DB) QuickCheck(ctx context.Context) error {
	var value string
	if err := d.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&value); err != nil {
		return err
	}
	if value != "ok" {
		return fmt.Errorf("database quick_check failed")
	}
	return nil
}

func (d *DB) CheckEncryptedApps(check func(string, string) bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	rows, err := d.db.Query("SELECT key_enc,key_hash FROM apps WHERE key_enc IS NOT NULL AND key_enc!=''")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var enc, hash string
		if err = rows.Scan(&enc, &hash); err != nil {
			return err
		}
		if !check(enc, hash) {
			return fmt.Errorf("existing API Key ciphertext cannot be verified")
		}
	}
	return rows.Err()
}
