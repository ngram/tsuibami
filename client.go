// Package tsuibami は、fxtwitter (FxEmbed) の API で x.com の投稿とリプライ元を取る。
package tsuibami

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultDepth は、リプライ元をさかのぼる段数の既定値。
	DefaultDepth = 10
	// MaxDepth は、リプライ元をさかのぼる段数の上限。
	MaxDepth = 50
	// DefaultInterval は、投稿1件ずつの取得の間隔の既定値。
	DefaultInterval = time.Second
	// DefaultUserAgent は、fxtwitter に送る User-Agent の既定値。
	DefaultUserAgent = "github.com/ngram/tsuibami"

	defaultTimeout = 15 * time.Second
	// maxResponseSize は、読む応答の大きさの上限。X Article でも数十 KB に収まる。
	maxResponseSize = 10 << 20
)

// Client は fxtwitter から投稿を取る。複数の goroutine から同時に使ってよい。
// 取得の間隔の制限は、同じ Client を使う呼び出しの間で共有する。
type Client struct {
	httpClient  *http.Client
	baseURL     string
	fallbackURL string
	userAgent   string
	gate        *rateGate

	downMu sync.Mutex
	down   bool // 自分の FxEmbed につながらず、公開版を使っているか
}

// Option は New に渡す設定。
type Option func(*Client)

// WithHTTPClient は、要求に使う http.Client を指定する。
// 指定しないときは、15 秒で時間切れになり、リダイレクトを追わない http.Client を使う。
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithBaseURL は、最初に問い合わせる fxtwitter の接続先を指定する（例: http://localhost:8787）。
// 空文字列を渡すと、DefaultBaseURL を使う。
func WithBaseURL(u string) Option {
	return func(c *Client) {
		if u = strings.TrimRight(strings.TrimSpace(u), "/"); u != "" {
			c.baseURL = u
		}
	}
}

// WithFallbackURL は、接続先につながらないときと 404 のときに使う接続先を指定する。
// 既定は DefaultBaseURL。空文字列を渡すと、取り直さない。
func WithFallbackURL(u string) Option {
	return func(c *Client) { c.fallbackURL = strings.TrimRight(strings.TrimSpace(u), "/") }
}

// WithInterval は、投稿1件ずつの取得の間隔を指定する。0 なら待たない。
func WithInterval(d time.Duration) Option {
	return func(c *Client) { c.gate = &rateGate{min: d} }
}

// WithUserAgent は、fxtwitter に送る User-Agent を指定する。
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// New は Client を作る。
func New(opts ...Option) *Client {
	c := &Client{
		baseURL:     DefaultBaseURL,
		fallbackURL: DefaultBaseURL,
		userAgent:   DefaultUserAgent,
		gate:        &rateGate{min: DefaultInterval},
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{
			Timeout: defaultTimeout,
			// 接続先の API 以外には要求を送らないので、リダイレクトは追わない。
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return c
}

// Post は、URL か投稿の ID が指す投稿を 1 件取る。
func (c *Client) Post(ctx context.Context, rawURL string) (*Post, error) {
	user, id, err := ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	t, err := c.fetch(ctx, user, id)
	if err != nil {
		return nil, err
	}
	p := t.toPost()
	return &p, nil
}

// Thread は、URL か投稿の ID が指す投稿を取り、depth 段までリプライ元をさかのぼる。
//
// 指定の投稿を取れなければエラーを返す。リプライ元の途中で取れないとき、循環しているとき、
// depth 段に達したときは、そこで打ち切り、取れた分と打ち切った理由を返す。
// ctx がキャンセルされたときは、取れた分を捨ててエラーを返す。
func (c *Client) Thread(ctx context.Context, rawURL string, depth int) (*Thread, error) {
	if depth < 0 || depth > MaxDepth {
		return nil, fmt.Errorf("depthは0から%dまでの整数にしてください（%dが渡された）", MaxDepth, depth)
	}
	user, id, err := ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	t, err := c.fetch(ctx, user, id)
	if err != nil {
		return nil, err
	}

	th := &Thread{Post: t.toPost()}
	seen := map[string]bool{t.ID: true, id: true}
	var reversed []Post // 新しい順に集め、最後に並べ替える
	cur := t
	for i := 0; cur.ReplyingToStatus != ""; i++ {
		parentURL := statusURL(cur.ReplyingTo, cur.ReplyingToStatus)
		if i == depth {
			th.Truncated = true
			th.TruncatedReason = fmt.Sprintf("さかのぼる段数の上限（%d段）に達したので、これより前のリプライ元（%s）は取っていない", depth, parentURL)
			break
		}
		if seen[cur.ReplyingToStatus] {
			th.Truncated = true
			th.TruncatedReason = fmt.Sprintf("リプライ元が循環している（%sが2度出た）ので、打ち切った", parentURL)
			break
		}
		seen[cur.ReplyingToStatus] = true

		parent, err := c.fetch(ctx, cur.ReplyingTo, cur.ReplyingToStatus)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			th.Truncated = true
			th.TruncatedReason = fmt.Sprintf("リプライ元（%s）を取れなかったので、これより前は取っていない: %v", parentURL, err)
			break
		}
		reversed = append(reversed, parent.toPost())
		cur = parent
	}

	for i := len(reversed) - 1; i >= 0; i-- {
		th.Parents = append(th.Parents, reversed[i])
	}
	return th, nil
}

// fetch は、間隔を空けてから fxtwitter に投稿 1 件を問い合わせる。
// 公開版への取り直しは同じ 1 件に含め、間隔を空けない。
func (c *Client) fetch(ctx context.Context, user, id string) (*fxTweet, error) {
	if _, err := c.gate.wait(ctx); err != nil {
		return nil, err
	}

	if !screenNameRe.MatchString(user) {
		user = "i"
	}
	resp, err := c.get(ctx, "/"+user+"/status/"+id)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("fxtwitterにつながらなかった: %w", err)
	}
	defer resp.Body.Close()

	var fx fxResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&fx); err != nil {
		return nil, fmt.Errorf("fxtwitterの応答を読めなかった（HTTP %d）: %w", resp.StatusCode, err)
	}
	if fx.Code != http.StatusOK {
		msg := fx.Message
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return nil, &APIError{Code: fx.Code, Message: msg, PostID: id}
	}
	if fx.Tweet.ID == "" {
		return nil, errors.New("fxtwitterの応答に投稿が入っていなかった")
	}
	return &fx.Tweet, nil
}

// APIError は、fxtwitter が投稿を返さなかったことを表す。Code と Message は fxtwitter の応答のまま。
type APIError struct {
	Code    int
	Message string
	PostID  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("fxtwitterが投稿%sを返さなかった: %s（code %d）", e.PostID, e.Message, e.Code)
}

func statusURL(user, id string) string {
	if !screenNameRe.MatchString(user) {
		user = "i"
	}
	return "https://x.com/" + user + "/status/" + id
}
