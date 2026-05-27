package model

import (
	dbmysql "github.com/huangyangke/go-aikit/database/mysql"
)

// Article represents the articles table.
type Article struct {
	dbmysql.Model
	Title   string `gorm:"size:255;not null" json:"title"`
	Content string `gorm:"type:text;not null" json:"content"`
	Author  string `gorm:"size:100;not null" json:"author"`
}

func (Article) TableName() string {
	return "articles"
}
