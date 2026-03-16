package basic

// Company represents a company record.
type Company struct {
	ID   int
	Name string
}

// User represents a user who belongs to a company.
type User struct {
	ID        int
	CompanyID int
	Name      string
	Email     string
}
