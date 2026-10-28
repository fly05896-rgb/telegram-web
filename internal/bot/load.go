/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package bot

import (
	"ashokshau/tg-web/internal/messaging"
	"strings"
	"time"

	"github.com/AshokShau/gotdbot"
	"github.com/AshokShau/gotdbot/filters/callbackquery"
)

var startTime = time.Now()

func LoadModules(c *gotdbot.Client, cancelCleanup func(int64)) {
	register := func(name string, handler func(*gotdbot.Client, *gotdbot.Message) error) {
		c.OnCommand(name, handler)
		if messaging.SupportsSilence(name) {
			c.OnCommand("s"+name, handler)
		}
	}

	register("reload", reloadAdminCacheHandler)
	register("authList", authListHandler)
	register("auths", authListHandler)
	register("auth", addAuthHandler)
	register("addAuth", addAuthHandler)
	register("removeAuth", removeAuthHandler)
	register("rmAuth", removeAuthHandler)
	register("broadcast", broadcastHandler)
	register("gCast", broadcastHandler)
	register("stop_gcast", cancelBroadcastHandler)
	register("stop_broadcast", cancelBroadcastHandler)
	register("av", activeVcHandler)
	register("activevc", activeVcHandler)
	register("logger", loggerHandler)
	register("privacy", privacyHandler)
	register("autoplay", autoplayHandler)
	register("loop", loopHandler)
	register("pause", pauseHandler)
	register("resume", resumeHandler)
	register("cplist", createPlaylistHandler)
	register("createplaylist", createPlaylistHandler)
	register("deleteplaylist", deletePlaylistHandler)
	register("queue", queueHandler)
	register("seek", seekHandler)
	register("cseek", seekHandler)
	register("seekback", seekBackHandler)
	register("cseekback", seekBackHandler)
	register("sh", shellCommand)
	register("skip", skipHandler)
	register("cskip", skipHandler)
	register("next", skipHandler)
	register("cnext", skipHandler)
	register("stop", stopHandler)
	register("end", stopHandler)
	register("start", startHandler)
	register("help", startHandler)
	register("ping", pingHandler)
	register("mix", mixHandler)
	register("play", playHandler)
	register("vplay", playHandler)
	register("cplay", playHandler)
	register("cvplay", playHandler)
	register("p", playHandler)
	register("fplay", fPlayHandler)
	register("playforce", fPlayHandler)
	register("vplayforce", fPlayHandler)
	register("fp", fPlayHandler)
	register("remove", removeHandler)
	register("settings", settingsHandler)
	register("lang", langCommandHandler)
	register("language", langCommandHandler)
	register("player", webAppCommandHandler)
	register("addtoplaylist", addToPlaylistHandler)
	register("addtoplist", addToPlaylistHandler)
	register("removefromplaylist", removeFromPlaylistHandler)
	register("rmplist", removeFromPlaylistHandler)
	register("plistinfo", playlistInfoHandler)
	register("playlistinfo", playlistInfoHandler)
	register("myplaylists", myPlaylistsHandler)
	register("myplist", myPlaylistsHandler)
	register("stats", statsHandler)

	// Arabic prefix-less & slash commands support (ported from play.py, skip.py, seek.py)
	c.OnMessage(arabicCommandRouter, func(m *gotdbot.Message) bool {
		if m == nil {
			return false
		}
		txt := strings.TrimSpace(m.Text())
		if txt == "" || strings.HasPrefix(txt, "/") {
			return false
		}
		return true
	})

	c.AddUpdateNewCallbackQueryHandlerGroup(recordTelegramChatContext, nil, -1)
	c.OnUpdateNewCallbackQuery(webAppVerifyCallbackHandler, callbackquery.Prefix("webapp_verify"))
	c.OnUpdateNewCallbackQuery(helpCallbackHandler, callbackquery.Prefix("help_"))
	c.OnUpdateNewCallbackQuery(playCallbackHandler, callbackquery.Prefix("play_"))
	c.OnUpdateNewCallbackQuery(func(client *gotdbot.Client, cb *gotdbot.UpdateNewCallbackQuery) error {
		return settingsCallbackHandler(client, cb, cancelCleanup)
	}, callbackquery.Prefix("settings_"))
	c.OnUpdateNewCallbackQuery(autoplayCallbackHandler, callbackquery.Equal("autoplay_toggle"))

	c.OnUpdateChatMember(handleParticipant, nil)
	c.OnUpdateNewGuestQuery(guestHandler, nil)
	c.OnUpdateNewInlineQuery(inlineQueryHandler, nil)

	c.Logger.Debug("Handlers loaded successfully")
}

func arabicCommandRouter(c *gotdbot.Client, m *gotdbot.Message) error {
	raw := strings.TrimSpace(m.Text())
	raw = strings.TrimLeft(raw, "!.#")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	switch raw {
	case "قفل البحث", "تعطيل البحث":
		if isDev(c, m) {
			setSearchLocked(true)
			_, _ = m.ReplyText(c, "<b>تم قفل البحث بنجاح .</b>", &gotdbot.SendMessageOpts{ParseMode: "HTML"})
		}
		return gotdbot.EndGroups
	case "فتح البحث", "تفعيل البحث":
		if isDev(c, m) {
			setSearchLocked(false)
			_, _ = m.ReplyText(c, "<b>تم فتح البحث بنجاح .</b>", &gotdbot.SendMessageOpts{ParseMode: "HTML"})
		}
		return gotdbot.EndGroups
	case "تخطي", "التالي":
		return skipHandler(c, m)
	case "ايقاف", "إيقاف", "انهاء", "إنهاء", "اسكت":
		return stopHandler(c, m)
	case "مؤقت", "توقف":
		return pauseHandler(c, m)
	case "كمل", "استئناف", "استمر":
		return resumeHandler(c, m)
	case "المشغل", "مشغل":
		return webAppCommandHandler(c, m)
	case "الاعدادات", "الإعدادات", "اعدادات":
		return settingsHandler(c, m)
	case "اللغة", "لغه", "لغة":
		return langCommandHandler(c, m)
	case "بينج", "بنج":
		return pingHandler(c, m)
	}

	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return nil
	}
	cmd := fields[0]
	switch cmd {
	case "شغل", "تشغيل", "فيد", "فيديو":
		return playHandler(c, m)
	case "تخطي", "التالي":
		return skipHandler(c, m)
	case "مرر", "قدم":
		return seekHandler(c, m)
	case "رجع":
		return seekBackHandler(c, m)
	case "تكرار":
		return loopHandler(c, m)
	}
	return nil
}
