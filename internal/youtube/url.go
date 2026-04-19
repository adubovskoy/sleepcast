package youtube

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	hmsPattern     = regexp.MustCompile(`^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s?)?$`)
)

type VideoRef struct {
	ID           string
	StartSeconds int
}

// ParseURL accepts a raw YouTube URL or bare video ID and returns the video ID
// plus any start time encoded in the URL (t= or start=).
// Supported URL shapes: watch?v=ID, youtu.be/ID, /shorts/ID, /embed/ID, /v/ID, /live/ID.
// Supported t formats: "90", "90s", "1h2m3s", "2m", "1h".
func ParseURL(input string) (VideoRef, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return VideoRef{}, errors.New("empty input")
	}
	if videoIDPattern.MatchString(s) {
		return VideoRef{ID: s}, nil
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return VideoRef{}, err
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	host = strings.TrimPrefix(host, "m.")

	var id string
	switch host {
	case "youtu.be":
		id, err = idOrErr(strings.TrimPrefix(u.Path, "/"))
	case "youtube.com", "music.youtube.com", "youtube-nocookie.com":
		if v := u.Query().Get("v"); v != "" {
			id, err = idOrErr(v)
		} else {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) >= 2 {
				switch parts[0] {
				case "shorts", "embed", "v", "live":
					id, err = idOrErr(parts[1])
				default:
					err = errors.New("could not extract video id from URL")
				}
			} else {
				err = errors.New("could not extract video id from URL")
			}
		}
	default:
		err = errors.New("not a YouTube URL")
	}
	if err != nil {
		return VideoRef{}, err
	}

	q := u.Query()
	start := parseStartTime(q.Get("t"))
	if start == 0 {
		start = parseStartTime(q.Get("start"))
	}
	return VideoRef{ID: id, StartSeconds: start}, nil
}

// ExtractVideoID is kept for callers that only need the ID.
func ExtractVideoID(input string) (string, error) {
	r, err := ParseURL(input)
	if err != nil {
		return "", err
	}
	return r.ID, nil
}

func parseStartTime(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Plain integer (with optional trailing 's')
	if n, err := strconv.Atoi(strings.TrimSuffix(s, "s")); err == nil && n >= 0 {
		return n
	}
	// 1h2m3s / 2m30s / 1h etc.
	m := hmsPattern.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	h, _ := strconv.Atoi(m[1])
	mn, _ := strconv.Atoi(m[2])
	sec, _ := strconv.Atoi(m[3])
	total := h*3600 + mn*60 + sec
	if total < 0 {
		return 0
	}
	return total
}

func idOrErr(s string) (string, error) {
	s = strings.SplitN(s, "?", 2)[0]
	s = strings.SplitN(s, "/", 2)[0]
	if !videoIDPattern.MatchString(s) {
		return "", errors.New("invalid video id")
	}
	return s, nil
}
