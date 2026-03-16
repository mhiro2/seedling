package sqlc

// Organization is a model struct as sqlc would generate it.
type Organization struct {
	ID   int64
	Name string
}

// Member is a model struct as sqlc would generate it.
type Member struct {
	ID             int64
	OrganizationID int64
	Name           string
	Email          string
}
