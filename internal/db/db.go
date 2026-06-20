package db

import (
	"context"
	"database/sql"
	"fmt"
)

const DriverName = "postgres"

type Adapter struct {
	db *sql.DB
}

func Open(driverName, dataSourceName string) (*Adapter, error) {
	if driverName != DriverName {
		return nil, fmt.Errorf("unsupported database driver %q", driverName)
	}
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}
	return &Adapter{db: db}, nil
}

func (a *Adapter) DB() *sql.DB {
	return a.db
}

func (a *Adapter) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func Savepoint(ctx context.Context, tx *sql.Tx, name string) error {
	_, err := tx.ExecContext(ctx, `SAVEPOINT `+name)
	return err
}

func RollbackToSavepoint(ctx context.Context, tx *sql.Tx, name string) error {
	_, err := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT `+name)
	return err
}
