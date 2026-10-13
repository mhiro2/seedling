// Package handwritten holds GORM models written by hand in the conventional
// style, as opposed to the gorm/gen output in the sibling model package.
package handwritten

import "gorm.io/gorm"

// Team embeds gorm.Model, so its key and timestamps are promoted fields.
type Team struct {
	gorm.Model
	Name string `gorm:"not null"`
}

// Player relies on GORM treating the untagged ID field as its primary key.
type Player struct {
	ID     uint
	Name   string
	TeamID uint `gorm:"not null"`
	Team   Team
}
