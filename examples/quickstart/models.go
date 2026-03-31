package quickstart

type Company struct {
	ID   int
	Name string
}

type User struct {
	ID        int
	CompanyID int
	Name      string
	Email     string
}
