package mysql

import (
	"testing"
	"testing/fstest"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/stub"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMigrator(t *testing.T) *Migrator {
	t.Helper()

	fs := fstest.MapFS{
		"migrations/000001_create_users.up.sql":   {Data: []byte("CREATE TABLE users (id INT PRIMARY KEY);")},
		"migrations/000001_create_users.down.sql": {Data: []byte("DROP TABLE users;")},
		"migrations/000002_add_email.up.sql":      {Data: []byte("ALTER TABLE users ADD COLUMN email TEXT;")},
		"migrations/000002_add_email.down.sql":    {Data: []byte("ALTER TABLE users DROP COLUMN email;")},
	}

	source, err := iofs.New(fs, "migrations")
	require.NoError(t, err)

	driver, err := stub.WithInstance(nil, &stub.Config{})
	require.NoError(t, err)

	m, err := migrate.NewWithInstance("iofs", source, "stub", driver)
	require.NoError(t, err)

	return &Migrator{m: m}
}

func TestMigrator_Up(t *testing.T) {
	mg := newTestMigrator(t)
	defer mg.Close()

	err := mg.Up()
	require.NoError(t, err)

	v, dirty, err := mg.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(2), v)
	assert.False(t, dirty)
}

func TestMigrator_Up_AlreadyUpToDate(t *testing.T) {
	mg := newTestMigrator(t)
	defer mg.Close()

	require.NoError(t, mg.Up())

	// Second call should succeed (no-op)
	err := mg.Up()
	assert.NoError(t, err)
}

func TestMigrator_Steps(t *testing.T) {
	mg := newTestMigrator(t)
	defer mg.Close()

	// Step up once
	require.NoError(t, mg.Steps(1))
	v, _, err := mg.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(1), v)

	// Step up again
	require.NoError(t, mg.Steps(1))
	v, _, err = mg.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(2), v)
}

func TestMigrator_MigrateTo(t *testing.T) {
	mg := newTestMigrator(t)
	defer mg.Close()

	require.NoError(t, mg.MigrateTo(1))
	v, _, err := mg.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(1), v)
}

func TestMigrator_Down(t *testing.T) {
	mg := newTestMigrator(t)
	defer mg.Close()

	require.NoError(t, mg.Up())
	require.NoError(t, mg.Down())

	v, _, err := mg.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(0), v)
}

func TestMigrator_Version_NoMigrations(t *testing.T) {
	mg := newTestMigrator(t)
	defer mg.Close()

	v, dirty, err := mg.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(0), v)
	assert.False(t, dirty)
}

func TestMigrator_Force(t *testing.T) {
	mg := newTestMigrator(t)
	defer mg.Close()

	require.NoError(t, mg.Force(2))
	v, dirty, err := mg.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(2), v)
	assert.False(t, dirty)
}
