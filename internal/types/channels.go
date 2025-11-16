package types

import (
	"time"
)

type Channels struct {
	Schema    int       `json:"schema"`
	Account   string    `json:"account"`
	UpdatedAt time.Time `json:"updated_at"`
	Channels  []string  `json:"channels"`
}
