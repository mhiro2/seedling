package testsqlc

import "context"

type DBTX any

type Queries struct{}

func New(DBTX) *Queries {
	return &Queries{}
}

type User struct {
	ID        int64
	Name      string
	CompanyID int64
}

type InsertUserParams struct {
	Name      string
	CompanyID int64
}

func (*Queries) InsertUser(context.Context, InsertUserParams) (User, error) {
	return User{}, nil
}

func (*Queries) DeleteUser(context.Context, int64) error {
	return nil
}
