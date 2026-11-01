package tsuibami

import (
	"context"
	"log/slog"
	"net/http"
)

// DefaultBaseURL は公開の fxtwitter (FxEmbed) の接続先。
//
// この枠は世界中で共有されている。X はゲストのトークンごとに 15 分 500 回までしか
// 返さないので、自分の FxEmbed を WithBaseURL で指定すると、その枠を使わずに済む。
const DefaultBaseURL = "https://api.fxtwitter.com"

// get は fxtwitter の API に GET を送る。path は "/" で始まる（例: "/jack/status/20"）。
//
// 自分の FxEmbed を指定したときは、次の 2 つの場合だけ公開版で取り直す。
//
//   - つながらない（起動していない、名前を引けない、時間切れになった）。
//     呼び出し元が ctx をキャンセルしたときは、つながらないのではないので取り直さない。
//   - 404 を返した。自分の FxEmbed はゲストとして X に問い合わせるので、年齢制限のある投稿や
//     フォロワー限定の投稿を 404 にする。公開版は X の資格情報で動いているので返せることがある。
//
// 本当に削除された投稿は両方で 404 になり、要求が 1 回増えるだけで結果は変わらない。
func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	resp, err := c.getFrom(ctx, c.baseURL, path)
	if c.fallbackURL == "" || c.baseURL == c.fallbackURL {
		return resp, err
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		c.noteDown(err)
		return c.getFrom(ctx, c.fallbackURL, path)
	}
	c.noteUp()

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		slog.Info("自分のFxEmbedが404を返したので、公開版で取り直す", "path", path)
		return c.getFrom(ctx, c.fallbackURL, path)
	}
	return resp, nil
}

func (c *Client) getFrom(ctx context.Context, base, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	// FxEmbed の API は User-Agent の無い要求を 401 で拒む。
	req.Header.Set("User-Agent", c.userAgent)
	return c.httpClient.Do(req)
}

// noteDown と noteUp は、切り替えの状態が変わったときだけログを書く。
// 取得のたびに書くと、MCP では Claude Code のログが同じ行で埋まる。

func (c *Client) noteDown(err error) {
	c.downMu.Lock()
	defer c.downMu.Unlock()
	if !c.down {
		c.down = true
		slog.Warn("自分のFxEmbedにつながらないので、公開版を使う（公開版の枠は世界中で共有されている）",
			"base", c.baseURL, "err", err)
	}
}

func (c *Client) noteUp() {
	c.downMu.Lock()
	defer c.downMu.Unlock()
	if c.down {
		c.down = false
		slog.Info("自分のFxEmbedにつながるようになった", "base", c.baseURL)
	}
}
