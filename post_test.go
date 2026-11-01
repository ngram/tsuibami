package tsuibami

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func loadFx(t *testing.T, id string) *fxTweet {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "fx", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx fxResponse
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	return &fx.Tweet
}

func TestToPost(t *testing.T) {
	p := loadFx(t, "102").toPost()

	if p.ID != "102" || p.URL != "https://x.com/example_alice/status/102" {
		t.Errorf("ID, URL = %q, %q", p.ID, p.URL)
	}
	if want := time.Date(2026, 1, 5, 2, 0, 0, 0, time.UTC); !p.CreatedAt.Equal(want) || p.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v; want %v", p.CreatedAt, want)
	}
	if p.Author != (Author{Name: "Alice", ScreenName: "example_alice"}) {
		t.Errorf("Author = %+v", p.Author)
	}
	wantPhotos := []Photo{{URL: "https://pbs.twimg.com/media/example102a.jpg"}, {URL: "https://pbs.twimg.com/media/example102b.jpg"}}
	if !reflect.DeepEqual(p.Photos, wantPhotos) {
		t.Errorf("Photos = %+v", p.Photos)
	}
	// MP4 のうち最高ビットレートのものを選ぶ。
	wantVideos := []Video{{Type: "video", URL: "https://video.twimg.com/example102/720.mp4"}}
	if !reflect.DeepEqual(p.Videos, wantVideos) {
		t.Errorf("Videos = %+v", p.Videos)
	}
	if p.ReplyingTo == nil || *p.ReplyingTo != (ReplyTarget{ScreenName: "example_bob", ID: "101"}) {
		t.Errorf("ReplyingTo = %+v", p.ReplyingTo)
	}
	if p.Quote == nil || p.Quote.ID != "90" || len(p.Quote.Photos) != 1 || p.Quote.Photos[0].URL != "https://pbs.twimg.com/media/example90.jpg" {
		t.Errorf("Quote = %+v", p.Quote)
	}
	if p.Article != nil {
		t.Errorf("Article = %+v; want nil", p.Article)
	}

	// リプライでない投稿は ReplyingTo を持たない（fxtwitter は null を返す）。
	if root := loadFx(t, "100").toPost(); root.ReplyingTo != nil || root.Quote != nil {
		t.Errorf("root ReplyingTo, Quote = %+v, %+v; want nil", root.ReplyingTo, root.Quote)
	}
}

func TestToPostArticle(t *testing.T) {
	p := loadFx(t, "400").toPost()
	if p.Article == nil {
		t.Fatal("Article = nil")
	}
	if p.Article.Title != "記事の題" || p.Article.CoverImageURL != "https://pbs.twimg.com/media/cover.jpg" {
		t.Errorf("Title, CoverImageURL = %q, %q", p.Article.Title, p.Article.CoverImageURL)
	}
	want := []ArticleBlock{
		{Type: BlockHeading, Level: 1, Text: "大見出し"},
		{Type: BlockParagraph, Text: "段落1\n2行目"},
		{Type: BlockHeading, Level: 2, Text: "中見出し"},
		{Type: BlockImage, URL: "https://pbs.twimg.com/media/body1.jpg"},
		{Type: BlockImage, URL: "https://pbs.twimg.com/media/body2.jpg"},
		{Type: BlockDivider},
		{Type: BlockPost, PostID: "90"},
		{Type: BlockCode, Text: "go test ./..."},
		{Type: BlockQuote, Text: "引用の段落"},
		{Type: BlockListItem, Text: "項目A"},
		{Type: BlockOrderedListItem, Text: "手順1"},
		{Type: BlockHeading, Level: 3, Text: "小見出し"},
	}
	if !reflect.DeepEqual(p.Article.Blocks, want) {
		got, _ := json.MarshalIndent(p.Article.Blocks, "", "  ")
		t.Errorf("Blocks =\n%s", got)
	}
}

func TestToArticleWithoutContent(t *testing.T) {
	a := (&fxArticle{Title: "題", PreviewText: "書き出し"}).toArticle()
	want := []ArticleBlock{{Type: BlockParagraph, Text: "書き出し"}}
	if !reflect.DeepEqual(a.Blocks, want) {
		t.Errorf("Blocks = %+v", a.Blocks)
	}
}

func TestParseEntityMap(t *testing.T) {
	divider := fxEntity{Type: "DIVIDER", Data: json.RawMessage(`{}`)}
	tests := []struct {
		name string
		raw  string
	}{
		{"{key, value}の配列", `[{"key":"1","value":{"type":"DIVIDER","data":{}}}]`},
		{"番号を数値で持つ{key, value}の配列", `[{"key":1,"value":{"type":"DIVIDER","data":{}}}]`},
		{"番号をキーにした連想配列", `{"1":{"type":"DIVIDER","data":{}}}`},
		{"番号順の配列", `[{"type":"TWEET","data":{}},{"type":"DIVIDER","data":{}}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := parseEntityMap(json.RawMessage(tt.raw))
			if got, ok := m[1]; !ok || got.Type != divider.Type {
				t.Errorf("m[1] = %+v, %v", got, ok)
			}
		})
	}
	if m := parseEntityMap(nil); len(m) != 0 {
		t.Errorf("nil: %+v", m)
	}
}

func TestBestVideoURL(t *testing.T) {
	// MP4 が無ければ fxtwitter の既定の URL を使う。
	v := fxVideo{URL: "https://video.twimg.com/default.mp4", Formats: []fxVideoFormat{{URL: "https://video.twimg.com/pl.m3u8", Container: "m3u8"}}}
	if got := bestVideoURL(v); got != v.URL {
		t.Errorf("bestVideoURL = %q", got)
	}
}
