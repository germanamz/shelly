// Package role defines the sender roles used in LLM conversations.
package role

// Role represents the sender of a message in a conversation.
type Role string

const (
	System    Role = "system"
	User      Role = "user"
	Assistant Role = "assistant"
	Tool      Role = "tool"
)

// Valid reports whether r is one of the known roles.
func (r Role) Valid() bool {
	switch r {
	case System, User, Assistant, Tool:
		return true
	}
	return false
}

// String returns the underlying string value of the role.
func (r Role) String() string {
	return string(r)
}
