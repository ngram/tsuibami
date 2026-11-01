package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ngram/tsuibami"
)

func TestRunFetch(t *testing.T) {
	srv, _ := newChainServer(t)
	c := tsuibami.New(tsuibami.WithBaseURL(srv.URL), tsuibami.WithFallbackURL(""), tsuibami.WithInterval(0))

	tests := []struct {
		name        string
		args        []string
		wantCode    int
		wantParents int
		wantErr     string
	}{
		{"既定のdepth", []string{"1003"}, exitOK, 3, ""},
		{"URLの前に--depth", []string{"--depth", "1", "1003"}, exitOK, 1, ""},
		{"URLの後ろに--depth", []string{"https://x.com/example_user/status/1003", "--depth=2"}, exitOK, 2, ""},
		{"URLが無い", nil, exitUsage, 0, "URLを1つだけ"},
		{"URLが2つ", []string{"1003", "1002"}, exitUsage, 0, "URLを1つだけ"},
		{"URLの誤り", []string{"https://example.com/"}, exitUsage, 0, "x.comかtwitter.com"},
		{"depthが負", []string{"--depth", "-1", "1003"}, exitUsage, 0, "--depth"},
		{"知らないフラグ", []string{"--foo", "1003"}, exitUsage, 0, "foo"},
		{"投稿が無い", []string{"999"}, exitFetchErr, 0, "NOT_FOUND"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runFetch(context.Background(), c, tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Fatalf("code = %d; want %d\nstderr: %s", code, tt.wantCode, stderr.String())
			}
			if tt.wantErr != "" {
				if !strings.Contains(stderr.String(), tt.wantErr) {
					t.Errorf("stderr = %q; want contains %q", stderr.String(), tt.wantErr)
				}
				if stdout.Len() != 0 {
					t.Errorf("stdout = %q; want empty", stdout.String())
				}
				return
			}
			var th tsuibami.Thread
			if err := json.Unmarshal(stdout.Bytes(), &th); err != nil {
				t.Fatalf("stdout が JSON ではない: %v\n%s", err, stdout.String())
			}
			if len(th.Parents) != tt.wantParents {
				t.Errorf("len(Parents) = %d; want %d", len(th.Parents), tt.wantParents)
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), []string{arg}, &stdout, &stderr); code != exitOK || !strings.Contains(stdout.String(), "使い方") {
			t.Errorf("%s: code = %d, stdout = %q", arg, code, stdout.String())
		}
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"version"}, &stdout, &stderr); code != exitOK || !strings.HasPrefix(stdout.String(), "tsuibami ") {
		t.Errorf("code = %d, stdout = %q", code, stdout.String())
	}
}
