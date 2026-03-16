package seedlingtest

type Company struct {
	ID   int
	Name string
}

type User struct {
	ID        int
	CompanyID int
	Name      string
}

type Project struct {
	ID        int
	CompanyID int
	Name      string
}

type Task struct {
	ID             int
	ProjectID      int
	AssigneeUserID int
	Title          string
	Status         string
}

type Department struct {
	ID   int
	Name string
}

type Employee struct {
	ID           int
	DepartmentID int
	Name         string
}

type Region struct {
	Code   string
	Number int
	Name   string
}

type Deployment struct {
	ID           int
	RegionCode   string
	RegionNumber int
	Name         string
}

type Article struct {
	ID    int
	Title string
}

type Tag struct {
	ID   int
	Name string
}

type ArticleTag struct {
	ArticleID int
	TagID     int
}

type PtrModel struct {
	ID       int
	Name     *string
	Optional *int
}

type InterfaceModel struct {
	ID       int
	Name     string
	Metadata any
}
