package testent

import "context"

type Client struct {
	Company *CompanyClient
}

type CompanyClient struct{}

func (CompanyClient) Create() *CompanyCreate {
	return &CompanyCreate{}
}

func (CompanyClient) DeleteOneID(int64) *CompanyDelete {
	return &CompanyDelete{}
}

type CompanyCreate struct{}

func (CompanyCreate) Save(context.Context) (*Company, error) {
	return &Company{}, nil
}

type Company struct {
	ID int64
}

type CompanyDelete struct{}

func (CompanyDelete) Exec(context.Context) error {
	return nil
}
