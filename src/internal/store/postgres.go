package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	// GORM's postgres driver typically registers "pgx" natively under the hood
	_ "github.com/jackc/pgx/v5/stdlib"

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
	pools     sync.Map // stores *sql.DB instances keyed by "dbID-role"
}

func (s *TargetStore) Configuration(ctx context.Context, databaseID string) (models.ManagedDatabase, error) {
	db, err := s.Databases.GetDatabase(ctx, databaseID)
	if err != nil {
		return db, err
	}
	if !db.IsEnabled {
		return db, apperror.ErrForbidden
	}
	return db, nil
}

// OpenPlatform opens the metadata database rather than a managed target.
func OpenPlatform(ctx context.Context, databaseURL string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, errors.New("could not open platform database")
	}
	pool, err := db.DB()
	if err != nil {
		return nil, errors.New("could not access platform database pool")
	}
	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(30 * time.Minute)

	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.PingContext(ctxPing); err != nil {
		pool.Close()
		return nil, fmt.Errorf("could not connect to platform database: %w", err)
	}
	return db, nil
}

// Reader opens a connection with only the reader credentials.
func (s *TargetStore) Reader(ctx context.Context, databaseID string) (*sql.DB, error) {
	db, err := s.Databases.GetDatabase(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	return s.getOrOpenPool(ctx, db, "reader", db.ReaderUsername, db.ReaderPasswordEnc)
}

// Migrator opens a connection for authorized migration execution.
func (s *TargetStore) Migrator(ctx context.Context, databaseID string) (*sql.DB, error) {
	db, err := s.Databases.GetDatabase(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	return s.getOrOpenPool(ctx, db, "migrator", db.MigratorUsername, db.MigratorPasswordEnc)
}

// TestConnection checks both configured roles without changing target data.
func (s *TargetStore) TestConnection(ctx context.Context, databaseID string) error {
	db, err := s.Databases.GetDatabase(ctx, databaseID)
	if err != nil {
		return err
	}

	return s.TestConfiguration(ctx, db)
}

// TestConfiguration checks unsaved settings without persisting them or creating cached pools.
func (s *TargetStore) TestConfiguration(ctx context.Context, db models.ManagedDatabase) error {

	testRole := func(username, encryptedPass string) error {
		pass, err := s.Cipher.Decrypt(encryptedPass)
		if err != nil {
			return errors.New("failed to decrypt test credentials")
		}

		driver, dsn, err := targetDSN(db, username, pass)
		if err != nil {
			return err
		}
		conn, err := sql.Open(driver, dsn)
		if err != nil {
			return errors.New("driver initialization failed")
		}
		defer conn.Close()

		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		if err := conn.PingContext(pingCtx); err != nil {
			return fmt.Errorf("connection test failed: %w", err)
		}
		return nil
	}

	if err := testRole(db.ReaderUsername, db.ReaderPasswordEnc); err != nil {
		return fmt.Errorf("reader (%s): %v", db.ReaderUsername, err)
	}
	if err := testRole(db.MigratorUsername, db.MigratorPasswordEnc); err != nil {
		return fmt.Errorf("migrator (%s): %v", db.MigratorUsername, err)
	}

	return nil
}

// InvalidateTargetPools safely drops all cached connection pools for a given database target.
// Call this whenever a database's configuration (host, port, credentials) is updated or disabled.
func (s *TargetStore) InvalidateTargetPools(databaseID string) {
	roles := []string{"reader", "migrator"}
	for _, role := range roles {
		key := databaseID + "-" + role
		if val, ok := s.pools.LoadAndDelete(key); ok {
			val.(*sql.DB).Close()
		}
	}
}

// getOrOpenPool manages the thread-safe connection cache and invalidates dead connections.
func (s *TargetStore) getOrOpenPool(ctx context.Context, db models.ManagedDatabase, role, username, encryptedPass string) (*sql.DB, error) {
	if !db.IsEnabled {
		return nil, apperror.ErrForbidden
	}

	cacheKey := db.ID + "-" + role
	if val, ok := s.pools.Load(cacheKey); ok {
		pool := val.(*sql.DB)
		// Quick check to see if the cached connection is still alive
		if err := pool.PingContext(ctx); err == nil {
			return pool, nil
		}
		pool.Close()
		s.pools.Delete(cacheKey)
	}

	pass, err := s.Cipher.Decrypt(encryptedPass)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt target credentials: %w", err)
	}

	driver, dsn, err := targetDSN(db, username, pass)
	if err != nil {
		return nil, err
	}
	pool, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database pool: %w", err)
	}

	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(2)
	pool.SetConnMaxLifetime(15 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("could not establish connection to target database: %w", err)
	}

	s.pools.Store(cacheKey, pool)
	return pool, nil
}

// MySQL's driver keeps multiStatements disabled; migration statements execute individually.
func targetDSN(db models.ManagedDatabase, username, password string) (string, string, error) {
	switch db.TargetEngine() {
	case models.EnginePostgres:
		return "pgx", buildDSN(db, username, password), nil
	case models.EngineMySQL:
		cfg := mysql.NewConfig()
		cfg.User, cfg.Passwd = username, password
		cfg.Net = "tcp"
		port := db.Port
		if port <= 0 {
			port = 3306
		}
		cfg.Addr = net.JoinHostPort(db.Host, strconv.Itoa(port))
		cfg.DBName = db.DatabaseName
		cfg.ParseTime = true
		cfg.Timeout = 3 * time.Second
		switch db.SSLMode {
		case "", "disable":
			cfg.TLSConfig = "false"
		case "require":
			cfg.TLSConfig = "skip-verify"
		case "verify-full":
			cfg.TLSConfig = "true"
		default:
			return "", "", fmt.Errorf("%w: unsupported MySQL SSL mode", apperror.ErrInvalidInput)
		}
		return "mysql", cfg.FormatDSN(), nil
	default:
		return "", "", fmt.Errorf("%w: unsupported database engine", apperror.ErrInvalidInput)
	}
}

func buildDSN(db models.ManagedDatabase, username, password string) string {
	host := db.Host
	if host == "" {
		host = "postgres"
	}
	port := db.Port
	if port <= 0 {
		port = 5432
	}
	dbName := db.DatabaseName
	if dbName == "" {
		dbName = "target_db"
	}
	ssl := db.SSLMode
	if ssl == "" {
		ssl = "disable"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(username),
		url.QueryEscape(password),
		host,
		port,
		url.QueryEscape(dbName),
		ssl,
	)
}
