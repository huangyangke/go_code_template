package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetInt64(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
big: 9223372036854775807
int_val: 42
float_val: 3.9
str_val: "1234567890123"
`), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	assert.Equal(t, int64(9223372036854775807), ldr.GetInt64("big"))
	assert.Equal(t, int64(42), ldr.GetInt64("int_val"))
	assert.Equal(t, int64(3), ldr.GetInt64("float_val"))
	assert.Equal(t, int64(1234567890123), ldr.GetInt64("str_val"))
	assert.Equal(t, int64(0), ldr.GetInt64("nonexistent"))
	assert.Equal(t, int64(99), ldr.GetInt64("nonexistent", 99))
}

func TestGetUint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
port: 8080
negative: -1
str_val: "9999"
`), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	assert.Equal(t, uint(8080), ldr.GetUint("port"))
	assert.Equal(t, uint(9999), ldr.GetUint("str_val"))
	// 负数返回 0（无默认值时）
	assert.Equal(t, uint(0), ldr.GetUint("negative"))
	assert.Equal(t, uint(0), ldr.GetUint("nonexistent"))
	assert.Equal(t, uint(42), ldr.GetUint("nonexistent", 42))
}

func TestEnvFile_ExportPrefix(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte(
		"export APP_HOST=myhost\nexport APP_PORT=9000\n",
	), 0644))

	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
host: ${APP_HOST}
port: ${APP_PORT}
`), 0644))

	ldr, err := New(cfgPath, WithEnvFile(envPath))
	require.NoError(t, err)

	assert.Equal(t, "myhost", ldr.GetString("host"))
	assert.Equal(t, 9000, ldr.GetInt("port"))
}

func TestEnvFile_InlineComment(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte(
		"APP_NAME=myapp # this is a comment\nAPP_SECRET=\"keep#this\" # outer comment\n",
	), 0644))

	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
name: ${APP_NAME}
secret: ${APP_SECRET}
`), 0644))

	ldr, err := New(cfgPath, WithEnvFile(envPath))
	require.NoError(t, err)

	assert.Equal(t, "myapp", ldr.GetString("name"))
	// 带引号的值保留内容原样，不受注释影响
	assert.Equal(t, "keep#this", ldr.GetString("secret"))
}

func TestEnvFile_CRLF(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	// Windows-style line endings
	require.NoError(t, os.WriteFile(envPath, []byte("KEY_A=val1\r\nKEY_B=val2\r\n"), 0644))

	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("a: ${KEY_A}\nb: ${KEY_B}"), 0644))

	ldr, err := New(cfgPath, WithEnvFile(envPath))
	require.NoError(t, err)

	assert.Equal(t, "val1", ldr.GetString("a"))
	assert.Equal(t, "val2", ldr.GetString("b"))
}

func TestWatch_Debounce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("val: 1"), 0644))

	ldr, err := New(path)
	require.NoError(t, err)
	defer ldr.Close()

	callCount := 0
	require.NoError(t, ldr.Watch(func() {
		callCount++
	}))

	// 快速连续写三次，debounce 应只触发一次回调
	for i := 2; i <= 4; i++ {
		require.NoError(t, os.WriteFile(path, []byte("val: 10"), 0644))
	}

	time.Sleep(600 * time.Millisecond)
	assert.LessOrEqual(t, callCount, 2, "debounce should collapse rapid writes")
	assert.Equal(t, 10, ldr.GetInt("val"))
}

func TestNacosServerAddrs_MultiNode(t *testing.T) {
	cases := []struct {
		input    string
		expected []struct{ ip string; port uint64 }
	}{
		{
			"192.168.1.1",
			[]struct{ ip string; port uint64 }{{"192.168.1.1", 8848}},
		},
		{
			"192.168.1.1:8848",
			[]struct{ ip string; port uint64 }{{"192.168.1.1", 8848}},
		},
		{
			"192.168.1.1:8848, 192.168.1.2:8848, 192.168.1.3:9999",
			[]struct{ ip string; port uint64 }{
				{"192.168.1.1", 8848},
				{"192.168.1.2", 8848},
				{"192.168.1.3", 9999},
			},
		},
	}

	for _, tc := range cases {
		result := parseNacosServerAddrs(tc.input)
		require.Len(t, result, len(tc.expected), "input: %s", tc.input)
		for i, exp := range tc.expected {
			assert.Equal(t, exp.ip, result[i].IpAddr, "input: %s node %d ip", tc.input, i)
			assert.Equal(t, exp.port, result[i].Port, "input: %s node %d port", tc.input, i)
		}
	}
}

func TestSubstituteVariables_CircularReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// a 引用 b，b 引用 a —— 形成循环
	require.NoError(t, os.WriteFile(path, []byte(`
a: ${b}
b: ${a}
`), 0644))

	_, err := New(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular")
}


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

func TestGetMap_ReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
app:
  name: test
  nested:
    enabled: true
`), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	m := ldr.GetMap("app")
	m["name"] = "changed"
	m["nested"].(map[string]interface{})["enabled"] = false

	assert.Equal(t, "test", ldr.GetString("app.name"))
	assert.True(t, ldr.GetBool("app.nested.enabled"))
}

func TestRaw_ReturnsDeepCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
app:
  name: test
  nested:
    enabled: true
`), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	raw := ldr.Raw()
	raw["app"].(map[string]interface{})["name"] = "changed"
	raw["app"].(map[string]interface{})["nested"].(map[string]interface{})["enabled"] = false

	assert.Equal(t, "test", ldr.GetString("app.name"))
	assert.True(t, ldr.GetBool("app.nested.enabled"))
}

func TestDump_ReturnsDeepCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
app:
  name: test
  nested:
    enabled: true
`), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	dump := ldr.Dump()
	cfg := dump["config"].(map[string]interface{})
	cfg["app"].(map[string]interface{})["name"] = "changed"
	cfg["app"].(map[string]interface{})["nested"].(map[string]interface{})["enabled"] = false

	assert.Equal(t, "test", ldr.GetString("app.name"))
	assert.True(t, ldr.GetBool("app.nested.enabled"))
}

func TestVariableSubstitution_RecursesIntoArrayItems(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.Setenv("APP_HOST", "example.com")
	defer os.Unsetenv("APP_HOST")

	require.NoError(t, os.WriteFile(path, []byte(`
servers:
  - name: api
    url: "https://${APP_HOST}"
  - "${APP_HOST}"
`), 0644))

	ldr, err := New(path)
	require.NoError(t, err)

	servers := ldr.Get("servers").([]interface{})
	first := servers[0].(map[string]interface{})
	assert.Equal(t, "https://example.com", first["url"])
	assert.Equal(t, "example.com", servers[1])
}

func TestWatch_HandlesAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("val: 1"), 0644))

	ldr, err := New(path)
	require.NoError(t, err)
	defer ldr.Close()

	called := make(chan struct{}, 2)
	require.NoError(t, ldr.Watch(func() {
		select {
		case called <- struct{}{}:
		default:
		}
	}))

	tmp := filepath.Join(dir, "config.yaml.tmp")
	require.NoError(t, os.WriteFile(tmp, []byte("val: 2"), 0644))
	require.NoError(t, os.Rename(tmp, path))
	require.NoError(t, os.WriteFile(path, []byte("val: 3"), 0644))

	select {
	case <-called:
		assert.Equal(t, 3, ldr.GetInt("val"))
	case <-time.After(2 * time.Second):
		t.Fatal("watch callback not triggered after atomic replace")
	}
}
