package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

// CommandReceipt is an early exact-retry check for services that perform slow
// observation before a write. The caller must establish current authority first.
// A miss grants nothing: Transact still rechecks the receipt and all state
// preconditions at publication. Do not use this to bypass scoped authorization.
func (s *Store) CommandReceipt(ctx context.Context, id string, request []byte) (json.RawMessage, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var digest string
	var result []byte
	err := s.db.QueryRowContext(ctx, "SELECT request_hash,result FROM commands WHERE id=?", id).Scan(&digest, &result)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if digest != hash(request) {
		return nil, true, ErrConflict
	}
	return result, true, nil
}
