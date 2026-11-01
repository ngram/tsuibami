package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ngram/tsuibami"
)

const readPostDescription = `x.comとtwitter.comの投稿を読む。投稿の本文、作者、日時、画像と動画のURL、引用元、X Articleの本文と、リプライ元（古い順）をJSONで返す。
x.comとtwitter.comの投稿は、WebFetchでは本文を取れないので、このツールで読む。
リプライ元を途中までしか取らなかったときは、truncatedがtrueになり、truncated_reasonに理由が入る。`

type readPostInput struct {
	URL   string `json:"url" jsonschema:"読む投稿のURL（https://x.com/<ユーザー名>/status/<ID>など）か、数字だけの投稿のID"`
	Depth *int   `json:"depth,omitempty" jsonschema:"リプライ元をさかのぼる段数。省略時は10、上限は50。0なら指定の投稿だけを返す"`
}

// newServer は、ツール read_post を 1 つ持つ MCP サーバーを作る。
// ツールの呼び出しはすべて同じ Client を使うので、取得の間隔は呼び出しの間でも共有される。
func newServer(c *tsuibami.Client) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "tsuibami", Version: version()}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_post",
		Title:       "x.comの投稿を読む",
		Description: readPostDescription,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in readPostInput) (*mcp.CallToolResult, *tsuibami.Thread, error) {
		depth := tsuibami.DefaultDepth
		if in.Depth != nil {
			depth = *in.Depth
		}
		// Content を設定せずに返すと、SDK が Thread を StructuredContent に入れ、
		// JSON の文字列にしたものを TextContent にも入れる。
		th, err := c.Thread(ctx, in.URL, depth)
		if err != nil {
			return nil, nil, err
		}
		return nil, th, nil
	})
	return server
}

func runMCP(ctx context.Context, c *tsuibami.Client) error {
	return newServer(c).Run(ctx, &mcp.StdioTransport{})
}
