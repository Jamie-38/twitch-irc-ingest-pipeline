package oauth

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/types"
)

func LoadTokenJSON(path string) (types.Token, error) {
	var tok types.Token

	f, err := os.Open(path)
	if err != nil {
		return tok, fmt.Errorf("open account file %q: %w", path, err)
	}
	defer func() {
		_ = f.Close()
	}()

	if err := json.NewDecoder(f).Decode(&tok); err != nil {
		return tok, fmt.Errorf("decode token json %q: %w", path, err)
	}

	if tok.AccessToken == "" {
		return tok, fmt.Errorf("token %q missing access_token", path)
	}
	return tok, nil
}
