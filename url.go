package tsuibami

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// ErrInvalidURL は、渡された文字列がx.comとtwitter.comの投稿のURLでも、投稿のIDでもないことを表す。
var ErrInvalidURL = errors.New("x.comかtwitter.comの投稿のURL（https://x.com/<ユーザー名>/status/<ID>）か、数字だけの投稿のIDを渡してください")

var (
	idRe         = regexp.MustCompile(`^[0-9]{1,25}$`)
	screenNameRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,50}$`)
)

// 受け付けるホスト。www.とmobile.の付いたものも含める。
var tweetHosts = map[string]bool{
	"x.com":              true,
	"www.x.com":          true,
	"mobile.x.com":       true,
	"twitter.com":        true,
	"www.twitter.com":    true,
	"mobile.twitter.com": true,
}

// ParseURL は投稿のURLか投稿のIDから、ユーザー名と投稿のIDを取り出す。
//
// 受け付けるのは、x.comとtwitter.comの /{user}/status/{id} と /i/web/status/{id}、
// 数字だけの投稿のID。/photo/1 のような後ろの続きと、クエリは無視する。
// ユーザー名が分からないときは "i" を返す。fxtwitterは /i/status/{id} でも投稿を返す。
func ParseURL(s string) (user, id string, err error) {
	s = strings.TrimSpace(s)
	if idRe.MatchString(s) {
		return "i", s, nil
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !tweetHosts[strings.ToLower(u.Hostname())] {
		return "", "", ErrInvalidURL
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch {
	// /i/web/status/{id}
	case len(parts) >= 4 && parts[0] == "i" && parts[1] == "web" && parts[2] == "status":
		user, id = "i", parts[3]
	// /{user}/status/{id}[/...]
	case len(parts) >= 3 && parts[1] == "status":
		user, id = parts[0], parts[2]
	default:
		return "", "", ErrInvalidURL
	}
	if !idRe.MatchString(id) || !screenNameRe.MatchString(user) {
		return "", "", ErrInvalidURL
	}
	return user, id, nil
}
