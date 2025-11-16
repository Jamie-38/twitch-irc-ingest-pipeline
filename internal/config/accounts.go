package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Jamie-38/stream-pipeline/internal/types"
)

func LoadAccount(path string) (types.Account, error) {
	var acc types.Account

	f, err := os.Open(path)
	if err != nil {
		return acc, fmt.Errorf("open account file %q: %w", path, err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	if err := dec.Decode(&acc); err != nil {
		return acc, fmt.Errorf("decode account json %q: %w", path, err)
	}

	if acc.User == "" {
		return acc, fmt.Errorf("account %q missing required field: username/name", path)
	}
	return acc, nil
}
