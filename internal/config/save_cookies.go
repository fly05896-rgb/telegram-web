/*
 * TgMusicBot - Telegram Music Bot
 * Copyright (c) 2025-2026 Ashok Shau
 *
 * Licensed under GNU GPL v3
 * See https://github.com/AshokShau/TgMusicBot
 */

package config

import (
	"os"
)

const (
	cookiesDr = "src/cookies"
)

// saveAllCookies enforces 2026 100% Cookie-Free mode (ported from cookie_handler.py & __main__.py):
// removes any legacy cookies directory or stale cookies.txt so expired cookies never trigger YouTube blocks.
func saveAllCookies(_ []string) {
	CookiesPath = nil
	_ = os.RemoveAll(cookiesDr)
	_ = os.Remove("cookies.txt")
}
