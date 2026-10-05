// Package models defines the GORM-backed domain entities.
package models

import "time"

// URL is the persisted representation of a short-code -> long-URL mapping.
//
// The Code column is unique and indexed so lookups on GET /:code are fast and
// collisions are rejected by the database. LongURL is also indexed to make the
// dedup query (does this long URL already have a code?) efficient.
type URL struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	Code      string    `gorm:"uniqueIndex;size:32;not null" json:"code"`
	LongURL   string    `gorm:"index;size:2048;not null" json:"long_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName overrides the default GORM table name for clarity.
func (URL) TableName() string {
	return "urls"
}
