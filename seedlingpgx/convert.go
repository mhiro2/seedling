package seedlingpgx

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mhiro2/seedling"
)

// Int8FK converts an int64 parent key into a pgtype.Int8 FK field, the type
// sqlc's pgx/v5 driver uses for a nullable bigint column.
//
//	Convert: map[string]seedling.FKConverter{"ManagerID": seedlingpgx.Int8FK()}
func Int8FK() seedling.FKConverter {
	return seedling.ConvertFK(func(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} })
}

// Int4FK converts an int32 parent key into a pgtype.Int4 FK field.
func Int4FK() seedling.FKConverter {
	return seedling.ConvertFK(func(v int32) pgtype.Int4 { return pgtype.Int4{Int32: v, Valid: true} })
}

// Int2FK converts an int16 parent key into a pgtype.Int2 FK field.
func Int2FK() seedling.FKConverter {
	return seedling.ConvertFK(func(v int16) pgtype.Int2 { return pgtype.Int2{Int16: v, Valid: true} })
}

// TextFK converts a string parent key into a pgtype.Text FK field.
func TextFK() seedling.FKConverter {
	return seedling.ConvertFK(func(v string) pgtype.Text { return pgtype.Text{String: v, Valid: true} })
}

// UUIDFK converts a 16-byte parent key, such as a github.com/google/uuid UUID,
// into a pgtype.UUID FK field.
func UUIDFK() seedling.FKConverter {
	return seedling.ConvertFK(func(v [16]byte) pgtype.UUID { return pgtype.UUID{Bytes: v, Valid: true} })
}
