package testent

import (
	"context"
)

type Client struct {
	Company *CompanyClient
	User    *UserClient
}

type CompanyClient struct {
	InsertCount   int
	InsertedValue Company
	DeleteCount   int
	DeletedID     int
}

func (c *CompanyClient) Create() *CompanyCreate {
	return &CompanyCreate{client: c}
}

func (c *CompanyClient) DeleteOneID(id int) *CompanyDelete {
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

func (c *CompanyCreate) Save(context.Context) (*Company, error) {
	c.value.ID = 42
	c.client.InsertCount++
	c.client.InsertedValue = c.value
	inserted := c.value
	return &inserted, nil
}

type Company struct {
	ID   int
	Name string
}

type CompanyDelete struct {
	client *CompanyClient
	id     int
}

func (d *CompanyDelete) Exec(context.Context) error {
	d.client.DeleteCount++
	d.client.DeletedID = d.id
	return nil
}

type UserClient struct {
	InsertCount   int
	InsertedValue User
	DeleteCount   int
	DeletedID     int
}

func (c *UserClient) Create() *UserCreate {
	return &UserCreate{client: c}
}

func (c *UserClient) DeleteOneID(id int) *UserDelete {
	return &UserDelete{client: c, id: id}
}

type UserCreate struct {
	client *UserClient
	value  User
}

func (c *UserCreate) SetName(name string) *UserCreate {
	c.value.Name = name
	return c
}

func (c *UserCreate) SetNillableNickname(nickname *string) *UserCreate {
	c.value.Nickname = nickname
	return c
}

func (c *UserCreate) SetNillableCompanyUUID(companyUUID *int) *UserCreate {
	c.value.CompanyUUID = companyUUID
	return c
}

func (c *UserCreate) Save(context.Context) (*User, error) {
	c.value.ID = 84
	c.client.InsertCount++
	c.client.InsertedValue = c.value
	inserted := c.value
	return &inserted, nil
}

type User struct {
	ID          int
	Name        string
	Nickname    *string
	CompanyUUID *int
}

type UserDelete struct {
	client *UserClient
	id     int
}

func (d *UserDelete) Exec(context.Context) error {
	d.client.DeleteCount++
	d.client.DeletedID = d.id
	return nil
}
