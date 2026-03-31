package batchinsert

type Company struct {
	ID   int
	Name string
}

type Project struct {
	ID        int
	CompanyID int
	Name      string
}

type Task struct {
	ID        int
	ProjectID int
	Title     string
}
