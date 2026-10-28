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
	"ashokshau/tg-web/internal/db"
	"ashokshau/tg-web/internal/messaging"
	"ashokshau/tg-web/internal/utils"
	"ashokshau/tg-web/internal/webapp"
	"fmt"
	"html"
	"strconv"
	"strings"

	td "github.com/AshokShau/gotdbot"
)

func renderSettingsMessageText(title string, chatID int64, lang string) string {
	if strings.EqualFold(lang, "ar") {
		return fmt.Sprintf("<u><b>إعدادات %s</b></u>\n<b>معرف الغرفة:</b> <code>%d</code>\n\nاضغط على الأزرار بالأسفل لتعديل الإعدادات واللغة (العربية / English):",
			html.EscapeString(title), chatID)
	}
	return fmt.Sprintf("<u><b>%s settings</b></u>\n<b>Room ID:</b> <code>%d</code>\n\nClick the buttons below to change this room's current settings and language.",
		html.EscapeString(title), chatID)
}

func settingsHandler(c *td.Client, m *td.Message) error {
	chatID := m.ChatId
	isPrivate := m.IsPrivate()

	var title string
	if isPrivate {
		_, fName, lName, _ := saveSenderProfile(c, m)
		title = strings.TrimSpace(fName + " " + lName)
		if title == "" {
			title = "Personal Room"
		}
	} else {
		chatInfo, chatErr := m.GetChat(c)
		if chatErr != nil || !messaging.IsGroup(chatInfo) {
			return nil
		}
		title = chatInfo.Title
		deleteCmd(c, m)

		if accessErr := cache.SettingsAccess(c, chatID, m.SenderID(), "main"); accessErr != nil {
			_, _ = m.ReplyText(c, accessErr.Error(), nil)
			return td.EndGroups
		}
	}

	if _, err := db.Instance.GetCleanSettings(chatID); err != nil {
		_, _ = m.ReplyText(c, "Unable to load settings. Please try again later.", nil)
		return td.EndGroups
	}

	getPlayMode := db.Instance.GetPlayMode(chatID)
	playModeStr := utils.Everyone
	if getPlayMode {
		playModeStr = utils.Admins
	}
	getAdminMode := db.Instance.GetAdminMode(chatID)
	cmdDelete := db.Instance.GetCmdDelete(chatID)
	language, _ := db.Instance.GetLanguage(chatID)
	if language == "" {
		language = "ar"
	}
	autoplay := cache.ChatCache.GetAutoplay(chatID)
	chatEnabled := db.Instance.GetChatEnabled(chatID)

	text := renderSettingsMessageText(title, chatID, language)

	_, err := m.ReplyText(c, text, &td.SendMessageOpts{
		ReplyMarkup: cleanSettingsKeyboard(chatID, playModeStr, getAdminMode, cmdDelete, language, autoplay, chatEnabled),
		ParseMode:   td.ParseModeHTML,
	})
	return err
}

func langCommandHandler(c *td.Client, m *td.Message) error {
	return settingsHandler(c, m)
}

func settingsCallbackHandler(c *td.Client, cb *td.UpdateNewCallbackQuery, cancelCleanup func(int64)) error {
	chatID := cb.ChatId
	isPrivate := cb.IsPrivate()

	var title string
	if isPrivate {
		title = "Personal Room"
		if u, err := c.GetUser(cb.SenderUserId); err == nil && u != nil {
			title = strings.TrimSpace(u.FirstName + " " + u.LastName)
		}
	} else {
		chat, err := c.GetChat(chatID)
		if err != nil || !messaging.IsGroup(chat) {
			return cb.Answer(c, "These settings are available only in groups or private bot chat.", &td.CallbackAnswerOpts{ShowAlert: true})
		}
		title = chat.Title
		if accessErr := cache.SettingsAccess(c, chatID, cb.SenderUserId, "main"); accessErr != nil {
			return cb.Answer(c, accessErr.Error(), &td.CallbackAnswerOpts{ShowAlert: true})
		}
	}

	data := cb.DataString()
	if data == "settings_main" {
		return cb.Answer(c, "Update your settings", nil)
	}

	parts := strings.Split(data, "_")
	if len(parts) < 2 || parts[0] != "settings" {
		return cb.Answer(c, "Invalid settings button. Open /settings again.", nil)
	}
	settingType := parts[1]
	if !isPrivate {
		if accessErr := cache.SettingsAccess(c, chatID, cb.SenderUserId, settingType); accessErr != nil {
			return cb.Answer(c, accessErr.Error(), &td.CallbackAnswerOpts{ShowAlert: true})
		}
	}
	_, err := db.Instance.GetCleanSettings(chatID)
	if err != nil {
		return cb.Answer(c, "Unable to load settings. Try again later.", nil)
	}

	if settingType == "setlang" && len(parts) == 3 {
		newLang := parts[2]
		if newLang != "ar" && newLang != "en" {
			newLang = "ar"
		}
		err = db.Instance.SetChatLanguage(chatID, newLang)
	} else if settingType == "autodelete" || settingType == "silent" || settingType == "timer" {
		if isPrivate {
			return cb.Answer(c, "Group cleanup settings apply only to groups.", &td.CallbackAnswerOpts{ShowAlert: true})
		}
		value, parseErr := parseCleanCallback(data)
		if parseErr != nil {
			return cb.Answer(c, parseErr.Error(), nil)
		}
		switch settingType {
		case "autodelete":
			err = db.Instance.SetAutoDelete(chatID, value == 1)
			if err == nil && value == 0 && cancelCleanup != nil {
				cancelCleanup(chatID)
			}
		case "silent":
			err = db.Instance.SetSilentMode(chatID, value == 1)
		case "timer":
			err = db.Instance.SetAutoDeleteMinutes(chatID, value)
		}
	} else if len(parts) != 2 {
		return cb.Answer(c, "This settings button is outdated. Open /settings again.", &td.CallbackAnswerOpts{ShowAlert: true})
	} else {
		switch settingType {
		case "delete":
			if isPrivate {
				return cb.Answer(c, "Command delete applies only to groups.", &td.CallbackAnswerOpts{ShowAlert: true})
			}
			cmdDelete := db.Instance.GetCmdDelete(chatID)
			if !cmdDelete {
				if !cache.BotCanDeleteCommands(c, chatID) {
					_ = cb.Answer(c, "I don't have permission to delete messages in this chat.", &td.CallbackAnswerOpts{ShowAlert: true})
					return nil
				}
				err = db.Instance.SetCmdDelete(chatID, true)
			} else {
				err = db.Instance.SetCmdDelete(chatID, false)
			}
		case "play":
			getPlayMode := db.Instance.GetPlayMode(chatID)
			err = db.Instance.SetPlayMode(chatID, !getPlayMode)
		case "admin":
			getAdminMode := db.Instance.GetAdminMode(chatID)
			newMode := utils.Everyone
			if getAdminMode == utils.Everyone {
				newMode = utils.Admins
			}
			err = db.Instance.SetAdminMode(chatID, newMode)
		case "autoplay":
			autoplay := cache.ChatCache.GetAutoplay(chatID)
			cache.ChatCache.SetAutoplay(chatID, !autoplay)
		case "chat":
			chatEnabled := db.Instance.GetChatEnabled(chatID)
			newEnabled := !chatEnabled
			err = db.Instance.SetChatEnabled(chatID, newEnabled)
		case "lang":
			currLang, _ := db.Instance.GetLanguage(chatID)
			nextLang := "en"
			if currLang == "en" {
				nextLang = "ar"
			}
			err = db.Instance.SetChatLanguage(chatID, nextLang)
		default:
			return cb.Answer(c, "Unknown setting", &td.CallbackAnswerOpts{ShowAlert: true})
		}
	}
	if err != nil {
		return cb.Answer(c, "Unable to save settings. Please try again.", &td.CallbackAnswerOpts{ShowAlert: true})
	}
	webapp.HubInstance.BroadcastChatSettings(c, chatID)

	getPlayMode := db.Instance.GetPlayMode(chatID)
	playModeStr := utils.Everyone
	if getPlayMode {
		playModeStr = utils.Admins
	}
	getAdminMode := db.Instance.GetAdminMode(chatID)
	cmdDelete := db.Instance.GetCmdDelete(chatID)
	language, _ := db.Instance.GetLanguage(chatID)
	autoplay := cache.ChatCache.GetAutoplay(chatID)
	chatEnabled := db.Instance.GetChatEnabled(chatID)

	text := renderSettingsMessageText(title, chatID, language)

	_, err = cb.EditMessageText(c, text, &td.EditMessageTextOpts{
		ReplyMarkup: cleanSettingsKeyboard(chatID, playModeStr, getAdminMode, cmdDelete, language, autoplay, chatEnabled),
		ParseMode:   td.ParseModeHTML,
	})
	if err != nil {
		return err
	}

	if strings.EqualFold(language, "ar") {
		_ = cb.Answer(c, "تم تحديث الإعدادات بنجاح", nil)
	} else {
		_ = cb.Answer(c, "Settings updated", nil)
	}
	return nil
}

func parseCleanCallback(data string) (int, error) {
	parts := strings.Split(data, "_")
	if len(parts) != 3 || parts[0] != "settings" {
		return 0, fmt.Errorf("invalid settings button. Open /settings again")
	}
	value, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("invalid setting value")
	}
	switch parts[1] {
	case "timer":
		if err := db.ValidateAutoDeleteMinutes(value); err != nil {
			return 0, err
		}
	case "autodelete", "silent":
		if value != 0 && value != 1 {
			return 0, fmt.Errorf("invalid setting value")
		}
	default:
		return 0, fmt.Errorf("unknown setting")
	}
	return value, nil
}

func cleanSettingsKeyboard(chatID int64, playMode, adminMode string, cmdDelete bool, language string, autoplay, chatEnabled bool) *td.ReplyMarkupInlineKeyboard {
	keyboard := utils.SettingsKeyboard(playMode, adminMode, cmdDelete, language, autoplay, chatEnabled)
	if chatID >= 0 {
		return keyboard
	}
	settings, err := db.Instance.GetCleanSettings(chatID)
	if err != nil {
		return keyboard
	}
	button := func(text, data string) td.InlineKeyboardButton {
		return td.InlineKeyboardButton{Text: text, Type: &td.InlineKeyboardButtonTypeCallback{Data: []byte(data)}}
	}
	state := func(on bool) string {
		if on {
			return "ON"
		}
		return "OFF"
	}
	next := func(on bool) int {
		if on {
			return 0
		}
		return 1
	}
	rows := [][]td.InlineKeyboardButton{
		{button("Auto-delete: "+state(settings.AutoDelete), fmt.Sprintf("settings_autodelete_%d", next(settings.AutoDelete)))},
		{button(fmt.Sprintf("Delete after: %d minutes", settings.Minutes), "settings_main")},
	}
	var timers []td.InlineKeyboardButton
	for minutes := 2; minutes <= 7; minutes++ {
		label := strconv.Itoa(minutes)
		if minutes == settings.Minutes {
			label = "✓ " + label
		}
		timers = append(timers, button(label, fmt.Sprintf("settings_timer_%d", minutes)))
	}

	rows = append(rows, timers, []td.InlineKeyboardButton{button("Silent mode: "+state(settings.Silent), fmt.Sprintf("settings_silent_%d", next(settings.Silent)))})
	keyboard.Rows = append(rows, keyboard.Rows...)
	return keyboard
}
