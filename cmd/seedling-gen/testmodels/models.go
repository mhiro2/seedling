package testmodels

type Company struct {
	ID   uint
	Name string
}

type User struct {
	ID        uint
	Name      string
	CompanyID uint
}
