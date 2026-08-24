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
	ID   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
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

func (c *UserCreate) SetOIDCURL(oidcURL string) *UserCreate {
	c.value.OIDCURL = oidcURL
	return c
}

func (c *UserCreate) SetCreatedAt(createdAt int64) *UserCreate {
	c.value.CreatedAt = createdAt
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
	ID          int     `json:"id,omitempty"`
	Name        string  `json:"name,omitempty"`
	Nickname    *string `json:"nickname,omitempty"`
	CompanyUUID *int    `json:"company_uuid,omitempty"`
	OIDCURL     string  `json:"oidc_url,omitempty"`
	CreatedAt   int64   `json:"created_at,omitempty"`
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
