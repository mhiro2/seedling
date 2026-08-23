package testent

import (
	"context"
	"time"
)

type Client struct {
	Company *CompanyClient
}

type CompanyClient struct {
	InsertCount   int
	InsertedValue Company
	DeleteCount   int
	DeletedID     int64
}

func (c *CompanyClient) Create() *CompanyCreate {
	return &CompanyCreate{client: c}
}

func (c *CompanyClient) DeleteOneID(id int64) *CompanyDelete {
	return &CompanyDelete{client: c, id: id}
}

type CompanyCreate struct {
	client *CompanyClient
	value  Company
}

func (c *CompanyCreate) SetName(name string) *CompanyCreate {
	c.value.Name = name
	return c
}

func (c *CompanyCreate) SetCreatedAt(createdAt time.Time) *CompanyCreate {
	c.value.CreatedAt = createdAt
	return c
}

func (c *CompanyCreate) Save(context.Context) (*Company, error) {
	c.value.ID = 42
	c.client.InsertCount++
	c.client.InsertedValue = c.value
	inserted := c.value
	return &inserted, nil
}

type Company struct {
	ID        int64
	Name      string
	CreatedAt time.Time
}

type CompanyDelete struct {
	client *CompanyClient
	id     int64
}

func (d *CompanyDelete) Exec(context.Context) error {
	d.client.DeleteCount++
	d.client.DeletedID = d.id
	return nil
}
