package types

type MembershipEvent struct {
	Op      string // "JOIN", "PART", "ROOMSTATE", etc.
	Channel string // e.g., "#chess"
}
