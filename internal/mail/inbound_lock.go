package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type PostgresAdvisoryInboundMessageLocker struct {
	Tx  *sql.Tx
	Ctx context.Context
}

func (l PostgresAdvisoryInboundMessageLocker) TryLockInboundMessageID(messageID string) (func(), bool, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return func() {}, true, nil
	}
	if l.Tx == nil {
		return func() {}, false, fmt.Errorf("postgres advisory inbound message lock requires transaction")
	}
	ctx := l.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	locked := false
	if err := l.Tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1))`, messageID).Scan(&locked); err != nil {
		return func() {}, false, err
	}
	return func() {}, locked, nil
}

type PostgresFetchmailServerLocker struct {
	Tx  *sql.Tx
	Ctx context.Context
}

func (l PostgresFetchmailServerLocker) TryLockFetchmailServer(serverID int64) (func(), bool, error) {
	if serverID == 0 {
		return func() {}, true, nil
	}
	if l.Tx == nil {
		return func() {}, false, fmt.Errorf("postgres fetchmail server lock requires transaction")
	}
	ctx := l.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var id int64
	err := l.Tx.QueryRowContext(ctx, `SELECT id FROM fetchmail_server WHERE id = $1 FOR NO KEY UPDATE SKIP LOCKED`, serverID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return func() {}, false, nil
	}
	if err != nil {
		return func() {}, false, err
	}
	return func() {}, true, nil
}
