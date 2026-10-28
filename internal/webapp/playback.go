/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package webapp

import (
	"ashokshau/tg-web/internal/cache"
	"ashokshau/tg-web/internal/config"
	"ashokshau/tg-web/internal/db"
	"ashokshau/tg-web/internal/downloader"
	"ashokshau/tg-web/internal/messaging"
	"ashokshau/tg-web/internal/utils"
	"context"
	"fmt"
	"html"
	"log/slog"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"time"

	td "github.com/AshokShau/gotdbot"
	tgwebapp "github.com/AshokShau/gotdbot/webapp"
)

var (
	chromeNoticeMu   sync.Mutex
	chromeNoticeSent = make(map[int64]time.Time)
)

func isLoggerActive() bool {
	if config.LoggerId == 0 {
		return false
	}
	if db.Instance == nil {
		return true
	}
	return db.Instance.GetLoggerStatus()
}

func formatUserMention(userID int64, fullName, username string) string {
	if fullName == "" {
		fullName = "User"
	}
	escName := html.EscapeString(fullName)
	mention := fmt.Sprintf("<a href=\"tg://user?id=%d\">%s</a>", userID, escName)
	if username != "" {
		mention += fmt.Sprintf(" (@%s)", html.EscapeString(strings.TrimPrefix(username, "@")))
	}
	return mention
}

// SendStartLog logs when a user sends /start to the bot in Telegram.
func SendStartLog(bot *td.Client, userID int64, fullName, username string, roomID int64) {
	if bot == nil || !isLoggerActive() {
		return
	}
	roomText := "Personal Start"
	if roomID != 0 {
		roomText = fmt.Sprintf("<code>%d</code>", roomID)
	}
	text := fmt.Sprintf(
		"<b>[سجل دخول البوت - Bot Start Log]</b>\n\n"+
			"<b>المستخدم:</b> %s\n"+
			"<b>الأيدي (ID):</b> <code>%d</code>\n"+
			"<b>اليوزر:</b> @%s\n"+
			"<b>المشغل / الغرفة:</b> %s",
		formatUserMention(userID, fullName, username),
		userID,
		html.EscapeString(strings.TrimPrefix(username, "@")),
		roomText,
	)
	_, _ = bot.SendMessage(config.LoggerId, text, &td.SendMessageOpts{
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	})
}

// SendChromeSessionNotice sends a message from the bot to the user on Telegram when they log in from Chrome.
func SendChromeSessionNotice(bot *td.Client, user *tgwebapp.WebAppUser, roomID int64) {
	if bot == nil || user == nil || user.ID <= 0 {
		return
	}
	chromeNoticeMu.Lock()
	last, exists := chromeNoticeSent[user.ID]
	if exists && time.Since(last) < 2*time.Minute {
		chromeNoticeMu.Unlock()
		return
	}
	chromeNoticeSent[user.ID] = time.Now()
	chromeNoticeMu.Unlock()

	fullName := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if fullName == "" {
		fullName = "Listener"
	}
	roomLink := utils.BuildChromeRoomLink(roomID)
	text := fmt.Sprintf(
		"<b>تم ربط مشغل متصفح كروم بنجاح</b>\n\n"+
			"<b>المستخدم:</b> %s\n"+
			"<b>الأيدي:</b> <code>%d</code>\n"+
			"<b>معرف الغرفة / المشغل:</b> <code>%d</code>\n"+
			"<b>رابط المشغل:</b> %s",
		html.EscapeString(fullName),
		user.ID,
		roomID,
		html.EscapeString(roomLink),
	)
	_, _ = bot.SendMessage(user.ID, text, &td.SendMessageOpts{
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	})
}

// SendChromeLoginLog logs Chrome browser logins to the Logger Group (LOGGER_ID).
func SendChromeLoginLog(bot *td.Client, user *tgwebapp.WebAppUser, roomID int64) {
	if bot == nil || !isLoggerActive() || user == nil {
		return
	}
	fullName := strings.TrimSpace(user.FirstName + " " + user.LastName)
	uname := user.Username
	if uname == "" {
		uname = "بدون_يوزر"
	}
	text := fmt.Sprintf(
		"<b>[سجل دخول متصفح كروم - Chrome Player Login]</b>\n\n"+
			"<b>المستخدم:</b> %s\n"+
			"<b>الأيدي (ID):</b> <code>%d</code>\n"+
			"<b>اليوزر:</b> @%s\n"+
			"<b>المشغل (Room ID):</b> <code>%d</code>\n"+
			"<b>المنصة:</b> Chrome Web Player",
		formatUserMention(user.ID, fullName, user.Username),
		user.ID,
		html.EscapeString(strings.TrimPrefix(uname, "@")),
		roomID,
	)
	_, _ = bot.SendMessage(config.LoggerId, text, &td.SendMessageOpts{
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	})
}

// SendPlayLog sends a detailed playback log to the Logger Group (LOGGER_ID) with ID, Player/Room, Username, and Track.
func SendPlayLog(bot *td.Client, chatID int64, userID int64, username string, playerSource string, song *utils.PlayerCache) {
	if bot == nil || !isLoggerActive() || song == nil {
		return
	}
	if playerSource == "" {
		playerSource = "TeleMusic Web Player"
	}
	unameDisplay := "غير متوفر"
	if username != "" {
		unameDisplay = "@" + html.EscapeString(strings.TrimPrefix(username, "@"))
	}
	userIDDisplay := "غير محدد"
	if userID > 0 {
		userIDDisplay = fmt.Sprintf("<code>%d</code>", userID)
	}
	title := html.EscapeString(song.Name)
	if strings.TrimSpace(song.URL) != "" {
		title = fmt.Sprintf("<a href='%s'>%s</a>", html.EscapeString(song.URL), title)
	}
	text := fmt.Sprintf(
		"<b>[سجل التشغيل - TeleMusic Playback Log]</b>\n\n"+
			"<b>الأغنية:</b> %s\n"+
			"<b>المدة:</b> <code>%s</code>\n"+
			"<b>المشغل / الغرفة (Room ID):</b> <code>%d</code>\n"+
			"<b>نوع المشغل:</b> %s\n"+
			"<b>اسم المشغل (User):</b> %s\n"+
			"<b>الأيدي (User ID):</b> %s\n"+
			"<b>اليوزر (Username):</b> %s",
		title,
		utils.SecToMin(song.Duration),
		chatID,
		html.EscapeString(playerSource),
		html.EscapeString(song.User),
		userIDDisplay,
		unameDisplay,
	)
	_, _ = bot.SendMessage(config.LoggerId, text, &td.SendMessageOpts{
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	})
}

// PlayNext plays the next song in the queue or handles loop/autoplay.
func PlayNext(bot *td.Client, chatID int64, quiet ...bool) error {
	return PlayNextForTrack(bot, chatID, "", quiet...)
}

// PlayNextForTrack plays the next song in the queue if the event matches fromTrackID.
func PlayNextForTrack(bot *td.Client, chatID int64, fromTrackID string, quiet ...bool) error {
	room := Manager.getOrCreate(bot, chatID)
	room.mu.Lock()

	if room.isTransitioning {
		room.mu.Unlock()
		return nil
	}

	if fromTrackID != "" && room.currentTrackID != "" && room.currentTrackID != fromTrackID {
		room.mu.Unlock()
		return nil
	}

	room.isTransitioning = true
	room.cancelTrackEndTimerLocked()

	if fromTrackID != "" {
		current := cache.ChatCache.GetPlayingTrack(chatID)
		if current != nil && current.Silent {
			quiet = []bool{true}
		}
	}
	loop := cache.ChatCache.GetLoopCount(chatID)
	if loop > 0 {
		cache.ChatCache.SetLoopCount(chatID, loop-1)
		if currentsSong := cache.ChatCache.GetPlayingTrack(chatID); currentsSong != nil {
			room.currentTrackID = currentsSong.TrackID
			room.Status = "playing"
			room.Position = 0
			room.ServerTime = time.Now().UnixMilli()
			room.isTransitioning = false
			room.mu.Unlock()
			return PlayTrack(bot, chatID, currentsSong, quiet...)
		}
	}

	finishedTrack := cache.ChatCache.RemoveCurrentSong(chatID)
	if nextSong := cache.ChatCache.GetPlayingTrack(chatID); nextSong != nil {
		room.currentTrackID = nextSong.TrackID
		room.Status = "playing"
		room.Position = 0
		room.ServerTime = time.Now().UnixMilli()
		room.isTransitioning = false
		room.mu.Unlock()
		return PlayTrack(bot, chatID, nextSong, quiet...)
	}

	lastTrack := cache.ChatCache.GetLastAutoplayTrackID(chatID)
	if finishedTrack != nil && finishedTrack.Platform == utils.Telegram {
		lastTrack = strings.TrimSpace(finishedTrack.Name + " " + finishedTrack.Channel)
	}
	isAutoplay := cache.ChatCache.GetAutoplay(chatID)
	room.mu.Unlock()

	if lastTrack != "" && isAutoplay {
		return handleAutoplay(bot, chatID, lastTrack, quiet...)
	}

	return handleNoSong(bot, chatID, quiet...)
}

func handleAutoplay(bot *td.Client, chatID int64, lastTrack string, quiet ...bool) error {
	history := cache.ChatCache.GetAutoplayHistory(chatID)
	if len(history) >= int(config.AutoPlayLimit) {
		cache.ChatCache.ClearAutoplayHistory(chatID)
		return handleNoSong(bot, chatID, quiet...)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	limit := int(config.AutoPlayLimit)
	tracks, err := downloader.GetRecommendations(ctx, lastTrack, limit)
	if err != nil {
		_, _ = bot.SendMessage(chatID, "Unable to load recommended tracks. Try /play to add another song.", nil)
		return handleNoSong(bot, chatID, true)
	}
	if len(tracks) == 0 {
		return handleNoSong(bot, chatID, quiet...)
	}

	var candidates []utils.GetUrlTrack
	for _, track := range tracks {
		if track.Id == lastTrack || track.Url == lastTrack ||
			slices.Contains(history, track.Id) || slices.Contains(history, track.Url) {
			continue
		}

		candidates = append(candidates, track)
	}

	if len(candidates) == 0 {
		return handleNoSong(bot, chatID, quiet...)
	}

	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	nextTrack := candidates[0]
	trackKey := nextTrack.Url
	if trackKey == "" {
		trackKey = nextTrack.Id
	}
	cache.ChatCache.AddAutoplayHistory(chatID, lastTrack, trackKey)

	platform := nextTrack.Platform
	if platform == "" {
		platform = utils.YouTube
	}

	saveCache := &utils.PlayerCache{
		Silent:    isQuiet(quiet),
		URL:       nextTrack.Url,
		Name:      nextTrack.Title,
		User:      utils.AutoPlay,
		Thumbnail: nextTrack.Thumbnail,
		TrackID:   nextTrack.Id,
		Duration:  nextTrack.Duration,
		Channel:   nextTrack.Channel,
		Views:     nextTrack.Views,
		Platform:  platform,
	}

	cache.ChatCache.AddSong(chatID, saveCache)

	room := Manager.getOrCreate(bot, chatID)
	room.mu.Lock()
	room.currentTrackID = saveCache.TrackID
	room.Status = "playing"
	room.Position = 0
	room.ServerTime = time.Now().UnixMilli()
	room.mu.Unlock()

	return PlayTrack(bot, chatID, saveCache, quiet...)
}

func StopPlayback(c *td.Client, chatID int64) {
	cache.ChatCache.ClearChat(chatID)
	Manager.Stop(c, chatID)
}

func handleNoSong(bot *td.Client, chatID int64, quiet ...bool) error {
	room := Manager.getOrCreate(bot, chatID)
	room.mu.Lock()
	alreadyStopped := room.Status == "stopped"
	room.Status = "stopped"
	room.currentTrackID = ""
	room.isTransitioning = false
	room.mu.Unlock()

	StopPlayback(bot, chatID)

	if !alreadyStopped {
		_, _ = messaging.Send(bot, chatID, "Queue finished. Add more songs with /play.", nil, isQuiet(quiet))
	}
	return nil
}

func PlayTrack(bot *td.Client, chatID int64, song *utils.PlayerCache, quiet ...bool) error {
	if song == nil {
		return nil
	}
	reply, err := messaging.Send(bot, chatID, fmt.Sprintf("Downloading %s...", song.Name), nil, song.Silent || isQuiet(quiet))
	if err != nil {
		slog.Info("[PlayTrack] Failed to send message", "error", err)
		return PlayTrackWithMessage(bot, &td.Message{ChatId: chatID}, chatID, song)
	}

	return PlayTrackWithMessage(bot, reply, chatID, song)
}

func PlayTrackWithMessage(bot *td.Client, reply *td.Message, chatID int64, song *utils.PlayerCache) error {
	if song == nil {
		return nil
	}
	value := *song
	song = &value
	room := Manager.getOrCreate(bot, chatID)
	room.mu.Lock()
	current := cache.ChatCache.GetPlayingTrack(chatID)
	if current == nil || current.TrackID != song.TrackID {
		room.mu.Unlock()
		if reply != nil && reply.Id != 0 {
			_ = reply.Delete(bot, &td.DeleteMessagesOpts{Revoke: true})
		}
		return nil
	}
	room.currentTrackID = song.TrackID
	room.cancelTrackEndTimerLocked()
	room.Status = "loading"
	room.Position = 0
	room.ServerTime = time.Now().UnixMilli()
	room.isTransitioning = false
	room.mu.Unlock()
	HubInstance.BroadcastRoomState(bot, chatID)

	if song.FilePath == "" {
		dlPath, err := downloader.DlCachedTrack(song, bot)
		song.FilePath = dlPath
		if err != nil || song.FilePath == "" {
			if reply != nil && reply.Id != 0 {
				_, _ = reply.EditText(bot, "Download failed. Skipping track...", nil)
			} else if reply != nil && reply.ChatId != 0 {
				_, _ = bot.SendMessage(reply.ChatId, "Download failed. Skipping track...", nil)
			}
			room.mu.Lock()
			room.isTransitioning = false
			room.mu.Unlock()
			return PlayNextForTrack(bot, chatID, song.TrackID, reply == nil || reply.Id == 0)
		}
	}

	if song.Duration == 0 && !strings.HasPrefix(song.FilePath, "http") {
		song.Duration = utils.GetMediaDuration(song.FilePath)
	}

	if !cache.ChatCache.UpdatePlayingMedia(chatID, song.TrackID, song.FilePath, song.Duration) {
		if reply != nil && reply.Id != 0 {
			_ = reply.Delete(bot, &td.DeleteMessagesOpts{Revoke: true})
		}
		return nil
	}

	go func() {
		queue := cache.ChatCache.GetQueue(chatID)
		if len(queue) > 1 && queue[1] != nil && queue[1].FilePath == "" {
			if path, err := downloader.DlCachedTrack(queue[1], bot); err == nil && path != "" {
				queue[1].FilePath = path
			}
		}
	}()

	title := html.EscapeString(song.Name)
	if strings.TrimSpace(song.URL) != "" {
		title = fmt.Sprintf("<a href='%s'>%s</a>", html.EscapeString(song.URL), title)
	}
	text := fmt.Sprintf(
		"<u><b>| Started streaming</b></u>\n\n"+
			"<b>Title:</b> %s\n\n"+
			"<b>Duration:</b> %s min\n"+
			"<b>Requested by:</b> %s",
		title,
		utils.SecToMin(song.Duration),
		html.EscapeString(song.User),
	)

	Manager.PlayTrack(bot, chatID, song)
	if reply == nil || reply.Id == 0 {
		return nil
	}
	botUsername := ""
	if bot != nil && bot.Me() != nil && bot.Me().Usernames != nil {
		botUsername = bot.Me().Usernames.EditableUsername
	}
	markup := WebAppControlButtons("play", botUsername, chatID)

	if _, err := messaging.FinishProgress(bot, reply, text, &td.EditMessageTextOpts{
		ReplyMarkup:           markup,
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	}, song.Silent); err != nil {
		bot.Logger.Error("Failed to update playback message", "chatID", chatID, "error", err)
		return err
	}

	return nil
}

func isQuiet(values []bool) bool { return len(values) > 0 && values[0] }
