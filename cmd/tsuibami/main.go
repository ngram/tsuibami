// tsuibami は、x.com の投稿とリプライ元を fxtwitter で取り、JSON で返す。
//
//	tsuibami [--depth N] <URL>   標準出力に JSON を書く
//	tsuibami mcp                 標準入出力で動く MCP サーバー
//	tsuibami version             版を書く
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/ngram/tsuibami"
)

// envBaseURL は、自分の FxEmbed の接続先を指定する環境変数。
const envBaseURL = "TSUIBAMI_FX_API_BASE"

const (
	exitOK       = 0
	exitFetchErr = 1
	exitUsage    = 2
)

const usage = `使い方:
  tsuibami [--depth N] <URL>   投稿とリプライ元をJSONで標準出力に書く
  tsuibami mcp                 標準入出力で動くMCPサーバーを起動する
  tsuibami version             版を書く

<URL>は、x.comかtwitter.comの投稿のURLか、数字だけの投稿のID。
--depthは、リプライ元をさかのぼる段数（既定は10、上限は50）。

環境変数:
  TSUIBAMI_FX_API_BASE  自分のFxEmbedの接続先（例: http://localhost:8787）。
                        無ければ公開のhttps://api.fxtwitter.comを使う
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "mcp":
			if err := runMCP(ctx, newClient()); err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintln(stderr, "tsuibami:", err)
				return exitFetchErr
			}
			return exitOK
		case "version":
			fmt.Fprintln(stdout, "tsuibami", version())
			return exitOK
		}
	}
	return runFetch(ctx, newClient(), args, stdout, stderr)
}

func runFetch(ctx context.Context, c *tsuibami.Client, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tsuibami", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	depth := fs.Int("depth", tsuibami.DefaultDepth, "")
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, usage)
		return exitOK
	} else if err != nil {
		return usageError(stderr, err)
	}
	// URL の後ろに --depth を書いても読めるように、URL より後ろももう一度読む。
	rest := fs.Args()
	if len(rest) > 1 {
		if err := fs.Parse(rest[1:]); err != nil {
			return usageError(stderr, err)
		}
		rest = append(rest[:1], fs.Args()...)
	}
	if len(rest) != 1 {
		return usageError(stderr, errors.New("URLを1つだけ渡してください"))
	}
	if *depth < 0 || *depth > tsuibami.MaxDepth {
		return usageError(stderr, fmt.Errorf("--depthは0から%dまでの整数にしてください", tsuibami.MaxDepth))
	}
	if _, _, err := tsuibami.ParseURL(rest[0]); err != nil {
		return usageError(stderr, err)
	}

	th, err := c.Thread(ctx, rest[0], *depth)
	if err != nil {
		fmt.Fprintln(stderr, "tsuibami:", err)
		return exitFetchErr
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(th); err != nil {
		fmt.Fprintln(stderr, "tsuibami:", err)
		return exitFetchErr
	}
	return exitOK
}

func usageError(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "tsuibami:", err)
	fmt.Fprint(stderr, "\n", usage)
	return exitUsage
}

// newClient は、環境変数の接続先を使う Client を作る。CLI と MCP サーバーの両方で使う。
func newClient(opts ...tsuibami.Option) *tsuibami.Client {
	opts = append([]tsuibami.Option{
		tsuibami.WithBaseURL(os.Getenv(envBaseURL)),
		tsuibami.WithUserAgent(tsuibami.DefaultUserAgent + "/" + version()),
	}, opts...)
	return tsuibami.New(opts...)
}

// version は、go install で入れたときのモジュールの版を返す。手元で作ったときは "(devel)"。
func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(devel)"
}
