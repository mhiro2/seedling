package testsqlc

import (
	"context"
	"time"
)

type DBTX any

type Queries struct{}

func New() *Queries {
	return &Queries{}
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
	RecordedAt        time.Time
	CompanySpotifyURL string
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

func (*Queries) InsertCompany(_ context.Context, db DBTX, spotifyUrl string) (*Company, error) {
	recorder := db.(*Recorder)
	recorder.NextID++
	company := Company{ID: recorder.NextID, SpotifyUrl: spotifyUrl}
	recorder.InsertedCompanies = append(recorder.InsertedCompanies, company)
	return &company, nil
}

func (*Queries) InsertUser(_ context.Context, db DBTX, arg *InsertUserParams) (*User, error) {
	recorder := db.(*Recorder)
	recorder.NextID++
	user := User{
		ID:                recorder.NextID,
		DisplayLabel:      arg.DisplayLabel,
		CreatedAt:         arg.RecordedAt,
		CompanySpotifyUrl: arg.CompanySpotifyURL,
	}
	recorder.InsertedUsers = append(recorder.InsertedUsers, user)
	return &user, nil
}

func (*Queries) InsertMembership(_ context.Context, db DBTX, arg *InsertMembershipParams) (*Membership, error) {
	membership := Membership(*arg)
	recorder := db.(*Recorder)
	recorder.InsertedMemberships = append(recorder.InsertedMemberships, membership)
	return &membership, nil
}

func (*Queries) DeleteCompany(_ context.Context, db DBTX, id int64) error {
	recorder := db.(*Recorder)
	recorder.DeletedCompanyIDs = append(recorder.DeletedCompanyIDs, id)
	return nil
}

func (*Queries) DeleteUser(_ context.Context, db DBTX, id int64) error {
	recorder := db.(*Recorder)
	recorder.DeletedUserIDs = append(recorder.DeletedUserIDs, id)
	return nil
}

func (*Queries) DeleteMembership(_ context.Context, db DBTX, arg *DeleteMembershipParams) error {
	recorder := db.(*Recorder)
	recorder.DeletedMemberships = append(recorder.DeletedMemberships, *arg)
	return nil
}
