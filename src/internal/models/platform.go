// Package models defines the seven platform metadata tables in one place.
// Keep these mappings aligned with db/migrations/000001_create_platform_schema.sql.
// SQL migrations own enum creation and check constraints and schema changes.
// Do not use AutoMigrate to replace those migrations.
package models

import "time"

type AccessLevel string
const (
	AccessViewOnly AccessLevel = "VIEW_ONLY"
	AccessViewEdit AccessLevel = "VIEW_EDIT"
)

type MigrationStatus string
const (
	MigrationDraft MigrationStatus = "DRAFT"
	MigrationRunning MigrationStatus = "RUNNING"
	MigrationApplied MigrationStatus = "APPLIED"
	MigrationFailed MigrationStatus = "FAILED"
)

// User stores a platform account rather than an API response.
type User struct {
	ID string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Username string `gorm:"type:varchar(64);not null;uniqueIndex:users_username_key"`
	PasswordHash string `gorm:"type:text;not null" json:"-"`
	IsPlatformAdmin bool `gorm:"not null;default:false"`
	IsActive bool `gorm:"not null;default:true"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
}

func (User) TableName() string { return "users" }

type Group struct {
	ID string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name string `gorm:"type:varchar(100);not null;uniqueIndex:groups_name_key"`
}

func (Group) TableName() string { return "groups" }

// UserGroup is the explicit join model for membership in multiple groups.
type UserGroup struct {
	UserID string `gorm:"type:uuid;primaryKey;not null"`
	GroupID string `gorm:"type:uuid;primaryKey;not null;index:user_groups_group_id_idx"`

	User User `gorm:"foreignKey:UserID;references:ID;constraint:user_groups_user_id_fkey,OnDelete:CASCADE" json:"-"`
	Group Group `gorm:"foreignKey:GroupID;references:ID;constraint:user_groups_group_id_fkey,OnDelete:CASCADE" json:"-"`
}

func (UserGroup) TableName() string { return "user_groups" }

// ManagedDatabase holds connection settings and encrypted target credentials.
type ManagedDatabase struct {
	ID string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Label string `gorm:"type:varchar(100);not null;uniqueIndex:managed_databases_label_key"`
	Host string `gorm:"type:varchar(255);not null"`
	Port int `gorm:"type:integer;not null;default:5432"`
	DatabaseName string `gorm:"type:varchar(100);not null"`
	SSLMode string `gorm:"type:varchar(20);not null"`
	ReaderUsername string `gorm:"type:varchar(100);not null"`
	ReaderPasswordEnc string `gorm:"type:text;not null" json:"-"`
	MigratorUsername string `gorm:"type:varchar(100);not null"`
	MigratorPasswordEnc string `gorm:"type:text;not null" json:"-"`
	IsEnabled bool `gorm:"not null;default:true"`
}

func (ManagedDatabase) TableName() string { return "managed_databases" }

// GroupAccess stores one permission for a group and database pair.
type GroupAccess struct {
	GroupID string `gorm:"type:uuid;primaryKey;not null"`
	DatabaseID string `gorm:"type:uuid;primaryKey;not null;index:group_access_database_id_idx"`
	AccessLevel AccessLevel `gorm:"type:access_level;not null"`
	Group Group `gorm:"foreignKey:GroupID;references:ID;constraint:group_access_group_id_fkey,OnDelete:CASCADE" json:"-"`
	Database ManagedDatabase `gorm:"foreignKey:DatabaseID;references:ID;constraint:group_access_database_id_fkey,OnDelete:CASCADE" json:"-"`
}

func (GroupAccess) TableName() string { return "group_access" }

// Session stores token hashes only and never stores a raw cookie identifier.
type Session struct {
	ID string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"-"`
	UserID string `gorm:"type:uuid;not null;index:sessions_user_id_idx"`
	TokenHash string `gorm:"type:char(64);not null;uniqueIndex:sessions_token_hash_key" json:"-"`
	PreviousTokenHash *string `gorm:"type:char(64);index:sessions_previous_token_hash_idx,where:previous_token_hash IS NOT NULL" json:"-"`
	PreviousValidUntil *time.Time `gorm:"type:timestamptz"`
	CSRFTokenHash string `gorm:"type:char(64);not null" json:"-"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	RotatedAt time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	LastSeenAt time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	AbsoluteExpiresAt time.Time `gorm:"type:timestamptz;not null;index:sessions_active_expiration_idx,where:revoked_at IS NULL"`
	RevokedAt *time.Time `gorm:"type:timestamptz"`

	User User `gorm:"foreignKey:UserID;references:ID;constraint:sessions_user_id_fkey,OnDelete:CASCADE" json:"-"`
}

func (Session) TableName() string { return "sessions" }

// Migration stores a draft or immutable execution history for one target.
// A nil Version preserves SQL NULL until a migration has been applied.
type Migration struct {
	ID string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	DatabaseID string `gorm:"type:uuid;not null;uniqueIndex:migrations_version_unique,priority:1;index:migrations_database_status_idx,priority:1"`
	Name string `gorm:"type:varchar(200);not null"`
	SQLText string `gorm:"type:text;not null"`
	Status MigrationStatus `gorm:"type:migration_status;not null;default:'DRAFT';index:migrations_database_status_idx,priority:2"`
	Version *int64 `gorm:"type:bigint;uniqueIndex:migrations_version_unique,priority:2"`
	CreatedBy string `gorm:"type:uuid;not null"`
	ExecutedBy *string `gorm:"type:uuid"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	ExecutedAt *time.Time `gorm:"type:timestamptz"`
	ErrorSummary *string `gorm:"type:text"`
	Database ManagedDatabase `gorm:"foreignKey:DatabaseID;references:ID;constraint:migrations_database_id_fkey,OnDelete:RESTRICT" json:"-"`
	Creator User `gorm:"foreignKey:CreatedBy;references:ID;constraint:migrations_created_by_fkey,OnDelete:RESTRICT" json:"-"`
	Executor *User `gorm:"foreignKey:ExecutedBy;references:ID;constraint:migrations_executed_by_fkey,OnDelete:RESTRICT" json:"-"`
}

func (Migration) TableName() string { return "migrations" }
