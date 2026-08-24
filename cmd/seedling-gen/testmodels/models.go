package testmodels

import "time"

type Company struct {
	ID        uint `gorm:"primaryKey"`
	Name      string
	CreatedAt time.Time
}

type User struct {
	ID        uint `gorm:"primaryKey"`
	Name      string
	CreatedAt time.Time
	CompanyID uint    `gorm:"not null"`
	Company   Company `gorm:"foreignKey:CompanyID"`
}

type Membership struct {
	CompanyID uint `gorm:"primaryKey"`
	UserID    uint `gorm:"primaryKey"`
}
