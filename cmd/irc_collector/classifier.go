package main

import (
	"context"
	"strings"

	ircevents "github.com/Jamie-38/stream-pipeline/internal/irc_events"
	"github.com/Jamie-38/stream-pipeline/internal/observe"
	"github.com/Jamie-38/stream-pipeline/internal/types"
)

func ClassifyLine(ctx context.Context, readerCh <-chan string, parseCh chan<- ircevents.Event, membershipCh chan<- types.MembershipEvent, username string) {
	lg := observe.C("classifier")

	for {
		select {
		case <-ctx.Done():
			return

		case line, ok := <-readerCh:
			if !ok { // channel closed
				lg.Info("reader channel closed")
				return
			}
			i := 0

			// TAGS
			var tags string
			var tagsMap map[string]string
			if i < len(line) && line[i] == '@' {
				j := strings.IndexByte(line[i:], ' ')
				if j < 0 {
					lg.Debug("skip malformed", "reason", "malformed tags")
					continue
				}
				tags = line[i+1 : i+j] // drop '@'
				tagsMap = parseTags(tags)
				i += j + 1
			} else {
				tagsMap = map[string]string{}
			}

			// PREFIX
			var prefix string
			if i < len(line) && line[i] == ':' {
				j := strings.IndexByte(line[i:], ' ')
				if j < 0 {
					lg.Debug("skip malformed", "reason", "malformed prefix")
					continue
				}
				prefix = line[i+1 : i+j] // drop ':'
				i += j + 1
			}

			// COMMAND
			if i >= len(line) {
				lg.Debug("skip malformed", "reason", "missing command")
				continue
			}
			var command string
			if j := strings.IndexByte(line[i:], ' '); j < 0 {
				command = line[i:]
				i = len(line)
			} else {
				command = line[i : i+j]
				i += j + 1
			}

			// PARAMS / TRAILING
			var params []string
			var trailing string
			if i <= len(line) {
				if k := strings.Index(line[i:], " :"); k >= 0 {
					paramsPart := line[i : i+k]
					trailing = line[i+k+2:] // everything to end (may contain spaces)
					params = fieldsNoEmpty(paramsPart)
				} else {
					params = fieldsNoEmpty(line[i:])
					trailing = ""
				}
			}

			lg.Debug("parsed line",
				"command", command,
				"params_len", len(params),
				"trailing_len", len(trailing),
			)

			// COMMAND HANDLERS
			switch command {
			case "PRIVMSG":
				if len(params) == 0 || len(trailing) == 0 {
					lg.Debug("skip malformed", "reason", "malformed PRIVMSG")
					continue
				}

				// From tags (authoritative when present)
				userID := tagsMap["user-id"]    // stable numeric id (string of digits)
				channelID := tagsMap["room-id"] // stable numeric id (string of digits)

				// From prefix/params (logins)
				userLogin := strings.ToLower(loginFromPrefix(prefix)) // mutable username/login
				chanLogin := strings.TrimPrefix(strings.ToLower(params[0]), "#")

				if channelID == "" && chanLogin == "" {
					lg.Debug("drop PRIVMSG: no channel id or login")
					continue
				}

				evt := ircevents.PrivMsg{
					UserID:       userID,    // may be empty if tags missing
					UserLogin:    userLogin, // may be empty if prefix absent
					ChannelID:    channelID, // may be empty if tags missing
					ChannelLogin: chanLogin, // fallback identity for channel
					Text:         trailing,
				}

				// debug print
				// fmt.Println(userID, trailing)

				select {
				case parseCh <- evt:
				case <-ctx.Done():
					return
				}

			case "JOIN", "PART":
				if len(params) == 0 {
					lg.Debug("skip malformed", "reason", "missing channel")
					continue
				}

				userLogin := strings.ToLower(loginFromPrefix(prefix))
				if userLogin == "" {
					// no prefix, can’t attribute, ignore
					continue
				}

				// own JOIN/PART as membership confirmations
				if userLogin != username {
					continue
				}

				ch := strings.ToLower(params[0])
				if !strings.HasPrefix(ch, "#") {
					ch = "#" + ch
				}

				// emit membership signal
				evt := types.MembershipEvent{
					Op:      command, // "JOIN" or "PART"
					Channel: ch,
				}
				select {
				case membershipCh <- evt:
				case <-ctx.Done():
					return
				default:
					// drop if full; rectifier will reconcile on next tick/timeout
					lg.Debug("membership event dropped (full)", "channel", ch, "op", command)
				}

			default:
				// USERNOTICE, ROOMSTATE, numerics, etc
			}
		}
	}
}

func fieldsNoEmpty(s string) []string {
	parts := strings.Fields(s)
	// strings.Fields already drops empties
	return parts
}

func loginFromPrefix(prefix string) string {
	// prefix looks like "login!login@login.tmi.twitch.tv"
	if prefix == "" {
		return ""
	}
	if idx := strings.IndexByte(prefix, '!'); idx >= 0 {
		return prefix[:idx]
	}
	// Sometimes prefix can be just "tmi.twitch.tv" for numerics; return as-is.
	return prefix
}

// tagsStr is everything after '@' up to the first space.
func parseTags(tagsStr string) map[string]string {
	tags := make(map[string]string, 16)
	if tagsStr == "" {
		return tags
	}
	for _, pair := range strings.Split(tagsStr, ";") {
		if pair == "" {
			continue
		}
		if eq := strings.IndexByte(pair, '='); eq >= 0 {
			k := pair[:eq]
			v := pair[eq+1:]
			tags[k] = unescapeIRCv3(v)
		} else {
			tags[pair] = "1"
		}
	}
	return tags
}

// IRCv3 tag value escapes: \s (space), \: (:), \; (;), \\ (\), \r, \n
func unescapeIRCv3(s string) string {
	// nothing to unescape
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 's':
			b.WriteByte(' ')
		case ':':
			b.WriteByte(':')
		case ';':
			b.WriteByte(';')
		case '\\':
			b.WriteByte('\\')
		case 'r':
			b.WriteByte('\r')
		case 'n':
			b.WriteByte('\n')
		default:
			// Unknown escape: keep as-is
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
