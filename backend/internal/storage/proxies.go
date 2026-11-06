package storage

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ProxyConfig 是跨渠道共享的代理；认证信息只保存在加密 URL 中。
type ProxyConfig struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	DisplayURL string    `gorm:"size:4096;not null" json:"display_url"`
	URLCipher  string    `gorm:"type:text;not null" json:"-"`
	URLHash    string    `gorm:"size:64;not null;uniqueIndex" json:"-"`
	CreatedAt  time.Time `json:"created_at"`
}

func (ProxyConfig) TableName() string { return "proxy_configs" }

type Proxies struct{ db *gorm.DB }

func NewProxies(db *gorm.DB) *Proxies { return &Proxies{db: db} }

func (r *Proxies) List() ([]ProxyConfig, error) {
	items := make([]ProxyConfig, 0)
	err := r.db.Order("id ASC").Find(&items).Error
	return items, err
}

func (r *Proxies) FindByID(id uint) (*ProxyConfig, error) {
	var item ProxyConfig
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Proxies) CreateOrFind(item *ProxyConfig) error {
	if err := r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "url_hash"}}, DoNothing: true}).Create(item).Error; err != nil {
		return err
	}
	return r.db.Where("url_hash = ?", item.URLHash).First(item).Error
}
