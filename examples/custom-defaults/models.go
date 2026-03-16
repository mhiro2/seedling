package customdefaults

// User represents a user with role and status fields
// that benefit from type-safe default customization via With().
type User struct {
	ID     int
	Name   string
	Email  string
	Role   string
	Status string
}
