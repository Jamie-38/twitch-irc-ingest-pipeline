package oauth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/observe"
	"github.com/Jamie-38/twitch-irc-ingest-pipeline/internal/types"
)

var lg = observe.C("oauth")

func Index(w http.ResponseWriter, r *http.Request) {
	clientID := os.Getenv("TWITCH_CLIENT_ID")
	redirectURI := os.Getenv("TWITCH_REDIRECT_URI")

	authURL := fmt.Sprintf(
		"https://id.twitch.tv/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=chat:read",
		clientID, redirectURI)

	lg.Info("oauth index hit", "remote", r.RemoteAddr)

	if _, err := fmt.Fprintf(w, `<a href="%s">Click here to authenticate with Twitch</a>`, authURL); err != nil {
		lg.Warn("failed to write index response", "err", err)
	}
}

func Callback(w http.ResponseWriter, r *http.Request) {
	clientID := os.Getenv("TWITCH_CLIENT_ID")
	clientSecret := os.Getenv("TWITCH_CLIENT_SECRET")
	redirectURI := os.Getenv("TWITCH_REDIRECT_URI")
	tokenURL := "https://id.twitch.tv/oauth2/token"
	path := os.Getenv("TOKENS_PATH")

	code := r.URL.Query().Get("code")
	if code == "" {
		lg.Warn("callback missing code", "remote", r.RemoteAddr)
		http.Error(w, "Error: No code received", http.StatusBadRequest)
		return
	}

	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)

	lg.Info("exchanging code for token", "remote", r.RemoteAddr)

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		lg.Error("token request failed", "err", err)
		http.Error(w, "Failed to post", http.StatusBadGateway)
		return
	}
	if resp.StatusCode != http.StatusOK {
		lg.Error("token endpoint returned non-200", "status", resp.StatusCode)
		http.Error(w, "Token exchange failed", http.StatusBadGateway)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			lg.Warn("failed to close token response body", "err", err)
		}
	}()

	var tokenData types.Token
	if err := json.NewDecoder(resp.Body).Decode(&tokenData); err != nil {
		lg.Error("decode token response failed", "err", err)
		http.Error(w, "Failed to parse response", http.StatusBadGateway)
		return
	}

	f, err := os.Create(path)
	if err != nil {
		lg.Error("failed to write token file", "path", path, "err", err)
		http.Error(w, "Failed to write token file", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			lg.Error("failed to close token file", "path", path, "err", err)
		}
	}()

	if err := json.NewEncoder(f).Encode(tokenData); err != nil {
		lg.Error("failed to encode token file", "path", path, "err", err)
		http.Error(w, "Failed to encode token file", http.StatusInternalServerError)
		return
	}

	lg.Info("oauth token saved", "path", path, "remote", r.RemoteAddr)

	if _, err := fmt.Fprintf(w, "Authentication successful. Token saved."); err != nil {
		lg.Warn("failed to write success response", "err", err)
	}
}
