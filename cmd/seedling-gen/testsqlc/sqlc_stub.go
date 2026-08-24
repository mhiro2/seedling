package testsqlc

import (
	"context"
	"time"
)

type DBTX any

type Queries struct {
	db DBTX
}

func New(db DBTX) *Queries {
	return &Queries{db: db}
}

type Recorder struct {
	NextID              int64
	InsertedCompanies   []Company
	InsertedUsers       []User
	InsertedMemberships []Membership
	DeletedCompanyIDs   []int64
	DeletedUserIDs      []int64
	DeletedMemberships  []DeleteMembershipParams
}

type Company struct {
	ID         int64
	SpotifyUrl string
}

type User struct {
	ID                int64
	DisplayLabel      string
	CreatedAt         time.Time
	CompanySpotifyUrl string
}

type InsertUserParams struct {
	DisplayLabel      string
	CreatedAt         time.Time
	CompanySpotifyUrl string
}

type Membership struct {
	OrganizationID int64
	UserID         int64
}

type InsertMembershipParams struct {
	OrganizationID int64
	UserID         int64
}

type DeleteMembershipParams struct {
	OrganizationID int64
	UserID         int64
}

func (q *Queries) InsertCompany(_ context.Context, spotifyUrl string) (Company, error) {
	recorder := q.db.(*Recorder)
	recorder.NextID++
	company := Company{ID: recorder.NextID, SpotifyUrl: spotifyUrl}
	recorder.InsertedCompanies = append(recorder.InsertedCompanies, company)
	return company, nil
}

func (q *Queries) InsertUser(_ context.Context, arg InsertUserParams) (User, error) {
	recorder := q.db.(*Recorder)
	recorder.NextID++
	user := User{
		ID:                recorder.NextID,
		DisplayLabel:      arg.DisplayLabel,
		CreatedAt:         arg.CreatedAt,
		CompanySpotifyUrl: arg.CompanySpotifyUrl,
	}
	recorder.InsertedUsers = append(recorder.InsertedUsers, user)
	return user, nil
}

func (q *Queries) InsertMembership(_ context.Context, arg InsertMembershipParams) (Membership, error) {
	membership := Membership(arg)
	recorder := q.db.(*Recorder)
	recorder.InsertedMemberships = append(recorder.InsertedMemberships, membership)
	return membership, nil
}

func (q *Queries) DeleteCompany(_ context.Context, id int64) error {
	recorder := q.db.(*Recorder)
	recorder.DeletedCompanyIDs = append(recorder.DeletedCompanyIDs, id)
	return nil
}

func (q *Queries) DeleteUser(_ context.Context, id int64) error {
	recorder := q.db.(*Recorder)
	recorder.DeletedUserIDs = append(recorder.DeletedUserIDs, id)
	return nil
}

func (q *Queries) DeleteMembership(_ context.Context, arg DeleteMembershipParams) error {
	recorder := q.db.(*Recorder)
	recorder.DeletedMemberships = append(recorder.DeletedMemberships, arg)
	return nil
}
