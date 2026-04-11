package testmodels

import "time"

type Company struct {
	ID        uint
	Name      string
	CreatedAt time.Time
}

type User struct {
	ID        uint
	Name      string
	CreatedAt time.Time
	CompanyID uint
}

type Membership struct {
	CompanyID uint
	UserID    uint
}
