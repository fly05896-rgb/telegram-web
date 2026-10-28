/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package bot

import (
	"ashokshau/tg-web/internal/cache"
	"ashokshau/tg-web/internal/messaging"
	"ashokshau/tg-web/internal/utils"
	"ashokshau/tg-web/internal/webapp"
	"fmt"
	"strconv"

	td "github.com/AshokShau/gotdbot"
)

func seekHandler(c *td.Client, m *td.Message) error {
	return handleSeekDirection(c, m, false)
}

func seekBackHandler(c *td.Client, m *td.Message) error {
	return handleSeekDirection(c, m, true)
}

func handleSeekDirection(c *td.Client, m *td.Message, backward bool) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}
	chatID := m.ChatId

	if !cache.ChatCache.IsActive(chatID) {
		_, err := m.ReplyText(c, "The bot is not streaming.", nil)
		return err
	}

	playingSong := cache.ChatCache.GetPlayingTrack(chatID)
	if playingSong == nil {
		_, err := m.ReplyText(c, "The bot is not streaming.", nil)
		return err
	}

	args := Args(m)
	if args == "" {
		_, _ = m.ReplyText(c, "<b>Usage:</b> /seek [seconds] or /seekback [seconds]\n<b>Example:</b> <code>/seek 15</code>", replyOpts)
		return nil
	}

	seekTime, err := strconv.Atoi(args)
	if err != nil {
		_, _ = m.ReplyText(c, "Invalid seek time provided. Please use a valid number of seconds.", nil)
		return nil
	}

	if seekTime < 10 {
		_, _ = m.ReplyText(c, "Minimum seek time is 10 seconds.", nil)
		return nil
	}

	currDurInt, err := webapp.Manager.PlayedTime(c, chatID)
	if err != nil {
		_, _ = m.ReplyText(c, "Failed to fetch the duration of the ongoing stream.", nil)
		return nil
	}

	currDur := int32(currDurInt)
	var toSeek int32
	if backward {
		if currDur-int32(seekTime) <= 5 {
			toSeek = 0
		} else {
			toSeek = currDur - int32(seekTime)
		}
	} else {
		toSeek = currDur + int32(seekTime)
		if playingSong.Duration > 0 && toSeek >= playingSong.Duration {
			_, _ = m.ReplyText(c, fmt.Sprintf("You cannot seek beyond the track duration. Maximum allowed is %s.", utils.SecToMin(playingSong.Duration)), nil)
			return nil
		}
	}

	if _, err = webapp.Manager.SeekRoom(c, chatID, float64(toSeek)); err != nil {
		_, _ = m.ReplyText(c, "An error occurred while seeking the track: Please try again later.", replyOpts)
		return nil
	}

	_, _ = messaging.Reply(c, m, fmt.Sprintf("<b>Stream seeked to %s by</b> %s", utils.SecToMin(toSeek), firstName(c, m)), replyOpts)
	return nil
}
