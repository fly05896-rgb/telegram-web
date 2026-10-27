package webapp

import (
	"ashokshau/tg-web/internal/utils"

	td "github.com/AshokShau/gotdbot"
)

const playerContextCallbackPrefix = "webapp_verify:"

func WebAppControlButtons(mode, botUsername string, chatID int64) *td.ReplyMarkupInlineKeyboard {
	return utils.WebAppControlButtons(mode, botUsername, chatID)
}

func webAppControlButtonsForContext(mode, botUsername string, chatID int64, _ string) *td.ReplyMarkupInlineKeyboard {
	return utils.WebAppControlButtons(mode, botUsername, chatID)
}

func PlayerContextCallbackMode(data string) (string, bool) {
	if data == "webapp_verify" {
		return "webapp", true
	}

	if len(data) < len(playerContextCallbackPrefix) || data[:len(playerContextCallbackPrefix)] != playerContextCallbackPrefix {
		return "", false
	}

	mode := data[len(playerContextCallbackPrefix):]
	switch mode {
	case "", "play", "pause", "resume", "webapp":
		return mode, true
	default:
		log.Warnf("playerContextCallbackMode: invalid mode: %s", mode)
		return "", false
	}
}
