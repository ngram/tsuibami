package tsuibami

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "testdata の期待値を書き直す")

var statusPathRe = regexp.MustCompile(`^/[A-Za-z0-9_]+/status/([0-9]+)$`)

// newFakeFx は、testdata/fx/<ID>.json を返す fxtwitter の代わりのサーバーを作る。
// ファイルが無い ID には、fxtwitter と同じく 404 を返す。
func newFakeFx(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := statusPathRe.FindStringSubmatch(r.URL.Path)
		if m == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		b, err := os.ReadFile(filepath.Join("testdata", "fx", m[1]+".json"))
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":404,"message":"NOT_FOUND"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(t *testing.T) *Client {
	return New(WithBaseURL(newFakeFx(t).URL), WithFallbackURL(""), WithInterval(0))
}

func parentIDs(th *Thread) []string {
	var ids []string
	for _, p := range th.Parents {
		ids = append(ids, p.ID)
	}
	return ids
}

func TestThread(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	t.Run("古い順に並ぶ", func(t *testing.T) {
		th, err := c.Thread(ctx, "https://x.com/example_alice/status/102", DefaultDepth)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(parentIDs(th), ","); got != "100,101" || th.Post.ID != "102" {
			t.Errorf("Parents = %s, Post = %s", got, th.Post.ID)
		}
		if th.Truncated || th.TruncatedReason != "" {
			t.Errorf("Truncated = %v, %q", th.Truncated, th.TruncatedReason)
		}
	})

	t.Run("depthで止まる", func(t *testing.T) {
		th, err := c.Thread(ctx, "102", 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(parentIDs(th), ","); got != "101" {
			t.Errorf("Parents = %s", got)
		}
		if !th.Truncated || !strings.Contains(th.TruncatedReason, "上限（1段）") || !strings.Contains(th.TruncatedReason, "/status/100") {
			t.Errorf("Truncated = %v, %q", th.Truncated, th.TruncatedReason)
		}
	})

	t.Run("depthが0なら指定の投稿だけ", func(t *testing.T) {
		th, err := c.Thread(ctx, "102", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(th.Parents) != 0 || th.Post.ID != "102" || !th.Truncated {
			t.Errorf("Parents = %v, Post = %s, Truncated = %v", parentIDs(th), th.Post.ID, th.Truncated)
		}
	})

	t.Run("リプライ元を取れなければ打ち切る", func(t *testing.T) {
		th, err := c.Thread(ctx, "200", DefaultDepth)
		if err != nil {
			t.Fatal(err)
		}
		if len(th.Parents) != 0 || !th.Truncated || !strings.Contains(th.TruncatedReason, "取れなかった") || !strings.Contains(th.TruncatedReason, "NOT_FOUND") {
			t.Errorf("Parents = %v, Truncated = %v, %q", parentIDs(th), th.Truncated, th.TruncatedReason)
		}
	})

	t.Run("循環で止まる", func(t *testing.T) {
		th, err := c.Thread(ctx, "300", DefaultDepth)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(parentIDs(th), ","); got != "301" || !th.Truncated || !strings.Contains(th.TruncatedReason, "循環") {
			t.Errorf("Parents = %s, Truncated = %v, %q", got, th.Truncated, th.TruncatedReason)
		}
	})

	t.Run("リプライでない投稿", func(t *testing.T) {
		th, err := c.Thread(ctx, "100", DefaultDepth)
		if err != nil {
			t.Fatal(err)
		}
		if len(th.Parents) != 0 || th.Truncated {
			t.Errorf("Parents = %v, Truncated = %v", parentIDs(th), th.Truncated)
		}
	})
}

func TestThreadErrors(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	if _, err := c.Thread(ctx, "https://example.com/", 0); !errors.Is(err, ErrInvalidURL) {
		t.Errorf("URLの誤り: err = %v", err)
	}
	for _, d := range []int{-1, MaxDepth + 1} {
		if _, err := c.Thread(ctx, "102", d); err == nil || !strings.Contains(err.Error(), "depth") {
			t.Errorf("depth = %d: err = %v", d, err)
		}
	}
	// 指定の投稿を取れなければエラーにする。
	var apiErr *APIError
	if _, err := c.Thread(ctx, "999", 0); !errors.As(err, &apiErr) || apiErr.Code != 404 {
		t.Errorf("存在しない投稿: err = %v", err)
	}
}

// 投稿 1 件ずつの取得に間隔が空くことを確かめる。3 件目の予約は 1 件目から 2 間隔後なので、
// 全体はそれより短くならない（タイマーは予約した時刻より前には発火しない）。
func TestThreadInterval(t *testing.T) {
	const interval = 30 * time.Millisecond
	c := New(WithBaseURL(newFakeFx(t).URL), WithFallbackURL(""), WithInterval(interval))
	start := time.Now()
	if _, err := c.Thread(context.Background(), "102", DefaultDepth); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 2*interval {
		t.Errorf("3件の取得が %v で終わった（%v 以上かかるはず）", d, 2*interval)
	}
}

// 出力の JSON の形を、testdata の期待値と比べる。欄の名前や形が変わったら、
// 意図した変更かを確かめてから go test -run TestThreadJSON -update で書き直す。
func TestThreadJSON(t *testing.T) {
	c := newTestClient(t)
	for _, id := range []string{"102", "400"} {
		t.Run(id, func(t *testing.T) {
			th, err := c.Thread(context.Background(), id, DefaultDepth)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(th, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join("testdata", "thread_"+id+".golden.json")
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
				t.Errorf("出力が期待値と違う。\n--- got\n%s\n--- want\n%s", got, want)
			}
		})
	}
}
