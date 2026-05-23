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

// NacosConfig 描述 Nacos 配置中心接入参数。
// 它定义的是“如何连接 Nacos、拉哪些远程配置、是否自动更新”等元信息。
type NacosConfig struct {
	// ServerAddr 支持 "host" 或 "host:port"；未显式指定端口时默认 8848。
	ServerAddr string `yaml:"server_addr"`
	// Namespace 用于隔离不同环境或项目下的配置空间。
	Namespace string `yaml:"namespace"`
	// Username / Password 为 Nacos 登录凭证，按需使用。
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// AccessKey / SecretKey 预留给鉴权场景。
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	// ConfigList 指定需要拉取的 dataId/group 列表。
	ConfigList []NacosConfigItem `yaml:"config_list"`
	// AutoUpdate 为 true 时，会注册远程监听器并在变更后自动 Reload。
	AutoUpdate bool `yaml:"auto_update"`
	// SnapshotPath 非空时，会把合并后的最终配置写到本地快照文件。
	SnapshotPath string `yaml:"snapshot_path"`
}

// NacosConfigItem 表示一条具体的 Nacos 配置项。
type NacosConfigItem struct {
	Group  string `yaml:"group"`
	DataID string `yaml:"data_id"`
}

// ConfigLoader 是整个配置模块的核心加载器。
//
// 它会把多个来源的配置统一合并成一个最终配置视图，支持：
// 1. 多个本地 JSON/YAML 文件按顺序合并；
// 2. .env 文件加载到进程环境变量；
// 3. ${VAR} / ${VAR:default} 变量替换；
// 4. Nacos 远程配置接入与自动更新；
// 5. 本地文件热重载、结构体映射、调试导出等能力。
type ConfigLoader struct {
	// mu 保护 config、本地 watcher 与 Nacos 相关状态，避免并发读写冲突。
	mu sync.RWMutex
	// configPaths 是本地配置文件路径列表；后加载的文件会覆盖前者。
	configPaths []string
	// envFiles 是需要导入的 .env 文件列表，默认只有 ".env"。
	envFiles []string
	// overrideEnv 控制 .env 文件是否覆盖当前进程已有环境变量。
	overrideEnv bool
	// enableSubstitution 控制是否启用 ${...} 变量替换。
	enableSubstitution bool
	// config 保存已经合并完成的最终配置树。
	config map[string]interface{}
	// watcher 用于监听本地配置文件变化，实现热重载。
	watcher *fsnotify.Watcher

	// nacosConfig 为 nil 表示未启用 Nacos。
	nacosConfig *NacosConfig
	// nacosClient 按需初始化，只有访问 Nacos 时才真正创建。
	nacosClient config_client.IConfigClient
	// nacosConfigCache 保存当前从 Nacos 拉取并解析后的配置结果。
	nacosConfigCache map[string]interface{}
	// nacosCallbacks 会在远程配置变化并成功 Reload 后依次触发。
	nacosCallbacks []func(map[string]interface{})
	// nacosListenerIDs 记录已经注册过的监听，避免重复监听同一项配置。
	nacosListenerIDs map[string]string // group+dataId -> listenerId
}

// Option 是 ConfigLoader 的可选项注入函数。
type Option func(*ConfigLoader)

// WithEnvFile 指定单个 .env 文件，并替换默认的 ".env"。
func WithEnvFile(path string) Option {
	return func(l *ConfigLoader) {
		l.envFiles = []string{path}
	}
}

// WithEnvFiles 指定多个 .env 文件，并替换默认的 ".env"。
// 加载顺序与 paths 顺序一致。
func WithEnvFiles(paths []string) Option {
	return func(l *ConfigLoader) {
		l.envFiles = paths
	}
}

// WithOverrideEnv 控制 .env 文件是否覆盖当前进程已有环境变量。
func WithOverrideEnv(override bool) Option {
	return func(l *ConfigLoader) {
		l.overrideEnv = override
	}
}

// WithEnableSubstitution 控制是否开启变量替换。
func WithEnableSubstitution(enable bool) Option {
	return func(l *ConfigLoader) {
		l.enableSubstitution = enable
	}
}

// WithNacosConfig 启用 Nacos 配置中心能力。
func WithNacosConfig(cfg *NacosConfig) Option {
	return func(l *ConfigLoader) {
		l.nacosConfig = cfg
	}
}

// New 使用单个配置文件创建 ConfigLoader。
// 它只是对 NewFromPaths 的便捷封装。
func New(configPath string, opts ...Option) (*ConfigLoader, error) {
	return NewFromPaths([]string{configPath}, opts...)
}

// NewFromPaths 使用多个本地配置文件创建加载器。
// 构造成功时会立即完成一次 load()，因此返回后的配置即可直接读取。
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

// load 按固定顺序重新构建最终配置。
//
// 当前顺序为：
// 1. 加载 .env 文件；
// 2. 加载本地配置文件；
// 3. 加载并覆盖 Nacos 配置；
// 4. 对最终配置树执行变量替换。
//
// 因此最终来源优先级为：Nacos > 本地配置文件；
// 而变量替换优先级为：config > env > default。
func (l *ConfigLoader) load() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.config = make(map[string]interface{})

	// 先导入 .env，给后续变量替换提供环境变量来源。
	if err := l.loadEnvFiles(); err != nil {
		return err
	}

	// 再加载本地配置文件，后者覆盖前者。
	if err := l.loadLocalConfigs(); err != nil {
		return err
	}

	// 如启用 Nacos，则远程配置会覆盖本地同名字段。
	if l.nacosConfig != nil {
		if err := l.loadNacosConfig(); err != nil {
			return err
		}
	}

	// 最后对合并后的最终配置做变量替换。
	if l.enableSubstitution {
		if err := l.substituteVariables(); err != nil {
			return err
		}
	}

	return nil
}

// loadEnvFiles 依次读取并导入 .env 文件。
// 这里不会直接写入 l.config，而是写入进程环境变量。
func (l *ConfigLoader) loadEnvFiles() error {
	for _, path := range l.envFiles {
		if _, err := os.Stat(path); err != nil {
			continue // 文件不存在时跳过，便于不同环境共用相同加载逻辑。
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

			// 移除包裹值的单双引号，兼容常见 .env 写法。
			val = strings.Trim(val, `"`)
			val = strings.Trim(val, `'`)

			if l.overrideEnv || os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}

	return nil
}

// loadLocalConfigs 读取本地 JSON/YAML 文件并按顺序深度合并。
// 对于未知后缀的文件，会直接跳过。
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

// loadNacosConfig 拉取 Nacos 远程配置并合并到当前配置树。
// 这一步发生在本地配置之后，因此远程配置优先级更高。
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

	// 远程配置覆盖本地同名字段。
	l.deepMerge(l.config, l.nacosConfigCache)

	// 快照属于辅助能力，失败时不阻断主流程。
	if err := l.saveSnapshot(); err != nil {
		// 不因为快照失败影响加载结果。
	}

	// 开启自动更新时，为每个配置项注册远程监听器。
	if l.nacosConfig.AutoUpdate {
		if err := l.setupNacosListeners(); err != nil {
			// 不因为监听注册失败而影响当前已加载好的配置。
		}
	}

	return nil
}

// initNacosClient 初始化 Nacos 客户端。
// 该方法采用惰性初始化，避免未使用 Nacos 时创建额外连接。
func (l *ConfigLoader) initNacosClient() error {
	if l.nacosClient != nil {
		return nil
	}

	cfg := l.nacosConfig

	// 先按默认端口构造，再根据 "host:port" 形式覆盖。
	serverConfigs := []constant.ServerConfig{
		{
			IpAddr: cfg.ServerAddr,
			Port:   8848, // 默认 Nacos 端口
		},
	}

	// 如果地址里显式带端口，则优先使用显式端口。
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

// fetchNacosConfigs 逐项从 Nacos 拉取配置并合并到 nacosConfigCache。
// 单个 dataId 拉取失败时会跳过，不中断整个加载过程。
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

// parseNacosConfig 根据 dataId 后缀推断远程内容格式。
// 未知后缀时会按 JSON -> YAML 的顺序依次兜底尝试。
func (l *ConfigLoader) parseNacosConfig(content, dataID string) map[string]interface{} {
	ext := strings.ToLower(filepath.Ext(dataID))

	switch ext {
	case ".json":
		return l.parseJSON(content, dataID)
	case ".yaml", ".yml":
		return l.parseYAML(content, dataID)
	default:
		// 未知后缀时尽量容错解析，减少命名不规范带来的影响。
		if result := l.parseJSON(content, dataID); result != nil {
			return result
		}
		return l.parseYAML(content, dataID)
	}
}

// parseJSON 解析 JSON 文本，失败时返回 nil。
func (l *ConfigLoader) parseJSON(content, dataID string) map[string]interface{} {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil
	}
	return result
}

// parseYAML 解析 YAML 文本，失败时返回 nil。
func (l *ConfigLoader) parseYAML(content, dataID string) map[string]interface{} {
	var result map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &result); err != nil {
		return nil
	}
	return result
}

// setupNacosListeners 为所有远程配置项注册变更监听器。
// 当 Nacos 配置变化时，会执行完整 Reload，然后触发注册过的回调。
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

				// 远程变更后走完整 Reload，保证与初始化流程一致。
				if err := l.Reload(); err != nil {
					return
				}

				// 先复制当前配置与回调列表，避免在持锁状态下执行用户回调。
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

// AddNacosListener 注册 Nacos 变更后的回调函数。
func (l *ConfigLoader) AddNacosListener(callback func(map[string]interface{})) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nacosCallbacks = append(l.nacosCallbacks, callback)
}

// RemoveNacosListener 移除已注册的 Nacos 回调。
// 这里通过函数地址比较，因此需要传入同一个函数引用。
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

// saveSnapshot 把当前最终配置保存到本地快照文件。
// 这个快照主要用于调试、排障或保留最终合并结果。
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

// deepMerge 递归合并两个 map。
// 当同名 key 的值都是 map 时继续向下合并，否则用 src 覆盖 dest。
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

// varPattern 匹配两种占位符：
// 1. ${VAR}
// 2. ${VAR:default}
var varPattern = regexp.MustCompile(`\$\{([^}:]+)(?::([^}]*))?\}`)

// substituteVariables 对整个配置树执行变量替换。
// 由于一个字段可能间接引用另一个字段，因此这里允许多轮替换；
// 为防止循环引用导致死循环，最多执行 maxPasses 轮。
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

// substituteRecursive 递归遍历 map，处理其中的字符串、数组和嵌套 map。
func (l *ConfigLoader) substituteRecursive(data map[string]interface{}) bool {
	changed := false
	for k, v := range data {
		switch val := v.(type) {
		case map[string]interface{}:
			if l.substituteRecursive(val) {
				changed = true
			}
		case []interface{}:
			// 数组中的元素也可能继续包含 map、数组或字符串占位符。
			for i, item := range val {
				switch itemVal := item.(type) {
				case map[string]interface{}:
					if l.substituteRecursive(itemVal) {
						changed = true
					}
				case []interface{}:
					if l.substituteSlice(itemVal) {
						changed = true
					}
				case string:
					newVal := l.substituteString(itemVal)
					if fmt.Sprintf("%v", newVal) != itemVal {
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

// substituteSlice 递归处理数组中的占位符。
// 单独拆函数是为了复用“数组里套数组”的处理逻辑。
func (l *ConfigLoader) substituteSlice(data []interface{}) bool {
	changed := false
	for i, item := range data {
		switch val := item.(type) {
		case map[string]interface{}:
			if l.substituteRecursive(val) {
				changed = true
			}
		case []interface{}:
			if l.substituteSlice(val) {
				changed = true
			}
		case string:
			newVal := l.substituteString(val)
			if fmt.Sprintf("%v", newVal) != val {
				data[i] = newVal
				changed = true
			}
		}
	}
	return changed
}

// substituteString 处理单个字符串中的变量引用。
// 如果整串就是一个占位符，则尽量保留其转换后的真实类型；
// 如果占位符只是字符串的一部分，则最终返回字符串。
func (l *ConfigLoader) substituteString(s string) interface{} {
	// 完整占位符匹配时，可以把结果转成 bool/int/float 等更具体类型。
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

	// 部分替换场景只能返回字符串，例如 "http://${HOST}:${PORT}"。
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

// resolveValue 按固定优先级解析占位符的值：config -> environment variable -> default。
// 当没有找到值且没有默认值时，返回 nil。
func (l *ConfigLoader) resolveValue(key string, defaultVal string) interface{} {
	// 1. 先从当前配置树中读取，支持 ${database.host} 这种跨字段引用。
	configValue := l.navigatePath(l.config, key)
	if configValue != nil {
		return configValue
	}

	// 2. 再读取环境变量，同时兼容 APP_PORT / app.port 两种风格。
	envKey := strings.ReplaceAll(key, ".", "_")
	envKey = strings.ToUpper(envKey)

	if val := os.Getenv(envKey); val != "" {
		return val
	}
	if val := os.Getenv(key); val != "" {
		return val
	}

	// 3. 最后使用默认值；没有默认值时返回 nil。
	if defaultVal == "" {
		return nil
	}
	return defaultVal
}

// convertType 尝试把字符串值转换为更具体的类型。
// 这样 ${APP_PORT} 在值为 "8080" 时，最终可表现为 int 而不是 string。
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

	// 保留前导 0 字符串，避免像编码/编号这类值被错误转成数字。
	if len(s) > 1 && s[0] == '0' && s[1] >= '0' && s[1] <= '9' {
		return value
	}

	// 优先尝试整型，避免 "1" 被转成 1.0。
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}

	// 整型失败后再尝试浮点。
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	// 都无法转换时，保持原始字符串。
	return value
}

// navigatePath 按点路径访问配置树，例如 "app.port"。
// 当 path 为空字符串时，直接返回整个 data。
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

// Get 是最基础的取值方法，支持点路径。
// 它返回 interface{}，适合通用场景；若已知类型，通常更推荐使用 GetXxx。
func (l *ConfigLoader) Get(key string, defaultValue ...interface{}) interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	value := l.navigatePath(l.config, key)
	if value == nil && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return value
}

// GetString 获取字符串值。
// 若底层不是字符串，也会通过 fmt.Sprintf 做字符串化处理。
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

// GetInt 获取整型值，支持 int / int64 / float64 / string 等常见输入。
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

// GetBool 获取布尔值，支持 bool 与字符串形式的 "true" / "1"。
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

// GetFloat 获取浮点值，支持 float64 / int / int64 / string。
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

// GetStringSlice 获取字符串切片。
// YAML 常见的 []interface{} 会逐项转成字符串，便于上层直接使用。
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

// GetMap 获取 map 值。
// 返回的是深拷贝，调用方修改结果不会污染 ConfigLoader 内部状态。
func (l *ConfigLoader) GetMap(key string, defaultValue ...map[string]interface{}) map[string]interface{} {
	value := l.Get(key)
	if value == nil {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return nil
	}

	if m, ok := value.(map[string]interface{}); ok {
		if copied, ok := l.deepCopyValue(m).(map[string]interface{}); ok {
			return copied
		}
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return nil
}

// GetDuration 获取 time.Duration。
// 支持 Go duration 字符串（如 "5s"、"100ms"），也支持把数字按秒解释。
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

// Scan 把某个配置节点反序列化到结构体。
// 它适合一次性读取一整段配置，而不是逐个字段调用 GetXxx。
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

// Reload 手动触发一次完整重载。
// 它走的就是与初始化时相同的 load() 流程。
func (l *ConfigLoader) Reload() error {
	return l.load()
}

// Watch 监听本地配置文件和 .env 文件的变化，并在变化后自动 Reload。
//
// 这里监听的是“目标文件所在目录”，不是只监听文件本身。
// 这样可以兼容很多编辑器的原子保存行为：先写临时文件，再 rename 覆盖原文件。
func (l *ConfigLoader) Watch(callback func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	l.watcher = watcher

	// watchedFiles 表示真正关心的目标文件集合。
	watchedFiles := make(map[string]struct{})
	// watchedDirs 用于避免重复监听同一个目录。
	watchedDirs := make(map[string]struct{})

	// 对配置文件：记录目标文件，并监听它所在目录。
	for _, path := range l.configPaths {
		cleanPath := filepath.Clean(path)
		if absPath, err := filepath.Abs(cleanPath); err == nil {
			watchedFiles[absPath] = struct{}{}
		}
		dir := filepath.Dir(cleanPath)
		if absDir, err := filepath.Abs(dir); err == nil {
			if _, exists := watchedDirs[absDir]; !exists {
				if _, err := os.Stat(absDir); err == nil {
					_ = watcher.Add(absDir)
					watchedDirs[absDir] = struct{}{}
				}
			}
		}
	}

	// 对 .env 文件：也采用“记录目标文件 + 监听目录”的方式。
	for _, path := range l.envFiles {
		cleanPath := filepath.Clean(path)
		if absPath, err := filepath.Abs(cleanPath); err == nil {
			watchedFiles[absPath] = struct{}{}
		}
		dir := filepath.Dir(cleanPath)
		if absDir, err := filepath.Abs(dir); err == nil {
			if _, exists := watchedDirs[absDir]; !exists {
				if _, err := os.Stat(absDir); err == nil {
					_ = watcher.Add(absDir)
					watchedDirs[absDir] = struct{}{}
				}
			}
		}
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				eventPath, err := filepath.Abs(filepath.Clean(event.Name))
				if err != nil {
					continue
				}
				// 只有目标文件本身的事件才触发 reload，目录下其他文件会被忽略。
				if _, exists := watchedFiles[eventPath]; !exists {
					continue
				}

				// Write / Create / Rename 都可能意味着配置内容实际发生了变化。
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
					time.Sleep(100 * time.Millisecond) // 简单防抖，避免一次保存触发多次 reload。
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

// Close 释放 ConfigLoader 持有的外部资源。
// 包括本地文件 watcher 和已注册的 Nacos 监听器。
func (l *ConfigLoader) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 先撤销远程监听，避免关闭后仍收到 Nacos 回调。
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

	// 最后关闭本地文件 watcher。
	if l.watcher != nil {
		return l.watcher.Close()
	}

	return nil
}

// Raw 返回当前最终配置的深拷贝。
// 调用方可以安全读取；即使修改返回值，也不会污染内部状态。
func (l *ConfigLoader) Raw() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if copied, ok := l.deepCopyValue(l.config).(map[string]interface{}); ok {
		return copied
	}
	return make(map[string]interface{})
}

// Dump 导出完整的调试视图。
// 除了最终配置本身，还会带上来源文件、Nacos 条目和当前开关设置等元数据。
// redactKeys 可用于把 password / secret / token 等敏感字段统一脱敏。
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

	// 对配置内容做快照，保证导出结果和内部状态相互隔离。
	configSnapshot := make(map[string]interface{})
	if copied, ok := l.deepCopyValue(l.config).(map[string]interface{}); ok {
		configSnapshot = copied
	}

	// 根据传入的敏感 key 名做递归脱敏。
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

// redact 递归遍历 map / slice，并把命中的敏感 key 替换成 "***"。
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

// deepCopyValue 递归深拷贝 map / slice。
// 对外暴露配置时统一走深拷贝，避免共享引用导致外部误改内部配置。
func (l *ConfigLoader) deepCopyValue(data interface{}) interface{} {
	switch d := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(d))
		for k, v := range d {
			result[k] = l.deepCopyValue(v)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(d))
		for i, v := range d {
			result[i] = l.deepCopyValue(v)
		}
		return result
	case []string:
		result := make([]string, len(d))
		copy(result, d)
		return result
	default:
		return data
	}
}
