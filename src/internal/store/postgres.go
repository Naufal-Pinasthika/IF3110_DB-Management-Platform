// Package store owns PostgreSQL connections rather than permission policy.
package store

import (
	"context"
	"database/sql"

	"gorm.io/gorm"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

type DatabaseRepository interface {
	GetDatabase(context.Context, string) (models.ManagedDatabase, error)
}

type CredentialCipher interface {
	Encrypt(string) (string, error)
	Decrypt(string) (string, error)
}

type TargetStore struct {
	Databases DatabaseRepository
	Cipher    CredentialCipher
}

// OpenPlatform opens the metadata database rather than a managed target.
func OpenPlatform(ctx context.Context, databaseURL string) (*gorm.DB, error) {
	// TODO Use gorm.Open with postgres.Open and disable automatic ping until context is set.
	// TODO Silence SQL logging so errors cannot expose hashes or credentials.
	// TODO Get the underlying sql.DB and configure the pool and PingContext with a timeout.
	// TODO Close a pool after a failed ping and never log its DSN.
	// TODO Let the composition root close the pool during shutdown.
	// TODO Apply versioned SQL migrations separately and never call AutoMigrate here.
	return nil, apperror.ErrNotImplemented
}

// Reader opens a connection with only the reader credentials.
func (s *TargetStore) Reader(ctx context.Context, databaseID string) (*sql.DB, error) {
	// TODO Load current settings and reject a disabled target.
	// TODO Decrypt reader credentials and safely encode the connection string.
	// TODO Open and ping with a timeout without falling back to the migrator.
	// TODO Use the pgx driver registered by the GORM PostgreSQL driver for raw target SQL.
	// TODO Define pool ownership and close or invalidate pools after config changes.
	return nil, apperror.ErrNotImplemented
}

// Migrator opens a connection for authorized migration execution.
func (s *TargetStore) Migrator(ctx context.Context, databaseID string) (*sql.DB, error) {
	// TODO Load current settings and reject a disabled target.
	// TODO Decrypt migrator credentials and open and ping with a timeout.
	// TODO Keep user authorization in the service before using this connection.
	return nil, apperror.ErrNotImplemented
}

// TestConnection checks both configured roles without changing target data.
func (s *TargetStore) TestConnection(ctx context.Context, databaseID string) error {
	// TODO Ping both reader and migrator and check their expected privileges.
	// TODO Close temporary connections and return sanitized connection errors.
	return apperror.ErrNotImplemented
}
