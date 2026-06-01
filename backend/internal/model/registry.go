package model

// All 包含所有需要 AutoMigrate 的 GORM model，仅供 SQLite 本地开发使用.
// 新增表时在此追加对应 model 指针.
var All = []any{
	&Article{},
}
