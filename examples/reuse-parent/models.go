package reuseparent

// Company represents a top-level organization.
type Company struct {
	ID   int
	Name string
}

// Project belongs to a Company.
type Project struct {
	ID        int
	CompanyID int
	Name      string
}

// Task belongs to a Project.
type Task struct {
	ID        int
	ProjectID int
	Title     string
}
