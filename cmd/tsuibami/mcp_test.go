package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ngram/tsuibami"
)

var statusPathRe = regexp.MustCompile(`^/[A-Za-z0-9_]+/status/([0-9]+)$`)

// newChainServer は、ID が n の投稿が n-1 へのリプライになっている fxtwitter の代わりのサーバーを作る。
// ID 1000 の投稿はリプライではない。1000 より小さい ID には 404 を返す。
func newChainServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var n int
		if m := statusPathRe.FindStringSubmatch(r.URL.Path); m != nil {
			n, _ = strconv.Atoi(m[1])
		}
		if n < 1000 {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":404,"message":"NOT_FOUND"}`))
			return
		}
		replyTo := "null"
		if n > 1000 {
			replyTo = fmt.Sprintf(`"%d"`, n-1)
		}
		fmt.Fprintf(w, `{"code":200,"message":"OK","tweet":{"id":"%d","url":"https://x.com/example_user/status/%d","text":"投稿%d","author":{"name":"Example","screen_name":"example_user"},"created_timestamp":1767571200,"replying_to":"example_user","replying_to_status":%s}}`, n, n, n, replyTo)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// connect は、環境変数 TSUIBAMI_FX_API_BASE に接続先を設定してサーバーを作り、メモリ上でクライアントとつなぐ。
func connect(t *testing.T, baseURL string) *mcp.ClientSession {
	t.Helper()
	t.Setenv(envBaseURL, baseURL)
	ctx := context.Background()

	server := newServer(newClient(tsuibami.WithInterval(0), tsuibami.WithFallbackURL("")))
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func callReadPost(t *testing.T, cs *mcp.ClientSession, args map[string]any) (*mcp.CallToolResult, *tsuibami.Thread) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "read_post", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		return res, nil
	}

	// StructuredContent と TextContent の両方に、同じ Thread の JSON が入る。
	if res.StructuredContent == nil {
		t.Fatal("StructuredContent = nil")
	}
	if len(res.Content) != 1 {
		t.Fatalf("len(Content) = %d; want 1", len(res.Content))
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] = %T; want *mcp.TextContent", res.Content[0])
	}
	var fromText tsuibami.Thread
	if err := json.Unmarshal([]byte(text.Text), &fromText); err != nil {
		t.Fatalf("TextContent が Thread の JSON ではない: %v\n%s", err, text.Text)
	}
	b, _ := json.Marshal(res.StructuredContent)
	var fromStructured tsuibami.Thread
	if err := json.Unmarshal(b, &fromStructured); err != nil {
		t.Fatal(err)
	}
	if fromText.Post.ID != fromStructured.Post.ID || len(fromText.Parents) != len(fromStructured.Parents) {
		t.Errorf("TextContent と StructuredContent が違う")
	}
	return res, &fromText
}

func TestListTools(t *testing.T) {
	srv, _ := newChainServer(t)
	cs := connect(t, srv.URL)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 1 || res.Tools[0].Name != "read_post" {
		t.Fatalf("Tools = %+v", res.Tools)
	}
	tool := res.Tools[0]
	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Errorf("Annotations = %+v; want ReadOnlyHint", tool.Annotations)
	}
	if !strings.Contains(tool.Description, "WebFetch") {
		t.Errorf("Description = %q", tool.Description)
	}
	if tool.OutputSchema == nil {
		t.Error("OutputSchema = nil")
	}
}

func TestReadPost(t *testing.T) {
	srv, hits := newChainServer(t)
	cs := connect(t, srv.URL)

	t.Run("depthを省くと10段までさかのぼる", func(t *testing.T) {
		_, th := callReadPost(t, cs, map[string]any{"url": "https://x.com/example_user/status/1015"})
		if th == nil {
			t.Fatal("IsError")
		}
		if len(th.Parents) != tsuibami.DefaultDepth || th.Parents[0].ID != "1005" || !th.Truncated {
			t.Errorf("len(Parents) = %d, Truncated = %v", len(th.Parents), th.Truncated)
		}
	})

	t.Run("depthに0を渡すと指定の投稿だけ", func(t *testing.T) {
		_, th := callReadPost(t, cs, map[string]any{"url": "1015", "depth": 0})
		if th == nil {
			t.Fatal("IsError")
		}
		if len(th.Parents) != 0 || th.Post.ID != "1015" || !th.Truncated {
			t.Errorf("Parents = %d, Post = %s, Truncated = %v", len(th.Parents), th.Post.ID, th.Truncated)
		}
	})

	t.Run("depthを指定する", func(t *testing.T) {
		_, th := callReadPost(t, cs, map[string]any{"url": "1003", "depth": 20})
		if th == nil {
			t.Fatal("IsError")
		}
		if len(th.Parents) != 3 || th.Truncated {
			t.Errorf("len(Parents) = %d, Truncated = %v", len(th.Parents), th.Truncated)
		}
	})

	// 接続先は環境変数 TSUIBAMI_FX_API_BASE で渡したサーバーだけ。
	if hits.Load() == 0 {
		t.Error("TSUIBAMI_FX_API_BASE のサーバーに要求が来ていない")
	}
}

func TestReadPostErrors(t *testing.T) {
	srv, _ := newChainServer(t)
	cs := connect(t, srv.URL)

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"URLの誤り", map[string]any{"url": "https://example.com/page"}, "x.comかtwitter.comの投稿のURL"},
		{"depthの誤り", map[string]any{"url": "1015", "depth": 51}, "depth"},
		{"投稿が無い", map[string]any{"url": "999"}, "NOT_FOUND"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, _ := callReadPost(t, cs, tt.args)
			if !res.IsError {
				t.Fatal("IsError = false")
			}
			text := res.Content[0].(*mcp.TextContent).Text
			if !strings.Contains(text, tt.want) {
				t.Errorf("Text = %q; want contains %q", text, tt.want)
			}
		})
	}
}
