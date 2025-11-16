package types

type IRCCommand struct {
	Op      string // "JOIN", "PART", etc.
	Channel string // e.g., "#chess"
}
