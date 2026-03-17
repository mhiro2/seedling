module github.com/mhiro2/seedling/seedlingpgx

go 1.26.0

require (
	github.com/jackc/pgx/v5 v5.8.0
	github.com/mhiro2/seedling v0.0.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	golang.org/x/text v0.33.0 // indirect
)

replace github.com/mhiro2/seedling => ..
