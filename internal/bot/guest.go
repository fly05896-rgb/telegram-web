/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/telegram-web-player
 */

package bot

import (
	"ashokshau/tg-web/internal/downloader"
	"ashokshau/tg-web/internal/utils"
	"errors"
	"fmt"
	"html"
	"strings"

	td "github.com/AshokShau/gotdbot"
)

func guestHandler(c *td.Client, u *td.UpdateNewGuestQuery) error {
	if u == nil || u.Message == nil {
		return nil
	}

	if c.Me() == nil || c.Me().Usernames == nil {
		return errors.New("cannot answer guest query: bot username is unavailable")
	}

	username := c.Me().Usernames.EditableUsername

	botURL := "https://t.me/" + username
	message := fmt.Sprintf(`<h3>Music, together on Telegram</h3>
<p>Listen together with <b>Telegram Web Player</b>: synchronized music playback, voice chat, shared queues, and personal playlists.</p>
<p>Find music from YouTube, Spotify, SoundCloud, and other supported platforms.</p>
<p><b>Get started:</b> Add me to your group, use <code>/play [song]</code> to queue music, then open the Web Player with <code>/player</code>.</p>
<p>Open a private chat with me for <code>/help</code> and <code>/privacy</code>.</p>
<tg-button-row align="left"><tg-button type="url" url="%s?startgroup=true">Add to group</tg-button><tg-button type="url" url="%s?start=guest">Open bot</tg-button></tg-button-row>`, botURL, botURL)

	_, err := u.Answer(c, &td.InputInlineQueryResultArticle{
		Id:    "player-introduction",
		Title: "Music, together on Telegram",
		InputMessageContent: &td.InputMessageRichMessage{
			Message: &td.InputRichMessage{
				Source: &td.RichMessageSourceHtml{Text: message},
			},
		},
	})
	return err
}

// inlineQueryHandler powers fast inline YouTube search (@BotUsername query) ported from inline.py.
func inlineQueryHandler(c *td.Client, u *td.UpdateNewInlineQuery) error {
	if u == nil {
		return nil
	}
	query := strings.TrimSpace(u.Query)
	if query == "" {
		return u.Answer(c, []td.InputInlineQueryResult{}, &td.AnswerInlineQueryOpts{CacheTime: 10})
	}

	wrapper := downloader.NewDlWrapper(query)
	res, err := wrapper.Search()
	if err != nil || res == nil || len(res.Results) == 0 {
		return u.Answer(c, []td.InputInlineQueryResult{}, &td.AnswerInlineQueryOpts{CacheTime: 5})
	}

	botName := "TeleMusic"
	if c.Me() != nil && c.Me().FirstName != "" {
		botName = c.Me().FirstName
	}

	limit := len(res.Results)
	if limit > 15 {
		limit = 15
	}

	var results []td.InputInlineQueryResult
	for i, item := range res.Results[:limit] {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = "Unknown Track"
		}
		durStr := utils.SecToMin(item.Duration)
		views := item.Views
		if views == "" {
			views = "YouTube"
		}
		channel := item.Channel
		if channel == "" {
			channel = "YouTube"
		}
		link := item.Url
		if link == "" && item.Id != "" {
			link = "https://www.youtube.com/watch?v=" + item.Id
		}
		thumb := item.Thumbnail
		if idx := strings.Index(thumb, "?"); idx > 0 {
			thumb = thumb[:idx]
		}

		description := fmt.Sprintf("%s | %s دقيقة | %s", views, durStr, channel)
		rawMsg := fmt.Sprintf(
			"الـعـنـوان : %s\nالـمـدة : %s دقـيـقـة\nالـمـشـاهـدات : %s\nالـقـنـاة : %s\nالـرابـط : %s\n\n➻ بـحـث الانـلايـن بـواسـطـة %s",
			html.UnescapeString(title),
			durStr,
			views,
			channel,
			link,
			botName,
		)

		results = append(results, &td.InputInlineQueryResultArticle{
			Id:           fmt.Sprintf("yt_%d_%s", i, item.Id),
			Title:        title,
			Description:  description,
			ThumbnailUrl: thumb,
			Url:          link,
			InputMessageContent: &td.InputMessageText{
				Text: &td.FormattedText{
					Text: rawMsg,
				},
			},
		})
	}

	return u.Answer(c, results, &td.AnswerInlineQueryOpts{CacheTime: 30})
}
