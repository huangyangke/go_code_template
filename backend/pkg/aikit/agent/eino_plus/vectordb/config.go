package vectordb

import (
	"errors"
)

type SearchType string

const (
	SearchTypeVector SearchType = "vector"
	SearchTypeHybrid SearchType = "hybrid"
)

type MetricType string

const (
	MetricCosine MetricType = "COSINE"
	MetricL2     MetricType = "L2"
	MetricIP     MetricType = "IP"
)

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
