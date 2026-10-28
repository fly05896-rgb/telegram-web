/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package bot

import (
	"ashokshau/tg-web/internal/db"
	"ashokshau/tg-web/internal/utils"
	"ashokshau/tg-web/internal/webapp"
	"fmt"
	"html"
	"runtime"
	"strconv"
	"strings"
	"time"

	td "github.com/AshokShau/gotdbot"
)

// pingHandler handles the /ping command.
func pingHandler(c *td.Client, m *td.Message) error {
	deleteCmd(c, m)

	start := time.Now()

	msg, err := m.ReplyText(c, "Pinging… please wait…", nil)
	if err != nil {
		return err
	}

	latency := time.Since(start).Milliseconds()
	uptime := getFormattedDuration(time.Since(startTime))

	response := fmt.Sprintf(
		"<b>System Performance Metrics</b>\n\n"+
			"<b>Bot Latency:</b> <code>%d ms</code>\n"+
			"<b>Uptime:</b> <code>%s</code>\n"+
			"<b>Go Routines:</b> <code>%d</code>\n",
		latency, uptime, runtime.NumGoroutine(),
	)

	_, err = msg.EditText(c, response, &td.EditMessageTextOpts{ParseMode: "HTML"})
	return err
}

func saveSenderProfile(c *td.Client, m *td.Message) (int64, string, string, string) {
	senderID := m.SenderID()
	if senderID <= 0 || db.Instance == nil {
		return senderID, "User", "", ""
	}
	var fName, lName, uname string
	if u, err := c.GetUser(senderID); err == nil && u != nil {
		fName = u.FirstName
		lName = u.LastName
		if u.Usernames != nil {
			uname = u.Usernames.EditableUsername
			if uname == "" && len(u.Usernames.ActiveUsernames) > 0 {
				uname = u.Usernames.ActiveUsernames[0]
			}
		}
	}
	if fName == "" {
		fName = firstName(c, m)
	}
	_ = db.Instance.SaveUserProfile(senderID, fName, lName, uname)
	return senderID, fName, lName, uname
}

func parseStartRoomID(arg string, fallbackUserID int64) int64 {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 0
	}
	if strings.HasPrefix(arg, "room_") {
		arg = strings.TrimPrefix(arg, "room_")
	} else if strings.HasPrefix(arg, "player_") {
		arg = strings.TrimPrefix(arg, "player_")
	} else if strings.HasPrefix(arg, "pm_") {
		parts := strings.Split(arg, "_")
		if len(parts) >= 2 {
			if id, err := strconv.ParseInt(parts[1], 10, 64); err == nil && id != 0 {
				return id
			}
		}
	}
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil && id != 0 {
		return id
	}
	if arg == "web" || arg == "player" {
		return fallbackUserID
	}
	return 0
}

// startHandler handles the /start command.
func startHandler(c *td.Client, m *td.Message) error {
	chatID := m.ChatId
	go storeChatToDB(chatID)

	if m.IsPrivate() {
		userID, fName, lName, uname := saveSenderProfile(c, m)
		fullName := strings.TrimSpace(fName + " " + lName)
		if fullName == "" {
			fullName = "Listener"
		}
		botUsername := ""
		if c.Me() != nil && c.Me().Usernames != nil {
			botUsername = c.Me().Usernames.EditableUsername
		}

		startArg := Args(m)
		targetRoomID := parseStartRoomID(startArg, userID)

		go webapp.SendStartLog(c, userID, fullName, uname, targetRoomID)

		lang := "ar"
		if db.Instance != nil {
			if l, err := db.Instance.GetLanguage(userID); err == nil && l != "" {
				lang = l
			}
		}

		if targetRoomID != 0 {
			var text string
			if lang == "ar" {
				text = fmt.Sprintf(
					"<b>مرحباً بك يا %s في TeleMusic Web Player</b>\n\n"+
						"<b>الأيدي الخاص بك (للدخول من متصفح كروم):</b> <code>%d</code>\n"+
						"<b>معرف الغرفة:</b> <code>%d</code>\n\n"+
						"اضغط على الزر بالأسفل لفتح المشغل مباشرة داخل تيليجرام أو افتحه في متصفح كروم لتشغيل الصوت في الخلفية بدون انقطاع:",
					html.EscapeString(fullName),
					userID,
					targetRoomID,
				)
			} else {
				text = fmt.Sprintf(
					"<b>Welcome %s to TeleMusic Web Player</b>\n\n"+
						"<b>Your Telegram ID (for Chrome Login):</b> <code>%d</code>\n"+
						"<b>Room ID:</b> <code>%d</code>\n\n"+
						"Tap the button below to launch the Web Player inside Telegram or open it in Chrome for uninterrupted background audio:",
					html.EscapeString(fullName),
					userID,
					targetRoomID,
				)
			}
			_, err := m.ReplyText(c, text, &td.SendMessageOpts{
				ParseMode:   "HTML",
				ReplyMarkup: utils.RoomJoinMarkup(botUsername, targetRoomID, userID),
			})
			return err
		}

		var text string
		if lang == "ar" {
			text = fmt.Sprintf(
				"<b>مرحباً بك يا %s في %s</b>\n\n"+
					"<b>الأيدي الخاص بك لتسجيل الدخول من متصفح كروم:</b>\n<code>%d</code>\n\n"+
					"يمكنك استماع ومشاركة الموسيقى بجودة عالية عبر مشغل الويب في تيليجرام أو متصفح كروم بدون انقطاع للصوت في الخلفية.\n"+
					"استخدم الأزرار بالأسفل لفتح المشغل أو التحكم في الإعدادات عبر /settings.",
				html.EscapeString(fullName),
				html.EscapeString(c.Me().FirstName),
				userID,
			)
		} else {
			text = fmt.Sprintf(
				"<b>Welcome, %s!</b>\n\n"+
					"<b>Your Telegram ID for Chrome Web Login:</b>\n<code>%d</code>\n\n"+
					"<b>%s</b> lets you stream high-quality synchronized music via Telegram Web Player or Chrome browser with background playback.\n"+
					"Use the buttons below to open your player or configure settings via /settings.",
				html.EscapeString(fullName),
				userID,
				html.EscapeString(c.Me().FirstName),
			)
		}

		_, err := m.ReplyText(c, text, &td.SendMessageOpts{
			ParseMode:   "HTML",
			ReplyMarkup: utils.AddMeMarkup(botUsername, userID),
		})
		return err
	}

	deleteCmd(c, m)

	uptime := getFormattedDuration(time.Since(startTime))
	botUsername := ""
	if c.Me() != nil && c.Me().Usernames != nil {
		botUsername = c.Me().Usernames.EditableUsername
	}
	text := fmt.Sprintf(
		"<b>%s is ready!</b>\n"+
			"<b>Uptime:</b> <code>%s</code>\n"+
			"<b>Group Room ID:</b> <code>%d</code>",
		html.EscapeString(c.Me().FirstName),
		uptime,
		chatID,
	)

	_, err := m.ReplyText(c, text, &td.SendMessageOpts{
		ParseMode:   "HTML",
		ReplyMarkup: utils.WebAppControlButtons("webapp", botUsername, chatID),
	})
	return err
}
