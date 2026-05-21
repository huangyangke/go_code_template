package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_FileNotExist(t *testing.T) {
	// New() 现在会跳过不存在的配置文件，但不会报错
	ldr, err := New("/nonexistent/path/config.yaml")
	require.NoError(t, err)
	assert.NotNil(t, ldr)
	// 未加载任何配置，config 是空的
	assert.Empty(t, ldr.Raw())
}

func TestNew_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
app:
  name: test
  port: 8080
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	ldr, err := New(path)
	require.NoError(t, err)
	assert.NotNil(t, ldr)
}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
app:
  name: myservice
  port: 9090
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	var cfg struct {
		Name string `yaml:"name"`
		Port int    `yaml:"port"`
	}
	require.NoError(t, ldr.Scan("app", &cfg))
	assert.Equal(t, "myservice", cfg.Name)
	assert.Equal(t, 9090, cfg.Port)
}

func TestScanRoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `name: toplevel`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	var cfg struct {
		Name string `yaml:"name"`
	}
	require.NoError(t, ldr.Scan("", &cfg))
	assert.Equal(t, "toplevel", cfg.Name)
}

func TestMultipleLoaders(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "a.yaml")
	path2 := filepath.Join(dir, "b.yaml")
	require.NoError(t, os.WriteFile(path1, []byte("val: 1"), 0644))
	require.NoError(t, os.WriteFile(path2, []byte("val: 2"), 0644))

	l1, err := New(path1)
	require.NoError(t, err)
	l2, err := New(path2)
	require.NoError(t, err)

	var v1, v2 struct{ Val int `yaml:"val"` }
	require.NoError(t, l1.Scan("", &v1))
	require.NoError(t, l2.Scan("", &v2))
	assert.Equal(t, 1, v1.Val)
	assert.Equal(t, 2, v2.Val)
}

func TestVariableSubstitution(t *testing.T) {
	os.Setenv("APP_PORT", "9090")
	os.Setenv("DB_USER", "admin")
	os.Setenv("DEBUG", "true")
	defer func() {
		os.Unsetenv("APP_PORT")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DEBUG")
	}()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
app:
  host: "127.0.0.1"
  port: ${APP_PORT:8080}
  env_tag: "prod_${APP_PORT}"
database:
  user: ${DB_USER}
  url: "mysql://${database.user}@${app.host}:${app.port}/db"
debug_mode: ${DEBUG:false}
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	assert.Equal(t, 9090, ldr.GetInt("app.port"))
	assert.Equal(t, "prod_9090", ldr.GetString("app.env_tag"))
	assert.Equal(t, "admin", ldr.GetString("database.user"))
	assert.Equal(t, "mysql://admin@127.0.0.1:9090/db", ldr.GetString("database.url"))
	assert.True(t, ldr.GetBool("debug_mode"))
}

func TestEnvFileLoading(t *testing.T) {
	dir := t.TempDir()

	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte(`
APP_PORT=8080
DB_NAME=mydb
SECRET_KEY=test_secret
`), 0644))

	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
app:
  port: ${APP_PORT}
database:
  name: ${DB_NAME}
security:
  secret: ${SECRET_KEY}
`), 0644))

	ldr, err := New(cfgPath, WithEnvFile(envPath))
	require.NoError(t, err)

	assert.Equal(t, 8080, ldr.GetInt("app.port"))
	assert.Equal(t, "mydb", ldr.GetString("database.name"))
	assert.Equal(t, "test_secret", ldr.GetString("security.secret"))
}

func TestWithMultipleEnvFiles(t *testing.T) {
	dir := t.TempDir()

	env1Path := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(env1Path, []byte(`
APP_PORT=8080
DB_NAME=mydb
`), 0644))

	env2Path := filepath.Join(dir, ".env.local")
	require.NoError(t, os.WriteFile(env2Path, []byte(`
APP_PORT=9090
SECRET_KEY=local_secret
`), 0644))

	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
app:
  port: ${APP_PORT}
database:
  name: ${DB_NAME}
security:
  secret: ${SECRET_KEY}
`), 0644))

	ldr, err := New(cfgPath, WithEnvFiles([]string{env1Path, env2Path}), WithOverrideEnv(true))
	require.NoError(t, err)

	assert.Equal(t, 9090, ldr.GetInt("app.port"))       // 第二个文件覆盖第一个
	assert.Equal(t, "mydb", ldr.GetString("database.name")) // 只在第一个文件中定义
	assert.Equal(t, "local_secret", ldr.GetString("security.secret")) // 只在第二个文件中定义
}

func TestGetStringSlice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
tags:
  - alpha
  - beta
  - gamma
empty_list: []
numbers:
  - 1
  - 2
  - 3
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	// Normal string slice
	result := ldr.GetStringSlice("tags")
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, result)

	// Numbers converted to strings
	nums := ldr.GetStringSlice("numbers")
	assert.Equal(t, []string{"1", "2", "3"}, nums)

	// Empty list
	empty := ldr.GetStringSlice("empty_list")
	assert.Equal(t, []string{}, empty)

	// Non-existent key with default
	def := ldr.GetStringSlice("nonexistent", []string{"fallback"})
	assert.Equal(t, []string{"fallback"}, def)

	// Non-existent key without default
	assert.Nil(t, ldr.GetStringSlice("nonexistent"))
}

func TestGetMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
database:
  host: localhost
  port: 5432
scalar: hello
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	// Normal map
	m := ldr.GetMap("database")
	assert.Equal(t, "localhost", m["host"])
	assert.Equal(t, 5432, m["port"])

	// Scalar value returns default
	def := map[string]interface{}{"fallback": true}
	assert.Equal(t, def, ldr.GetMap("scalar", def))

	// Non-existent key
	assert.Nil(t, ldr.GetMap("nonexistent"))
}

func TestGetDuration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
timeouts:
  connect: "5s"
  read: "100ms"
  write: 30
  idle: "2m30s"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	assert.Equal(t, 5*time.Second, ldr.GetDuration("timeouts.connect"))
	assert.Equal(t, 100*time.Millisecond, ldr.GetDuration("timeouts.read"))
	assert.Equal(t, 30*time.Second, ldr.GetDuration("timeouts.write"))
	assert.Equal(t, 150*time.Second, ldr.GetDuration("timeouts.idle"))

	// Non-existent with default
	assert.Equal(t, 10*time.Second, ldr.GetDuration("nonexistent", 10*time.Second))

	// Non-existent without default
	assert.Equal(t, time.Duration(0), ldr.GetDuration("nonexistent"))
}
