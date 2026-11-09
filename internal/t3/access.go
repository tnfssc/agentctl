package t3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Access is one way into the server: a paired device's session, or a
// pairing link nobody has used yet.
type Access struct {
	ID      string
	Link    bool // an unused pairing link
	Paired  bool // a device that paired with a link, not the desktop app or a CLI-issued token
	Label   string
	Device  string // e.g. "Chrome · macOS"
	IP      string
	Issued  time.Time
	Expires time.Time
	// LastSeen is when its current or last connection started. T3 sets it
	// only on connecting, so a device connected without a break since then
	// looks the same as one that left. Zero when it never connected.
	LastSeen time.Time
}

// Idle reports whether a paired device last connected over IdleAfter ago.
// One that never connected counts from when it was issued. Links expire on
// their own, and other sessions (the desktop app's own, tokens from `t3
// auth session issue`, agents) are never idle.
func (a Access) Idle(now time.Time) bool {
	if !a.Paired {
		return false
	}
	seen := a.LastSeen
	if seen.IsZero() {
		seen = a.Issued
	}
	return now.Sub(seen) > IdleAfter
}

type wireSession struct {
	SessionID string `json:"sessionId"`
	Subject   string `json:"subject"`
	Client    struct {
		Label      string `json:"label"`
		IPAddress  string `json:"ipAddress"`
		DeviceType string `json:"deviceType"`
		OS         string `json:"os"`
		Browser    string `json:"browser"`
	} `json:"client"`
	IssuedAt        time.Time  `json:"issuedAt"`
	ExpiresAt       time.Time  `json:"expiresAt"`
	LastConnectedAt *time.Time `json:"lastConnectedAt"`
}

type wireLink struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// ListAccess returns unused pairing links, then sessions, most recently
// seen first.
func ListAccess(ctx context.Context) ([]Access, error) {
	home, bin, err := setupCLI(ctx, authCLI)
	if err != nil {
		return nil, err
	}
	out, err := run(ctx, bin, home, "auth", "pairing", "list", "--json")
	if err != nil {
		return nil, err
	}
	var links []wireLink
	if err := json.Unmarshal(out, &links); err != nil {
		return nil, fmt.Errorf("t3 auth pairing list: %w", err)
	}
	if out, err = run(ctx, bin, home, "auth", "session", "list", "--json"); err != nil {
		return nil, err
	}
	var sessions []wireSession
	if err := json.Unmarshal(out, &sessions); err != nil {
		return nil, fmt.Errorf("t3 auth session list: %w", err)
	}

	var all []Access
	for _, l := range links {
		all = append(all, Access{ID: l.ID, Link: true, Label: l.Label, Issued: l.CreatedAt, Expires: l.ExpiresAt})
	}
	slices.SortFunc(all, func(a, b Access) int { return b.Issued.Compare(a.Issued) })
	n := len(all)
	for _, s := range sessions {
		a := Access{
			ID: s.SessionID, Paired: s.Subject == pairedSubject, Label: s.Client.Label, IP: s.Client.IPAddress,
			Issued: s.IssuedAt, Expires: s.ExpiresAt,
		}
		if s.LastConnectedAt != nil {
			a.LastSeen = *s.LastConnectedAt
		}
		var parts []string
		for _, p := range []string{s.Client.Browser, s.Client.OS} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) == 0 && s.Client.DeviceType != "" && s.Client.DeviceType != "unknown" {
			parts = append(parts, s.Client.DeviceType)
		}
		a.Device = strings.Join(parts, " · ")
		all = append(all, a)
	}
	slices.SortStableFunc(all[n:], func(a, b Access) int { return lastActive(b).Compare(lastActive(a)) })
	return all, nil
}

// pairedSubject marks sessions created by redeeming a pairing link.
const pairedSubject = "one-time-token"

func lastActive(a Access) time.Time {
	if a.LastSeen.IsZero() {
		return a.Issued
	}
	return a.LastSeen
}

// Revoke ends a session or cancels an unused link. Revoking one that is
// already gone is not an error.
func Revoke(ctx context.Context, a Access) error {
	home, bin, err := setupCLI(ctx, authCLI)
	if err != nil {
		return err
	}
	kind := "session"
	if a.Link {
		kind = "pairing"
	}
	_, err = run(ctx, bin, home, "auth", kind, "revoke", a.ID)
	return err
}

// Pairing is a one-time link that pairs a new device.
type Pairing struct {
	URL     string
	Expires time.Time
}

// DefaultTTL is how long a new pairing link works, as in `t3 pair`.
const DefaultTTL = 5 * time.Minute

// Pair mints a one-time pairing link on base, the URL devices use to reach
// the server.
func Pair(ctx context.Context, label, base string, ttl time.Duration) (Pairing, error) {
	home, bin, err := setupCLI(ctx, authCLI)
	if err != nil {
		return Pairing{}, err
	}
	if label == "" {
		label = "agentctl"
	}
	args := []string{
		"auth", "pairing", "create", "--json", "--label", label,
		"--ttl", fmt.Sprintf("%dm", max(1, int(ttl.Round(time.Minute).Minutes()))), "--base-url", base,
	}
	out, err := run(ctx, bin, home, args...)
	if err != nil {
		return Pairing{}, err
	}
	var p struct {
		PairURL   string    `json:"pairUrl"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	if err := json.Unmarshal(out, &p); err != nil || p.PairURL == "" {
		return Pairing{}, fmt.Errorf("t3 auth pairing create gave no link: %s", errorMessage(string(out), errors.New("no output")))
	}
	return Pairing{URL: p.PairURL, Expires: p.ExpiresAt}, nil
}

// LinkBase is the URL pairing links use: the one set in agentctl's config,
// else the address the server listens on.
func LinkBase(configured string, st Status) string {
	if configured != "" {
		return configured
	}
	return st.Origin
}

// Ago describes how long ago t was, coarsely: "3m ago", "2h ago", "5d ago".
// Future times read "in 4m".
func Ago(now, t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d, suffix, prefix := now.Sub(t), " ago", ""
	if d < 0 {
		d, suffix, prefix = -d, "", "in "
	}
	switch {
	case d < time.Minute:
		if prefix != "" {
			return "in <1m"
		}
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%s%dm%s", prefix, int(d.Minutes()), suffix)
	case d < 48*time.Hour:
		return fmt.Sprintf("%s%dh%s", prefix, int(d.Hours()), suffix)
	}
	return fmt.Sprintf("%s%dd%s", prefix, int(d.Hours()/24), suffix)
}

// NormalizeURL checks a base URL for pairing links. T3 replaces any path
// with /pair, so one is refused rather than silently dropped.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%q is not an http(s) URL", raw)
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%q has a path; T3 Code serves pairing links at the root of an address", raw)
	}
	return raw, nil
}

// LocalOnly reports whether a base URL only works on this machine.
func LocalOnly(base string) bool {
	u, err := url.Parse(base)
	if err != nil {
		return true
	}
	h := u.Hostname()
	return h == "localhost" || h == "::1" || strings.HasPrefix(h, "127.")
}

// CompareVersions orders exact SemVer versions, prereleases included
// (0.1.8-nightly.20261106.4262 < 0.1.8-nightly.20261108.4380 < 0.1.8).
func CompareVersions(a, b string) int {
	coreA, preA, _ := strings.Cut(strings.SplitN(a, "+", 2)[0], "-")
	coreB, preB, _ := strings.Cut(strings.SplitN(b, "+", 2)[0], "-")
	if c := compareIDs(strings.Split(coreA, "."), strings.Split(coreB, ".")); c != 0 {
		return c
	}
	switch {
	case preA == preB:
		return 0
	case preA == "":
		return 1
	case preB == "":
		return -1
	}
	return compareIDs(strings.Split(preA, "."), strings.Split(preB, "."))
}

func compareIDs(a, b []string) int {
	for i := range max(len(a), len(b)) {
		if i >= len(a) {
			return -1
		}
		if i >= len(b) {
			return 1
		}
		x, xNum := new(big.Int).SetString(a[i], 10)
		y, yNum := new(big.Int).SetString(b[i], 10)
		switch {
		case xNum && yNum:
			if c := x.Cmp(y); c != 0 {
				return c
			}
		case xNum != yNum:
			if xNum {
				return -1
			}
			return 1
		default:
			if c := strings.Compare(a[i], b[i]); c != 0 {
				return c
			}
		}
	}
	return 0
}
