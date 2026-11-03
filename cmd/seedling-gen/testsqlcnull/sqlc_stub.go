// Package testsqlcnull mimics sqlc output for nullable foreign keys, which
// sqlc's database/sql driver maps to sql.Null* types.
package testsqlcnull

import (
	"context"
	"database/sql"
)

type DBTX any

type Queries struct{}

func New() *Queries {
	return &Queries{}
}

type Recorder struct {
	NextID int64
}

type Team struct {
	ID   int64
	Name string
}

type Player struct {
	ID     int64
	TeamID sql.NullInt64
}

type Manager struct {
	ID     int64
	TeamID sql.Null[int64]
}

type InsertPlayerParams struct {
	TeamID sql.NullInt64
}

type InsertManagerParams struct {
	TeamID sql.Null[int64]
}

func (*Queries) InsertTeam(_ context.Context, db DBTX, name string) (Team, error) {
	recorder := db.(*Recorder)
	recorder.NextID++
	return Team{ID: recorder.NextID, Name: name}, nil
}

func (*Queries) InsertPlayer(_ context.Context, db DBTX, arg InsertPlayerParams) (Player, error) {
	recorder := db.(*Recorder)
	recorder.NextID++
	return Player{ID: recorder.NextID, TeamID: arg.TeamID}, nil
}

func (*Queries) InsertManager(_ context.Context, db DBTX, arg InsertManagerParams) (Manager, error) {
	recorder := db.(*Recorder)
	recorder.NextID++
	return Manager{ID: recorder.NextID, TeamID: arg.TeamID}, nil
}
