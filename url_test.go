package tsuibami

import (
	"errors"
	"testing"
)

func TestParseURL(t *testing.T) {
	ok := []struct {
		in       string
		user, id string
	}{
		{"https://x.com/example_user/status/1234567890", "example_user", "1234567890"},
		{"https://twitter.com/example_user/status/1234567890", "example_user", "1234567890"},
		{"https://www.x.com/example_user/status/1234567890", "example_user", "1234567890"},
		{"https://mobile.twitter.com/example_user/status/1234567890", "example_user", "1234567890"},
		{"http://X.com/example_user/status/1234567890", "example_user", "1234567890"},
		{"x.com/example_user/status/1234567890", "example_user", "1234567890"},
		{"https://x.com/example_user/status/1234567890/photo/1", "example_user", "1234567890"},
		{"https://x.com/example_user/status/1234567890?s=20&t=abc", "example_user", "1234567890"},
		{"https://x.com/i/web/status/1234567890", "i", "1234567890"},
		{"https://x.com/i/status/1234567890", "i", "1234567890"},
		{"1234567890", "i", "1234567890"},
		{"  1234567890\n", "i", "1234567890"},
	}
	for _, tt := range ok {
		user, id, err := ParseURL(tt.in)
		if err != nil || user != tt.user || id != tt.id {
			t.Errorf("ParseURL(%q) = %q, %q, %v; want %q, %q", tt.in, user, id, err, tt.user, tt.id)
		}
	}

	ng := []string{
		"",
		"https://example.com/example_user/status/1234567890",
		"https://x.com.example.com/example_user/status/1234567890",
		"https://x.com/example_user",
		"https://x.com/example_user/status/",
		"https://x.com/example_user/status/abc",
		"https://x.com/example_user/likes",
		"https://x.com/../status/1234567890",
		"https://x.com/a%2Fb/status/1234567890",
		"ftp://x.com/example_user/status/1234567890",
		"12345678901234567890123456",
	}
	for _, in := range ng {
		if _, _, err := ParseURL(in); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("ParseURL(%q) err = %v; want ErrInvalidURL", in, err)
		}
	}
}
