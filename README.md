# tsuibami

tsuibamiは、x.comとtwitter.comの投稿を、fxtwitter（FxEmbed）のAPIで取り、本文、画像と動画のURL、引用元、リプライ元をJSONで返すツールです。
Claude CodeのWebFetchではx.comの投稿の本文を読めませんが、tsuibamiをMCPサーバーとして登録すると、Claude Codeが投稿のURLから内容を読めるようになります。

## インストール

Go 1.25以降が必要です。

```sh
go install github.com/ngram/tsuibami/cmd/tsuibami@latest
```

## Claude Codeへの登録

```sh
claude mcp add --scope user tsuibami -- tsuibami mcp
```

登録すると、Claude Codeはx.comとtwitter.comの投稿のURLを、ツール`read_post`で読みます。

### 任意：呼び出しのたびの確認を省く

`~/.claude/settings.json`の許可の一覧に、ツールの名前を足します。

```json
{
  "permissions": {
    "allow": ["mcp__tsuibami__read_post"]
  }
}
```

### 任意：自分のFxEmbedを使う

公開のfxtwitter（`https://api.fxtwitter.com`）の枠は、世界中で共有されています。
自分でFxEmbedを動かしているときは、環境変数`TSUIBAMI_FX_API_BASE`に接続先を設定します。自分のFxEmbedにつながらないときと、404を返したときは、公開のfxtwitterで取り直します。

```sh
claude mcp add --scope user tsuibami -e TSUIBAMI_FX_API_BASE=http://localhost:8787 -- tsuibami mcp
```

## 使い方

### 投稿とリプライ元をJSONで読む

```sh
tsuibami https://x.com/<ユーザー名>/status/<ID>
```

リプライ元は、既定で10段までさかのぼります。段数は`--depth`で変えられます（上限は50で、0なら指定の投稿だけ）。

```sh
tsuibami --depth 0 https://x.com/<ユーザー名>/status/<ID>
```

### 出力の例

```json
{
  "parents": [
    {"id": "100", "url": "https://x.com/example_alice/status/100", "text": "最初の投稿", "created_at": "2026-01-05T00:00:00Z", "author": {"name": "Alice", "screen_name": "example_alice"}}
  ],
  "post": {
    "id": "101",
    "url": "https://x.com/example_bob/status/101",
    "text": "@example_alice 返信です",
    "created_at": "2026-01-05T01:00:00Z",
    "author": {"name": "Bob", "screen_name": "example_bob"},
    "photos": [{"url": "https://pbs.twimg.com/media/example.jpg"}],
    "replying_to": {"screen_name": "example_alice", "id": "100"}
  }
}
```

`parents`は古い順のリプライ元です。リプライ元を途中までしか取らなかったときは、`truncated`が`true`になり、`truncated_reason`に理由が入ります。

## ドキュメント

- 使い方とオプション：`tsuibami -h`
- 出力のJSONの欄と、ライブラリとしての使い方：[pkg.go.dev](https://pkg.go.dev/github.com/ngram/tsuibami)

## ライセンス

[MIT License](LICENSE)（Copyright (c) 2026 ngram）
