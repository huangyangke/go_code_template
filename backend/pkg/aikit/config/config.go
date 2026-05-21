package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"gopkg.in/yaml.v3"
)

// NacosConfig represents Nacos configuration center settings
type NacosConfig struct {
	ServerAddr  string            `yaml:"server_addr"`
	Namespace   string            `yaml:"namespace"`
	Username    string            `yaml:"username"`
	Password    string            `yaml:"password"`
	AccessKey   string            `yaml:"access_key"`
	SecretKey   string            `yaml:"secret_key"`
	ConfigList  []NacosConfigItem `yaml:"config_list"`
	AutoUpdate  bool              `yaml:"auto_update"`
	SnapshotPath string           `yaml:"snapshot_path"`
}

// NacosConfigItem represents a single Nacos configuration item
type NacosConfigItem struct {
	Group  string `yaml:"group"`
	DataID string `yaml:"data_id"`
}

// ConfigLoader configuration loader, supports multi-file merge, variable substitution, .env support, and Nacos integration
// Reference Python aikit's ConfigLoader implementation
type ConfigLoader struct {
	mu                  sync.RWMutex
	configPaths         []string
	envFiles            []string
	overrideEnv         bool
	enableSubstitution  bool
	config              map[string]interface{}
	watcher             *fsnotify.Watcher

	// Nacos-related fields
	nacosConfig         *NacosConfig
	nacosClient         config_client.IConfigClient
	nacosConfigCache    map[string]interface{}
	nacosCallbacks      []func(map[string]interface{})
	nacosListenerIDs    map[string]string // group+dataId -> listenerId
}

// Option configuration loader option
type Option func(*ConfigLoader)

// WithEnvFile enable .env file support (replace default .env)
func WithEnvFile(path string) Option {
	return func(l *ConfigLoader) {
		l.envFiles = []string{path}
	}
}

// WithEnvFiles enable multiple .env file support (replace default .env)
func WithEnvFiles(paths []string) Option {
	return func(l *ConfigLoader) {
		l.envFiles = paths
	}
}

// WithOverrideEnv whether .env file overrides existing environment variables
func WithOverrideEnv(override bool) Option {
	return func(l *ConfigLoader) {
		l.overrideEnv = override
	}
}

// WithEnableSubstitution enable variable substitution
func WithEnableSubstitution(enable bool) Option {
	return func(l *ConfigLoader) {
		l.enableSubstitution = enable
	}
}

// WithNacosConfig enable Nacos configuration center support
func WithNacosConfig(cfg *NacosConfig) Option {
	return func(l *ConfigLoader) {
		l.nacosConfig = cfg
	}
}

// New creates a configuration loader instance (non-singleton)
func New(configPath string, opts ...Option) (*ConfigLoader, error) {
	return NewFromPaths([]string{configPath}, opts...)
}

// NewFromPaths creates a loader from multiple configuration files
func NewFromPaths(configPaths []string, opts ...Option) (*ConfigLoader, error) {
	l := &ConfigLoader{
		configPaths:        configPaths,
		envFiles:           []string{".env"}, // load .env by default
		overrideEnv:        false,
		enableSubstitution: true,
		config:             make(map[string]interface{}),
		nacosConfigCache:   make(map[string]interface{}),
		nacosCallbacks:     make([]func(map[string]interface{}), 0),
		nacosListenerIDs:   make(map[string]string),
	}

	for _, opt := range opts {
		opt(l)
	}

	if err := l.load(); err != nil {
		return nil, err
	}

	return l, nil
}

// load loads all configuration sources (priority: Nacos > local files)
func (l *ConfigLoader) load() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.config = make(map[string]interface{})

	// 1. Load .env files
	if err := l.loadEnvFiles(); err != nil {
		return err
	}

	// 2. Load local configuration files
	if err := l.loadLocalConfigs(); err != nil {
		return err
	}

	// 3. Load Nacos configuration (if enabled)
	if l.nacosConfig != nil {
		if err := l.loadNacosConfig(); err != nil {
			return err
		}
	}

	// 4. Variable substitution
	if l.enableSubstitution {
		if err := l.substituteVariables(); err != nil {
			return err
		}
	}

	return nil
}

// loadEnvFiles loads .env files
func (l *ConfigLoader) loadEnvFiles() error {
	for _, path := range l.envFiles {
		if _, err := os.Stat(path); err != nil {
			continue // file doesn't exist, skip
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("config: read .env file %s: %w", path, err)
		}

		lines := strings.Split(strings.TrimSpace(string(content)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			// remove quotes
			val = strings.Trim(val, `"`)
			val = strings.Trim(val, `'`)

			if l.overrideEnv || os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}

	return nil
}

// loadLocalConfigs loads local configuration files
func (l *ConfigLoader) loadLocalConfigs() error {
	for _, path := range l.configPaths {
		if _, err := os.Stat(path); err != nil {
			continue
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("config: read file %s: %w", path, err)
		}

		var data map[string]interface{}
		switch {
		case strings.HasSuffix(path, ".json"):
			if err := json.Unmarshal(content, &data); err != nil {
				return fmt.Errorf("config: parse JSON file %s: %w", path, err)
			}
		case strings.HasSuffix(path, ".yaml"), strings.HasSuffix(path, ".yml"):
			if err := yaml.Unmarshal(content, &data); err != nil {
				return fmt.Errorf("config: parse YAML file %s: %w", path, err)
			}
		default:
			continue
		}

		l.deepMerge(l.config, data)
	}

	return nil
}

// loadNacosConfig loads configuration from Nacos and merges with existing config
func (l *ConfigLoader) loadNacosConfig() error {
	if l.nacosConfig == nil {
		return nil
	}

	if err := l.initNacosClient(); err != nil {
		return err
	}

	if err := l.fetchNacosConfigs(); err != nil {
		return err
	}

	// Merge Nacos config (overrides local config)
	l.deepMerge(l.config, l.nacosConfigCache)

	// Save snapshot
	if err := l.saveSnapshot(); err != nil {
		// Don't fail on snapshot error
	}

	// Setup listeners if auto-update is enabled
	if l.nacosConfig.AutoUpdate {
		if err := l.setupNacosListeners(); err != nil {
			// Don't fail on listener setup error
		}
	}

	return nil
}

// initNacosClient initializes Nacos client if not already initialized
func (l *ConfigLoader) initNacosClient() error {
	if l.nacosClient != nil {
		return nil
	}

	cfg := l.nacosConfig

	// Parse server address (host:port)
	serverConfigs := []constant.ServerConfig{
		{
			IpAddr: cfg.ServerAddr,
			Port:   8848, // default Nacos port
		},
	}

	// If serverAddr contains port, parse it
	if strings.Contains(cfg.ServerAddr, ":") {
		parts := strings.SplitN(cfg.ServerAddr, ":", 2)
		serverConfigs[0].IpAddr = parts[0]
		if port, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
			serverConfigs[0].Port = port
		}
	}

	clientConfig := constant.ClientConfig{
		NamespaceId:         cfg.Namespace,
		Username:            cfg.Username,
		Password:            cfg.Password,
		AccessKey:           cfg.AccessKey,
		SecretKey:           cfg.SecretKey,
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
		LogDir:              filepath.Join(os.TempDir(), "nacos", "log"),
		CacheDir:            filepath.Join(os.TempDir(), "nacos", "cache"),
		LogLevel:            "info",
	}

	client, err := clients.NewConfigClient(
		vo.NacosClientParam{
			ClientConfig:  &clientConfig,
			ServerConfigs: serverConfigs,
		},
	)
	if err != nil {
		return fmt.Errorf("config: create Nacos client: %w", err)
	}

	l.nacosClient = client
	return nil
}

// fetchNacosConfigs fetches all configurations from Nacos
func (l *ConfigLoader) fetchNacosConfigs() error {
	if l.nacosClient == nil {
		return nil
	}

	l.nacosConfigCache = make(map[string]interface{})

	for _, item := range l.nacosConfig.ConfigList {
		if item.Group == "" || item.DataID == "" {
			continue
		}

		content, err := l.nacosClient.GetConfig(vo.ConfigParam{
			DataId: item.DataID,
			Group:  item.Group,
		})
		if err != nil {
			continue
		}

		if content == "" {
			continue
		}

		parsedConfig := l.parseNacosConfig(content, item.DataID)
		if parsedConfig != nil {
			l.deepMerge(l.nacosConfigCache, parsedConfig)
		}
	}

	return nil
}

// parseNacosConfig parses Nacos configuration content based on data_id extension
func (l *ConfigLoader) parseNacosConfig(content, dataID string) map[string]interface{} {
	ext := strings.ToLower(filepath.Ext(dataID))

	switch ext {
	case ".json":
		return l.parseJSON(content, dataID)
	case ".yaml", ".yml":
		return l.parseYAML(content, dataID)
	default:
		// Fallback: try JSON, then YAML
		if result := l.parseJSON(content, dataID); result != nil {
			return result
		}
		return l.parseYAML(content, dataID)
	}
}

// parseJSON parses JSON content
func (l *ConfigLoader) parseJSON(content, dataID string) map[string]interface{} {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil
	}
	return result
}

// parseYAML parses YAML content
func (l *ConfigLoader) parseYAML(content, dataID string) map[string]interface{} {
	var result map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &result); err != nil {
		return nil
	}
	return result
}

// setupNacosListeners sets up Nacos configuration change listeners
func (l *ConfigLoader) setupNacosListeners() error {
	if l.nacosClient == nil {
		return nil
	}

	for _, item := range l.nacosConfig.ConfigList {
		if item.Group == "" || item.DataID == "" {
			continue
		}

		key := item.Group + ":" + item.DataID
		if _, exists := l.nacosListenerIDs[key]; exists {
			continue
		}

		param := vo.ConfigParam{
			DataId: item.DataID,
			Group:  item.Group,
			OnChange: func(namespace, group, dataId, data string) {
				l.mu.Lock()
				listening := l.nacosConfig != nil && l.nacosConfig.AutoUpdate
				l.mu.Unlock()

				if !listening {
					return
				}

				// Reload all config
				if err := l.Reload(); err != nil {
					return
				}

				// Call callbacks
				l.mu.RLock()
				configCopy := make(map[string]interface{})
				for k, v := range l.config {
					configCopy[k] = v
				}
				callbacks := make([]func(map[string]interface{}), len(l.nacosCallbacks))
				copy(callbacks, l.nacosCallbacks)
				l.mu.RUnlock()

				for _, cb := range callbacks {
					func() {
						defer func() { recover() }()
						cb(configCopy)
					}()
				}
			},
		}

		err := l.nacosClient.ListenConfig(param)
		if err != nil {
			continue
		}

		l.nacosListenerIDs[key] = key
	}

	return nil
}

// AddNacosListener adds a callback to be invoked when Nacos configuration changes
func (l *ConfigLoader) AddNacosListener(callback func(map[string]interface{})) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nacosCallbacks = append(l.nacosCallbacks, callback)
}

// RemoveNacosListener removes a Nacos configuration change callback
func (l *ConfigLoader) RemoveNacosListener(callback func(map[string]interface{})) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for i, cb := range l.nacosCallbacks {
		if fmt.Sprintf("%p", cb) == fmt.Sprintf("%p", callback) {
			l.nacosCallbacks = append(l.nacosCallbacks[:i], l.nacosCallbacks[i+1:]...)
			break
		}
	}
}

// saveSnapshot saves the current merged configuration to a snapshot file
func (l *ConfigLoader) saveSnapshot() error {
	if l.nacosConfig == nil || l.nacosConfig.SnapshotPath == "" {
		return nil
	}

	snapshot := map[string]interface{}{
		"snapshot_at": time.Now().Format(time.RFC3339),
		"sources": map[string]interface{}{
			"config_files": l.configPaths,
			"nacos": func() []map[string]string {
				var items []map[string]string
				for _, item := range l.nacosConfig.ConfigList {
					if item.Group != "" && item.DataID != "" {
						items = append(items, map[string]string{
							"group": item.Group, "data_id": item.DataID,
						})
					}
				}
				return items
			}(),
		},
		"config": l.config,
	}

	var content []byte
	var err error

	ext := strings.ToLower(filepath.Ext(l.nacosConfig.SnapshotPath))
	switch ext {
	case ".yaml", ".yml":
		content, err = yaml.Marshal(snapshot)
	default:
		content, err = json.MarshalIndent(snapshot, "", "  ")
	}

	if err != nil {
		return err
	}

	return os.WriteFile(l.nacosConfig.SnapshotPath, content, 0644)
}

// deepMerge deep merges maps
func (l *ConfigLoader) deepMerge(dest, src map[string]interface{}) {
	for k, v := range src {
		if existing, ok := dest[k]; ok {
			if destMap, ok := existing.(map[string]interface{}); ok {
				if srcMap, ok := v.(map[string]interface{}); ok {
					l.deepMerge(destMap, srcMap)
					continue
				}
			}
		}
		dest[k] = v
	}
}

// VAR_PATTERN ${VAR} or ${VAR:default}
var varPattern = regexp.MustCompile(`\$\{([^}:]+)(?::([^}]*))?\}`)

// substituteVariables variable substitution ${VAR}
func (l *ConfigLoader) substituteVariables() error {
	const maxPasses = 10
	for i := 0; i < maxPasses; i++ {
		changed := l.substituteRecursive(l.config)
		if !changed {
			return nil
		}
	}

	return fmt.Errorf("config: variable substitution may have circular references (max passes exceeded)")
}

// substituteRecursive recursively substitutes variables
func (l *ConfigLoader) substituteRecursive(data map[string]interface{}) bool {
	changed := false
	for k, v := range data {
		switch val := v.(type) {
		case map[string]interface{}:
			if l.substituteRecursive(val) {
				changed = true
			}
		case []interface{}:
			for i, item := range val {
				if s, ok := item.(string); ok {
					newVal := l.substituteString(s)
					if fmt.Sprintf("%v", newVal) != s {
						val[i] = newVal
						changed = true
					}
				}
			}
		case string:
			newVal := l.substituteString(val)
			if fmt.Sprintf("%v", newVal) != val {
				data[k] = newVal
				changed = true
			}
		}
	}
	return changed
}

// substituteString substitutes variables in a single string
func (l *ConfigLoader) substituteString(s string) interface{} {
	// Full match: ${VAR}
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") && strings.Count(s, "${") == 1 {
		parts := varPattern.FindStringSubmatch(s)
		if len(parts) >= 2 {
			key := parts[1]
			defaultVal := ""
			if len(parts) >= 3 {
				defaultVal = parts[2]
			}

			value := l.resolveValue(key, defaultVal)
			return l.convertType(value)
		}
	}

	// Partial match: abc${VAR}def
	return varPattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := varPattern.FindStringSubmatch(match)
		if len(parts) >= 2 {
			key := parts[1]
			defaultVal := ""
			if len(parts) >= 3 {
				defaultVal = parts[2]
			}

			value := l.resolveValue(key, defaultVal)
			return fmt.Sprintf("%v", l.convertType(value))
		}
		return match
	})
}

// resolveValue resolves variable value (config -> environment variable -> default)
// Returns nil when no value is found and no default is provided (matching Python behavior).
func (l *ConfigLoader) resolveValue(key string, defaultVal string) interface{} {
	// 1. Lookup from config
	configValue := l.navigatePath(l.config, key)
	if configValue != nil {
		return configValue
	}

	// 2. Lookup from environment variable
	envKey := strings.ReplaceAll(key, ".", "_")
	envKey = strings.ToUpper(envKey)

	if val := os.Getenv(envKey); val != "" {
		return val
	}
	if val := os.Getenv(key); val != "" {
		return val
	}

	// 3. Return default; nil if no default provided (matching Python: returns None for unresolved)
	if defaultVal == "" {
		return nil
	}
	return defaultVal
}

// convertType type conversion (string -> bool/int/float)
func (l *ConfigLoader) convertType(value interface{}) interface{} {
	s, ok := value.(string)
	if !ok {
		return value
	}

	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "true":
		return true
	case "false":
		return false
	case "null", "nil", "none":
		return nil
	}

	// Leading-zero guard: preserve strings like "01", "007" as-is (matching Python behavior)
	if len(s) > 1 && s[0] == '0' && s[1] >= '0' && s[1] <= '9' {
		return value
	}

	// Try to convert to integer
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}

	// Try to convert to float
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	// Keep as is
	return value
}

// navigatePath path navigation (dot notation)
func (l *ConfigLoader) navigatePath(data interface{}, path string) interface{} {
	if path == "" {
		return data
	}

	keys := strings.Split(path, ".")
	current := data
	for _, key := range keys {
		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[key]
			if !ok {
				return nil
			}
		default:
			return nil
		}
	}
	return current
}

// Get gets configuration value (supports dot notation)
func (l *ConfigLoader) Get(key string, defaultValue ...interface{}) interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	value := l.navigatePath(l.config, key)
	if value == nil && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return value
}

// GetString gets string
func (l *ConfigLoader) GetString(key string, defaultValue ...string) string {
	value := l.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return ""
	}
	return fmt.Sprintf("%v", value)
}

// GetInt gets integer
func (l *ConfigLoader) GetInt(key string, defaultValue ...int) int {
	value := l.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}

	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

// GetBool gets boolean
func (l *ConfigLoader) GetBool(key string, defaultValue ...bool) bool {
	value := l.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}

	switch v := value.(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		return s == "true" || s == "1"
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return false
}

// GetFloat gets float
func (l *ConfigLoader) GetFloat(key string, defaultValue ...float64) float64 {
	value := l.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0.0
	}

	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0.0
}

// GetStringSlice gets a string slice. Each element is converted to string via fmt.Sprintf.
// Supports both []interface{} (from YAML) and []string values.
func (l *ConfigLoader) GetStringSlice(key string, defaultValue ...[]string) []string {
	value := l.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return nil
	}

	switch v := value.(type) {
	case []interface{}:
		result := make([]string, len(v))
		for i, item := range v {
			result[i] = fmt.Sprintf("%v", item)
		}
		return result
	case []string:
		return v
	default:
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return nil
	}
}

// GetMap gets a map[string]interface{} value at the given key path.
func (l *ConfigLoader) GetMap(key string, defaultValue ...map[string]interface{}) map[string]interface{} {
	value := l.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return nil
	}

	if m, ok := value.(map[string]interface{}); ok {
		return m
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return nil
}

// GetDuration gets a time.Duration value. Supports Go duration strings (e.g. "5s", "100ms")
// and integer values interpreted as seconds.
func (l *ConfigLoader) GetDuration(key string, defaultValue ...time.Duration) time.Duration {
	value := l.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}

	switch v := value.(type) {
	case string:
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	case int:
		return time.Duration(v) * time.Second
	case int64:
		return time.Duration(v) * time.Second
	case float64:
		return time.Duration(v * float64(time.Second))
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

// Scan deserializes to struct (compatible with old method)
func (l *ConfigLoader) Scan(key string, v interface{}) error {
	l.mu.RLock()
	data := l.navigatePath(l.config, key)
	l.mu.RUnlock()

	if data == nil {
		return fmt.Errorf("config: key %q not found", key)
	}

	buf, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("config: marshal key %q: %w", key, err)
	}

	return yaml.Unmarshal(buf, v)
}

// Reload manually reloads configuration
func (l *ConfigLoader) Reload() error {
	return l.load()
}

// Watch watches file changes (hot reload)
func (l *ConfigLoader) Watch(callback func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	l.watcher = watcher

	for _, path := range l.configPaths {
		if _, err := os.Stat(path); err == nil {
			_ = watcher.Add(path)
		}
	}

	for _, path := range l.envFiles {
		if _, err := os.Stat(path); err == nil {
			_ = watcher.Add(path)
		}
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					time.Sleep(100 * time.Millisecond) // debounce
					if err := l.load(); err == nil && callback != nil {
						callback()
					}
				}

			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return nil
}

// Close closes file watcher and Nacos listeners
func (l *ConfigLoader) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Remove Nacos listeners
	if l.nacosClient != nil {
		for key, _ := range l.nacosListenerIDs {
			parts := strings.SplitN(key, ":", 2)
			if len(parts) == 2 {
				_ = l.nacosClient.CancelListenConfig(vo.ConfigParam{
					DataId: parts[1],
					Group:  parts[0],
				})
			}
		}
		l.nacosListenerIDs = make(map[string]string)
		l.nacosClient = nil
	}

	// Close file watcher
	if l.watcher != nil {
		return l.watcher.Close()
	}

	return nil
}

// Raw gets raw configuration (for debugging)
func (l *ConfigLoader) Raw() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make(map[string]interface{})
	for k, v := range l.config {
		result[k] = v
	}
	return result
}

// Dump returns all loaded configuration values and metadata, useful for debugging
func (l *ConfigLoader) Dump(redactKeys ...[]string) map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var envFiles []string
	if l.envFiles != nil {
		envFiles = make([]string, len(l.envFiles))
		copy(envFiles, l.envFiles)
	}

	var nacosItems []map[string]string
	if l.nacosConfig != nil {
		for _, item := range l.nacosConfig.ConfigList {
			if item.Group != "" && item.DataID != "" {
				nacosItems = append(nacosItems, map[string]string{
					"group": item.Group, "data_id": item.DataID,
				})
			}
		}
	}

	configSnapshot := make(map[string]interface{})
	for k, v := range l.config {
		configSnapshot[k] = v
	}

	if len(redactKeys) > 0 && len(redactKeys[0]) > 0 {
		redactKeyMap := make(map[string]bool)
		for _, k := range redactKeys[0] {
			redactKeyMap[strings.ToLower(k)] = true
		}
		redacted := l.redact(configSnapshot, redactKeyMap)
		var ok bool
		if configSnapshot, ok = redacted.(map[string]interface{}); !ok {
			configSnapshot = make(map[string]interface{})
		}
	}

	return map[string]interface{}{
		"sources": map[string]interface{}{
			"env_files":    envFiles,
			"config_files": l.configPaths,
			"nacos":        nacosItems,
		},
		"settings": map[string]interface{}{
			"enable_substitution": l.enableSubstitution,
			"override_env":        l.overrideEnv,
		},
		"config": configSnapshot,
	}
}

func (l *ConfigLoader) redact(data interface{}, keys map[string]bool) interface{} {
	switch d := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, v := range d {
			if keys[strings.ToLower(k)] {
				result[k] = "***"
			} else {
				result[k] = l.redact(v, keys)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(d))
		for i, v := range d {
			result[i] = l.redact(v, keys)
		}
		return result
	default:
		return data
	}
}
