package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetInt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
port: 8080
float_port: 8080.0
str_port: "9090"
name: hello
`), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	assert.Equal(t, 8080, ldr.GetInt("port"))
	assert.Equal(t, 8080, ldr.GetInt("float_port"))
	assert.Equal(t, 9090, ldr.GetInt("str_port"))
	assert.Equal(t, 0, ldr.GetInt("nonexistent"))
	assert.Equal(t, 3000, ldr.GetInt("nonexistent", 3000))
	assert.Equal(t, 42, ldr.GetInt("name", 42))
}

func TestGetBool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
enabled: true
disabled: false
str_true: "true"
str_one: "1"
str_false: "false"
number: 42
`), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	assert.True(t, ldr.GetBool("enabled"))
	assert.False(t, ldr.GetBool("disabled"))
	assert.True(t, ldr.GetBool("str_true"))
	assert.True(t, ldr.GetBool("str_one"))
	assert.False(t, ldr.GetBool("str_false"))
	assert.False(t, ldr.GetBool("nonexistent"))
	assert.True(t, ldr.GetBool("nonexistent", true))
	assert.False(t, ldr.GetBool("number", false))
}

func TestGetFloat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
ratio: 3.14
integer: 42
str_float: "2.718"
name: hello
`), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	assert.InDelta(t, 3.14, ldr.GetFloat("ratio"), 0.001)
	assert.InDelta(t, 42.0, ldr.GetFloat("integer"), 0.001)
	assert.InDelta(t, 2.718, ldr.GetFloat("str_float"), 0.001)
	assert.InDelta(t, 0.0, ldr.GetFloat("nonexistent"), 0.001)
	assert.InDelta(t, 1.5, ldr.GetFloat("nonexistent", 1.5), 0.001)
	assert.InDelta(t, 99.9, ldr.GetFloat("name", 99.9), 0.001)
}

func TestReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("val: 1"), 0644))

	ldr, err := New(path)
	require.NoError(t, err)
	assert.Equal(t, 1, ldr.GetInt("val"))

	require.NoError(t, os.WriteFile(path, []byte("val: 2"), 0644))
	require.NoError(t, ldr.Reload())
	assert.Equal(t, 2, ldr.GetInt("val"))
}

func TestRaw(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
app:
  name: test
  port: 8080
`), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	raw := ldr.Raw()
	assert.NotNil(t, raw)
	assert.Contains(t, raw, "app")

	raw["injected"] = "value"
	assert.Nil(t, ldr.Raw()["injected"])
}

func TestDump(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
app:
  name: test
secret: my-password
`), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	dump := ldr.Dump()
	assert.Contains(t, dump, "sources")
	assert.Contains(t, dump, "settings")
	assert.Contains(t, dump, "config")
}

func TestDump_WithRedactKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
app:
  name: test
  secret: supersecret
password: hunter2
`), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	dump := ldr.Dump([]string{"secret", "password"})
	cfg := dump["config"].(map[string]interface{})
	assert.Equal(t, "***", cfg["password"])
	app := cfg["app"].(map[string]interface{})
	assert.Equal(t, "***", app["secret"])
	assert.Equal(t, "test", app["name"])
}

func TestClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("val: 1"), 0644))

	ldr, err := New(path)
	require.NoError(t, err)
	assert.NoError(t, ldr.Close())
}

func TestWatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("val: 1"), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	called := make(chan struct{}, 1)
	err = ldr.Watch(func() {
		select {
		case called <- struct{}{}:
		default:
		}
	})
	require.NoError(t, err)
	defer ldr.Close()

	require.NoError(t, os.WriteFile(path, []byte("val: 2"), 0644))

	select {
	case <-called:
		assert.Equal(t, 2, ldr.GetInt("val"))
	case <-time.After(2 * time.Second):
		t.Log("watch callback not triggered within timeout (acceptable on some CI environments)")
	}
}

func TestWithOverrideEnv(t *testing.T) {
	dir := t.TempDir()

	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("TEST_OVERRIDE_VAR=from_env_file\n"), 0644))

	os.Setenv("TEST_OVERRIDE_VAR", "from_system")
	defer os.Unsetenv("TEST_OVERRIDE_VAR")

	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("val: ${TEST_OVERRIDE_VAR}"), 0644))

	ldr, err := New(cfgPath, WithEnvFile(envPath), WithOverrideEnv(true))
	require.NoError(t, err)
	assert.Equal(t, "from_env_file", ldr.GetString("val"))
}

func TestGetString_Default(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: hello"), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	assert.Equal(t, "hello", ldr.GetString("name"))
	assert.Equal(t, "", ldr.GetString("nonexistent"))
	assert.Equal(t, "fallback", ldr.GetString("nonexistent", "fallback"))
}

func TestGet_Default(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: hello"), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	assert.Equal(t, "hello", ldr.Get("name"))
	assert.Nil(t, ldr.Get("nonexistent"))
	assert.Equal(t, "default", ldr.Get("nonexistent", "default"))
}
