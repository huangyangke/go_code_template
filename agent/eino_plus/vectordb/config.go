// Package vectordb 腾讯云向量数据库客户端封装.
package vectordb

import (
	"errors"
)

// SearchType 搜索类型.
type SearchType string

const (
	// SearchTypeVector 纯向量搜索.
	SearchTypeVector SearchType = "vector"
	// SearchTypeHybrid 混合检索 (dense + sparse).
	SearchTypeHybrid SearchType = "hybrid"
)

// MetricType 向量距离度量类型.
type MetricType string

const (
	// MetricCosine 余弦相似度.
	MetricCosine MetricType = "COSINE"
	// MetricL2 欧氏距离.
	MetricL2 MetricType = "L2"
	// MetricIP 内积.
	MetricIP MetricType = "IP"
)

// Config 向量数据库客户端配置，字段规范与 Python tencentdb.py 完全一致.
type Config struct {
	URL            string     `yaml:"url" json:"url"`
	Key            string     `yaml:"key" json:"key"`
	Username       string     `yaml:"username" json:"username"`
	DBName         string     `yaml:"db_name" json:"db_name"`
	CollectionName string     `yaml:"collection_name" json:"collection_name"`
	Dimensions     int        `yaml:"dimensions" json:"dimensions"`
	SearchType     SearchType `yaml:"search_type" json:"search_type"`
	Metric         MetricType `yaml:"metric" json:"metric"`
	Timeout        int        `yaml:"timeout" json:"timeout"`

	// PartitionFields 参与 doc ID hash 的 metadata key 列表
	PartitionFields []string `yaml:"partition_fields" json:"partition_fields"`

	// HybridVectorWeight 混合检索中 vector 的权重 (0-1)，sparse = 1 - weight
	HybridVectorWeight float64 `yaml:"hybrid_vector_weight" json:"hybrid_vector_weight"`
}

// Fix 填充配置默认值.
func (c *Config) Fix() {
	if c.Username == "" {
		c.Username = "root"
	}
	if c.SearchType == "" {
		c.SearchType = SearchTypeVector
	}
	if c.Metric == "" {
		c.Metric = MetricIP
	}
	if c.Timeout <= 0 {
		c.Timeout = 30
	}
	if c.HybridVectorWeight <= 0 || c.HybridVectorWeight > 1 {
		c.HybridVectorWeight = 0.7
	}
}

// Validate 校验配置必要字段.
// 返回值：err - 校验失败时的错误.
func (c *Config) Validate() error {
	if c.URL == "" {
		return errors.New("vectordb: url is required")
	}
	if c.Key == "" {
		return errors.New("vectordb: key is required")
	}
	if c.DBName == "" {
		return errors.New("vectordb: db_name is required")
	}
	if c.CollectionName == "" {
		return errors.New("vectordb: collection_name is required")
	}
	if c.Dimensions <= 0 {
		return errors.New("vectordb: dimensions must be positive")
	}
	return nil
}
