package utils

import (
	"ashokshau/tg-web/internal/config"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/AshokShau/gotdbot"
)

func cb(text, data string, style gotdbot.ButtonStyle) gotdbot.InlineKeyboardButton {
	return gotdbot.InlineKeyboardButton{
		Text: text,
		Type: &gotdbot.InlineKeyboardButtonTypeCallback{
			Data: []byte(data),
		},
		Style: style,
	}
}

func url(text, link string, style gotdbot.ButtonStyle) gotdbot.InlineKeyboardButton {
	return gotdbot.InlineKeyboardButton{
		Text: text,
		Type: &gotdbot.InlineKeyboardButtonTypeUrl{
			Url: link,
		},
		Style: style,
	}
}

func webAppNativeBtn(text, link string, style gotdbot.ButtonStyle) gotdbot.InlineKeyboardButton {
	return gotdbot.InlineKeyboardButton{
		Text: text,
		Type: &gotdbot.InlineKeyboardButtonTypeWebApp{
			Url: link,
		},
		Style: style,
	}
}

var CloseBtn = cb("Close", "play_close", gotdbot.ButtonStyleDanger{})
var HomeBtn = cb("Home", "help_back", gotdbot.ButtonStylePrimary{})
var HelpBtn = cb("Help", "help_all", gotdbot.ButtonStyleDefault{})
var UserBtn = cb("Users", "help_user", gotdbot.ButtonStyleDefault{})
var AdminBtn = cb("Admins", "help_admin", gotdbot.ButtonStyleDefault{})
var OwnerBtn = cb("Owner", "help_owner", gotdbot.ButtonStyleDefault{})
var DevsBtn = cb("Devs", "help_devs", gotdbot.ButtonStyleDefault{})
var PlaylistBtn = cb("Playlist", "help_playlist", gotdbot.ButtonStyleDefault{})
var AutoplayBtn = cb("Autoplay", "help_autoplay", gotdbot.ButtonStyleDefault{})

var channelBtn = url("Updates", config.SupportChannel, gotdbot.ButtonStyleDefault{})
var groupBtn = url("Group", config.SupportGroup, gotdbot.ButtonStyleDefault{})

func SupportKeyboard() *gotdbot.ReplyMarkupInlineKeyboard {
	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{
			{channelBtn, groupBtn},
			{CloseBtn},
		},
	}
}

func SettingsKeyboard(playMode, adminMode string, cmdDelete bool, language string, autoplay bool, chatEnabled bool) *gotdbot.ReplyMarkupInlineKeyboard {
	isAr := strings.EqualFold(language, "ar")

	playText := "Everyone"
	if isAr {
		playText = "الجميع"
	}
	if playMode == Admins {
		playText = "Admins"
		if isAr {
			playText = "المشرفين"
		}
	}

	deleteText := "False"
	if isAr {
		deleteText = "معطل"
	}
	if cmdDelete {
		deleteText = "True"
		if isAr {
			deleteText = "مفعل"
		}
	}

	adminText := "Everyone"
	if isAr {
		adminText = "الجميع"
	}
	if adminMode == Admins {
		adminText = "Admins"
		if isAr {
			adminText = "المشرفين"
		}
	}

	langText := "English"
	if isAr {
		langText = "العربية"
	}

	autoplayText := "Disabled"
	if isAr {
		autoplayText = "معطل"
	}
	if autoplay {
		autoplayText = "Enabled"
		if isAr {
			autoplayText = "مفعل"
		}
	}

	chatText := "Disabled"
	if isAr {
		chatText = "معطل"
	}
	if chatEnabled {
		chatText = "Enabled"
		if isAr {
			chatText = "مفعل"
		}
	}

	lblPlay := "Play Mode ➜"
	lblDel := "Command Delete ➜"
	lblAdmin := "Admin Mode ➜"
	lblAuto := "Autoplay ➜"
	lblChat := "Chat Mode ➜"
	lblLang := "Language ➜"
	if isAr {
		lblPlay = "وضع التشغيل ➜"
		lblDel = "حذف الأوامر ➜"
		lblAdmin = "وضع التحكم ➜"
		lblAuto = "التشغيل التلقائي ➜"
		lblChat = "دردشة الغرفة ➜"
		lblLang = "اللغة / Language ➜"
	}

	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{
			{
				cb(lblPlay, "settings_main", gotdbot.ButtonStyleDefault{}),
				cb(playText, "settings_play", gotdbot.ButtonStyleDefault{}),
			},
			{
				cb(lblDel, "settings_main", gotdbot.ButtonStyleDefault{}),
				cb(deleteText, "settings_delete", gotdbot.ButtonStyleDefault{}),
			},
			{
				cb(lblAdmin, "settings_main", gotdbot.ButtonStyleDefault{}),
				cb(adminText, "settings_admin", gotdbot.ButtonStyleDefault{}),
			},
			{
				cb(lblAuto, "settings_main", gotdbot.ButtonStyleDefault{}),
				cb(autoplayText, "settings_autoplay", gotdbot.ButtonStyleDefault{}),
			},
			{
				cb(lblChat, "settings_main", gotdbot.ButtonStyleDefault{}),
				cb(chatText, "settings_chat", gotdbot.ButtonStyleDefault{}),
			},
			{
				cb(lblLang, "settings_main", gotdbot.ButtonStyleDefault{}),
				cb(langText, "settings_lang", gotdbot.ButtonStylePrimary{}),
			},
			{
				cb("العربية", "settings_setlang_ar", gotdbot.ButtonStyleDefault{}),
				cb("English", "settings_setlang_en", gotdbot.ButtonStyleDefault{}),
			},
			{CloseBtn},
		},
	}
}

func HelpMenuKeyboard() *gotdbot.ReplyMarkupInlineKeyboard {
	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{
			{UserBtn, AdminBtn, OwnerBtn},
			{PlaylistBtn, DevsBtn, AutoplayBtn},
			{HomeBtn, CloseBtn},
		},
	}
}

func BackHelpMenuKeyboard() *gotdbot.ReplyMarkupInlineKeyboard {
	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{
			{HelpBtn, HomeBtn},
			{CloseBtn},
		},
	}
}

func AddMeMarkup(username string, userID ...int64) *gotdbot.ReplyMarkupInlineKeyboard {
	addMeBtn := url(
		"Add Me To Your Group",
		fmt.Sprintf("https://t.me/%s?startgroup=true", username),
		gotdbot.ButtonStylePrimary{},
	)

	var rows [][]gotdbot.InlineKeyboardButton
	if len(userID) > 0 && userID[0] > 0 {
		rows = append(rows, []gotdbot.InlineKeyboardButton{webAppButton(username, userID[0])})
	}
	rows = append(rows,
		[]gotdbot.InlineKeyboardButton{addMeBtn},
		[]gotdbot.InlineKeyboardButton{HelpBtn},
		[]gotdbot.InlineKeyboardButton{channelBtn, groupBtn},
	)

	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: rows,
	}
}

func RoomJoinMarkup(botUsername string, roomID, userID int64) *gotdbot.ReplyMarkupInlineKeyboard {
	var rows [][]gotdbot.InlineKeyboardButton
	baseURL := strings.TrimRight(config.WebAppURL, "/")
	if baseURL != "" {
		webAppTarget := fmt.Sprintf("%s/room?startapp=%d&chat_id=%d&room=%d&uid=%d", baseURL, roomID, roomID, roomID, userID)
		rows = append(rows, []gotdbot.InlineKeyboardButton{
			webAppNativeBtn("Open TeleMusic Web Player", webAppTarget, gotdbot.ButtonStylePrimary{}),
		})
	} else {
		rows = append(rows, []gotdbot.InlineKeyboardButton{
			webAppButton(botUsername, roomID),
		})
	}
	return &gotdbot.ReplyMarkupInlineKeyboard{Rows: rows}
}

func PlayNowButton(trackID string) gotdbot.InlineKeyboardButton {
	return cb("Play Now", PlayNowCallbackData(trackID), gotdbot.ButtonStyleDanger{})
}

func PlayNowCallbackData(trackID string) string {
	data := "play_now_" + trackID
	if len(data) <= 64 {
		return data
	}
	hash := sha256.Sum256([]byte(trackID))
	return "play_now_hash_" + base64.RawURLEncoding.EncodeToString(hash[:])
}

func BuildBotStartPlayerLink(botUsername string, roomID int64) string {
	return fmt.Sprintf("https://t.me/%s?start=room_%s", strings.TrimPrefix(botUsername, "@"), strconv.FormatInt(roomID, 10))
}

func BuildChromeRoomLink(roomID int64) string {
	return fmt.Sprintf("Room ID: %d", roomID)
}

func webAppButton(botUsername string, chatID int64) gotdbot.InlineKeyboardButton {
	baseURL := strings.TrimRight(config.WebAppURL, "/")
	if chatID > 0 && baseURL != "" {
		directRoomURL := fmt.Sprintf("%s/room?startapp=%d&chat_id=%d&room=%d&uid=%d", baseURL, chatID, chatID, chatID, chatID)
		return webAppNativeBtn("Web Player", directRoomURL, gotdbot.ButtonStylePrimary{})
	}
	return url("Web Player", BuildBotStartPlayerLink(botUsername, chatID), gotdbot.ButtonStylePrimary{})
}

func QueueMarkup(trackID, botUsername string, chatId int64) *gotdbot.ReplyMarkupInlineKeyboard {
	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{
			{PlayNowButton(trackID), webAppButton(botUsername, chatId)},
		},
	}
}

func WebAppControlButtons(mode string, botUsername string, chatID int64) *gotdbot.ReplyMarkupInlineKeyboard {
	webAppBtn := webAppButton(botUsername, chatID)
	skipBtn := cb("‣‣I", "play_skip", gotdbot.ButtonStyleDefault{})
	stopBtn := cb("▢", "play_stop", gotdbot.ButtonStyleDefault{})
	pauseBtn := cb("II", "play_pause", gotdbot.ButtonStyleDefault{})
	resumeBtn := cb("▷", "play_resume", gotdbot.ButtonStyleDefault{})

	switch mode {
	case "play", "resume":
		return &gotdbot.ReplyMarkupInlineKeyboard{
			Rows: [][]gotdbot.InlineKeyboardButton{
				{skipBtn, stopBtn, pauseBtn},
				{webAppBtn},
			},
		}
	case "pause":
		return &gotdbot.ReplyMarkupInlineKeyboard{
			Rows: [][]gotdbot.InlineKeyboardButton{
				{skipBtn, stopBtn, resumeBtn},
				{webAppBtn},
			},
		}
	default:
		return &gotdbot.ReplyMarkupInlineKeyboard{
			Rows: [][]gotdbot.InlineKeyboardButton{
				{webAppBtn},
			},
		}
	}
}
