package tsuibami

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Thread は、指定の投稿と、さかのぼったリプライ元。CLIとMCPの出力の形でもある。
type Thread struct {
	// Parents は古い順のリプライ元。最後の要素が、Post の直接のリプライ先。
	Parents []Post `json:"parents,omitempty"`
	// Post は指定の投稿。
	Post Post `json:"post"`
	// Truncated は、さらに前のリプライ元があるのに、取らずに打ち切ったかどうか。
	Truncated bool `json:"truncated,omitempty"`
	// TruncatedReason は打ち切った理由。
	TruncatedReason string `json:"truncated_reason,omitempty"`
}

// Post は投稿1件。
//
// 引用元を *Post ではなく *PostBody で持つのは、MCPのSDKが型から出力のスキーマを作るときに、
// 再帰する型を扱えないため。fxtwitterも、引用元の中の引用元は返さない。
type Post struct {
	PostBody
	// Quote は引用元の投稿。
	Quote *PostBody `json:"quote,omitempty"`
}

// PostBody は、投稿のうち引用元を除いた部分。
type PostBody struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Text      string    `json:"text,omitempty"`
	CreatedAt time.Time `json:"created_at,omitzero"`
	Author    Author    `json:"author"`
	Photos    []Photo   `json:"photos,omitempty"`
	Videos    []Video   `json:"videos,omitempty"`
	// ReplyingTo はリプライ先。リプライでなければ nil。
	ReplyingTo *ReplyTarget `json:"replying_to,omitempty"`
	// Article は X Article の本文。X Article でなければ nil。
	Article *Article `json:"article,omitempty"`
}

// Author は投稿の作者。
type Author struct {
	Name       string `json:"name"`
	ScreenName string `json:"screen_name"`
}

// Photo は画像1枚。URLは元の大きさの画像を指す。
type Photo struct {
	URL string `json:"url"`
}

// Video は動画1本。
type Video struct {
	// Type は "video" か "gif"。
	Type string `json:"type,omitempty"`
	URL  string `json:"url"`
}

// ReplyTarget はリプライ先の投稿。
type ReplyTarget struct {
	ScreenName string `json:"screen_name,omitempty"`
	ID         string `json:"id"`
}

// Article は X Article。
type Article struct {
	Title         string         `json:"title,omitempty"`
	CoverImageURL string         `json:"cover_image_url,omitempty"`
	Blocks        []ArticleBlock `json:"blocks,omitempty"`
}

// ArticleBlock の Type の値。
const (
	BlockParagraph       = "paragraph"
	BlockHeading         = "heading"
	BlockQuote           = "quote"
	BlockCode            = "code"
	BlockListItem        = "list_item"
	BlockOrderedListItem = "ordered_list_item"
	BlockImage           = "image"
	BlockPost            = "post"
	BlockDivider         = "divider"
)

// ArticleBlock は X Article の本文のブロック1つ。
type ArticleBlock struct {
	Type string `json:"type"`
	// Level は見出しの段（1〜3）。見出しのときだけ使う。
	Level int    `json:"level,omitempty"`
	Text  string `json:"text,omitempty"`
	// URL は画像のURL。画像のときだけ使う。
	URL string `json:"url,omitempty"`
	// PostID は埋め込まれた投稿のID。投稿は取りに行かない。
	PostID string `json:"post_id,omitempty"`
}

// fxtwitterの応答のうち、tsuibamiが読む部分。

type fxResponse struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Tweet   fxTweet `json:"tweet"`
}

type fxTweet struct {
	ID               string     `json:"id"`
	URL              string     `json:"url"`
	Text             string     `json:"text"`
	CreatedTimestamp int64      `json:"created_timestamp"`
	Author           fxAuthor   `json:"author"`
	Media            *fxMedia   `json:"media"`
	Quote            *fxTweet   `json:"quote"`
	ReplyingTo       string     `json:"replying_to"`
	ReplyingToStatus string     `json:"replying_to_status"`
	Article          *fxArticle `json:"article"`
}

type fxAuthor struct {
	Name       string `json:"name"`
	ScreenName string `json:"screen_name"`
}

type fxMedia struct {
	Photos []fxPhoto `json:"photos"`
	Videos []fxVideo `json:"videos"`
}

type fxPhoto struct {
	URL string `json:"url"`
}

type fxVideo struct {
	Type    string          `json:"type"`
	URL     string          `json:"url"`
	Formats []fxVideoFormat `json:"formats"`
}

type fxVideoFormat struct {
	URL       string `json:"url"`
	Bitrate   int    `json:"bitrate"`
	Container string `json:"container"`
}

type fxArticle struct {
	Title         string           `json:"title"`
	PreviewText   string           `json:"preview_text"`
	CoverMedia    *fxArticleMedia  `json:"cover_media"`
	Content       *fxDraftContent  `json:"content"`
	MediaEntities []fxArticleMedia `json:"media_entities"`
}

type fxArticleMedia struct {
	MediaID   string `json:"media_id"`
	MediaInfo struct {
		OriginalImgURL string `json:"original_img_url"`
	} `json:"media_info"`
}

type fxDraftContent struct {
	Blocks    []fxDraftBlock  `json:"blocks"`
	EntityMap json.RawMessage `json:"entityMap"`
}

type fxDraftBlock struct {
	Text         string `json:"text"`
	Type         string `json:"type"`
	EntityRanges []struct {
		Key int `json:"key"`
	} `json:"entityRanges"`
}

type fxEntity struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// toPost は fxtwitter の投稿を Post に直す。
func (t *fxTweet) toPost() Post {
	p := Post{PostBody: t.toBody()}
	if t.Quote != nil {
		q := t.Quote.toBody()
		p.Quote = &q
	}
	return p
}

func (t *fxTweet) toBody() PostBody {
	b := PostBody{
		ID:     t.ID,
		URL:    t.URL,
		Text:   t.Text,
		Author: Author{Name: t.Author.Name, ScreenName: t.Author.ScreenName},
	}
	if t.CreatedTimestamp > 0 {
		b.CreatedAt = time.Unix(t.CreatedTimestamp, 0).UTC()
	}
	if t.Media != nil {
		for _, ph := range t.Media.Photos {
			if ph.URL != "" {
				b.Photos = append(b.Photos, Photo{URL: ph.URL})
			}
		}
		for _, v := range t.Media.Videos {
			if u := bestVideoURL(v); u != "" {
				b.Videos = append(b.Videos, Video{Type: v.Type, URL: u})
			}
		}
	}
	if t.ReplyingToStatus != "" {
		b.ReplyingTo = &ReplyTarget{ScreenName: t.ReplyingTo, ID: t.ReplyingToStatus}
	}
	if t.Article != nil {
		b.Article = t.Article.toArticle()
	}
	return b
}

// bestVideoURL は formats の中から最高ビットレートの MP4 URL を返す。
// formats に MP4 が無い場合は fxtwitter がデフォルト最高画質として返す URL を使う。
func bestVideoURL(v fxVideo) string {
	var best string
	var bestBitrate int
	for _, f := range v.Formats {
		if f.Container != "mp4" {
			continue
		}
		if f.Bitrate > bestBitrate {
			bestBitrate = f.Bitrate
			best = f.URL
		}
	}
	if best != "" {
		return best
	}
	return v.URL
}

// toArticle は Draft.js の形の本文を、ArticleBlock の列に直す。
func (a *fxArticle) toArticle() *Article {
	art := &Article{Title: a.Title}
	if a.CoverMedia != nil {
		art.CoverImageURL = a.CoverMedia.MediaInfo.OriginalImgURL
	}
	if a.Content == nil {
		if a.PreviewText != "" {
			art.Blocks = []ArticleBlock{{Type: BlockParagraph, Text: a.PreviewText}}
		}
		return art
	}

	images := make(map[string]string, len(a.MediaEntities))
	for _, m := range a.MediaEntities {
		if m.MediaID != "" && m.MediaInfo.OriginalImgURL != "" {
			images[m.MediaID] = m.MediaInfo.OriginalImgURL
		}
	}
	entities := parseEntityMap(a.Content.EntityMap)

	for _, block := range a.Content.Blocks {
		switch block.Type {
		case "atomic":
			if len(block.EntityRanges) == 0 {
				continue
			}
			if e, ok := entities[block.EntityRanges[0].Key]; ok {
				art.Blocks = append(art.Blocks, entityBlocks(e, images)...)
			}
		case "header-one", "header-two", "header-three":
			if strings.TrimSpace(block.Text) == "" {
				continue
			}
			level := map[string]int{"header-one": 1, "header-two": 2, "header-three": 3}[block.Type]
			art.Blocks = append(art.Blocks, ArticleBlock{Type: BlockHeading, Level: level, Text: block.Text})
		case "blockquote":
			art.Blocks = append(art.Blocks, ArticleBlock{Type: BlockQuote, Text: block.Text})
		case "code-block":
			art.Blocks = append(art.Blocks, ArticleBlock{Type: BlockCode, Text: block.Text})
		case "unordered-list-item":
			art.Blocks = append(art.Blocks, ArticleBlock{Type: BlockListItem, Text: block.Text})
		case "ordered-list-item":
			art.Blocks = append(art.Blocks, ArticleBlock{Type: BlockOrderedListItem, Text: block.Text})
		default:
			// unstyled と、知らない種類のブロックは段落にする。空の段落は省く。
			if strings.TrimSpace(block.Text) == "" {
				continue
			}
			art.Blocks = append(art.Blocks, ArticleBlock{Type: BlockParagraph, Text: block.Text})
		}
	}
	return art
}

// parseEntityMap は Draft.js の entityMap を、番号から要素への対応に直す。
//
// fxtwitterは [{"key":"0","value":{...}}] の形で返す。Draft.js のほかの書き方に備えて、
// {"0":{...}} の形と、番号順の配列 [{...}] の形も読む。
func parseEntityMap(raw json.RawMessage) map[int]fxEntity {
	out := make(map[int]fxEntity)
	if len(raw) == 0 {
		return out
	}

	var pairs []struct {
		Key   json.RawMessage `json:"key"`
		Value *fxEntity       `json:"value"`
	}
	if json.Unmarshal(raw, &pairs) == nil && len(pairs) > 0 && pairs[0].Value != nil {
		for _, p := range pairs {
			k, err := strconv.Atoi(strings.Trim(string(p.Key), `"`))
			if err == nil && p.Value != nil {
				out[k] = *p.Value
			}
		}
		return out
	}

	var obj map[string]fxEntity
	if json.Unmarshal(raw, &obj) == nil {
		for ks, e := range obj {
			if k, err := strconv.Atoi(ks); err == nil {
				out[k] = e
			}
		}
		return out
	}

	var arr []fxEntity
	if json.Unmarshal(raw, &arr) == nil {
		for i, e := range arr {
			out[i] = e
		}
	}
	return out
}

// entityBlocks は atomic のブロックが指す要素を、ArticleBlock に直す。
func entityBlocks(e fxEntity, images map[string]string) []ArticleBlock {
	switch e.Type {
	case "DIVIDER":
		return []ArticleBlock{{Type: BlockDivider}}
	case "TWEET":
		var data struct {
			TweetID string `json:"tweetId"`
		}
		if json.Unmarshal(e.Data, &data) == nil && data.TweetID != "" {
			return []ArticleBlock{{Type: BlockPost, PostID: data.TweetID}}
		}
	case "MEDIA":
		// 画像のURLは要素に無く、mediaId で media_entities を引く。
		// 古い形の応答に備えて、要素に url があればそれを使う。
		var data struct {
			MediaItems []struct {
				MediaID string `json:"mediaId"`
				URL     string `json:"url"`
			} `json:"mediaItems"`
		}
		if json.Unmarshal(e.Data, &data) != nil {
			return nil
		}
		var blocks []ArticleBlock
		for _, item := range data.MediaItems {
			u := item.URL
			if u == "" {
				u = images[item.MediaID]
			}
			if u != "" {
				blocks = append(blocks, ArticleBlock{Type: BlockImage, URL: u})
			}
		}
		return blocks
	}
	return nil
}
