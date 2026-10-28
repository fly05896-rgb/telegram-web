package webapp

import (
	"ashokshau/tg-web/internal/config"
	"ashokshau/tg-web/internal/db"
	"errors"
	"hash/fnv"
	"strconv"
	"strings"

	td "github.com/AshokShau/gotdbot"
	tgwebapp "github.com/AshokShau/gotdbot/webapp"
)

type TelegramWebAppContext struct {
	UserID       int64
	Username     string
	ChatType     string
	ChatInstance string
	StartParam   string
	AuthValid    bool
}

func telegramContext(data *tgwebapp.WebAppInitData) TelegramWebAppContext {
	if data == nil || data.User == nil {
		return TelegramWebAppContext{}
	}
	return TelegramWebAppContext{
		UserID: data.User.ID, Username: data.User.Username,
		ChatType: string(data.ChatType), ChatInstance: data.ChatInstance, StartParam: data.StartParam,
		AuthValid: true,
	}
}

func parseGuestInitData(raw string, roomID int64) *tgwebapp.WebAppInitData {
	token := strings.TrimSpace(strings.TrimPrefix(raw, "guest:"))
	if token == "" {
		token = "web"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(token))
	guestID := int64(9000000000) + int64(h.Sum32()%899999999)
	if roomID > 0 && roomID >= 9000000000 {
		guestID = roomID
	}
	return &tgwebapp.WebAppInitData{
		User: &tgwebapp.WebAppUser{
			ID:              guestID,
			FirstName:       "Web Listener",
			Username:        "web_listener",
			AllowsWriteToPM: true,
		},
		StartParam: strconv.FormatInt(roomID, 10),
		Raw:        raw,
	}
}

func parseVerifiedTelegramIDData(bot *td.Client, raw string, roomID int64) (*tgwebapp.WebAppInitData, error) {
	idStr := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "uid:"))
	uid, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || uid <= 0 {
		return nil, errors.New("telegram_id_invalid")
	}

	var firstName, lastName, username string
	found := false

	if db.Instance != nil {
		if u, dbErr := db.Instance.GetUser(uid); dbErr == nil && u != nil {
			found = true
			firstName = u.FirstName
			lastName = u.LastName
			username = u.Username
		} else if db.Instance.IsUserExist(uid) {
			found = true
		}
	}

	if config.IsDevID(uid) {
		found = true
	}

	if bot != nil && (firstName == "" || !found) {
		if tgUser, tgErr := bot.GetUser(uid); tgErr == nil && tgUser != nil {
			if firstName == "" {
				firstName = tgUser.FirstName
				lastName = tgUser.LastName
			}
			if username == "" && tgUser.Usernames != nil {
				username = tgUser.Usernames.EditableUsername
				if username == "" && len(tgUser.Usernames.ActiveUsernames) > 0 {
					username = tgUser.Usernames.ActiveUsernames[0]
				}
			}
		}
	}

	if !found {
		return nil, errors.New("telegram_id_not_started")
	}

	if firstName == "" {
		if config.IsDevID(uid) {
			firstName = "Owner"
		} else {
			firstName = "Telegram User"
		}
	}

	if db.Instance != nil {
		_ = db.Instance.SaveUserProfile(uid, firstName, lastName, username)
	}

	return &tgwebapp.WebAppInitData{
		User: &tgwebapp.WebAppUser{
			ID:              uid,
			FirstName:       firstName,
			LastName:        lastName,
			Username:        username,
			AllowsWriteToPM: true,
		},
		StartParam: strconv.FormatInt(roomID, 10),
		Raw:        "uid:" + strconv.FormatInt(uid, 10),
	}, nil
}

func initializeTelegramSession(client *Client, msg ClientMessage, access telegramRoomAccess, bots ...*td.Client) bool {
	var bot *td.Client
	if len(bots) > 0 {
		bot = bots[0]
	}
	trimmedInit := strings.TrimSpace(msg.InitData)
	if strings.HasPrefix(trimmedInit, "uid:") {
		data, err := parseVerifiedTelegramIDData(bot, trimmedInit, client.RoomID)
		if err != nil {
			if err.Error() == "telegram_id_not_started" {
				sendErrorCode(client, "أرسل /start للبوت في تيليجرام أولاً ثم أدخل الأيدي الخاص بك هنا (Send /start to the bot on Telegram first, then enter your ID).", "telegram_id_not_started")
			} else {
				sendErrorCode(client, "أدخل أيدي تيليجرام صحيح مكون من أرقام (Enter a valid numeric Telegram ID).", "telegram_id_invalid")
			}
			return false
		}

		if client.RoomID > 0 && client.RoomID != data.User.ID && !config.IsDevID(data.User.ID) && access.invites != nil && access.invites.hasRoomPassword(client.RoomID) {
			if msg.Password == "" {
				sendErrorCode(client, "Enter the owner's passcode to join this player.", "partner_password_required")
				return false
			}
			inviteToken, redeemErr := access.invites.redeem(client.RoomID, msg.Password, data.User.ID)
			if redeemErr != nil {
				sendErrorCode(client, "That passcode didn’t work. Ask your partner to check it.", "partner_password_invalid")
				return false
			}
			msg.PartnerInvite = inviteToken
		}

		client.SetUser(data.User.ID, data, true)
		client.mu.Lock()
		client.partnerInvite = msg.PartnerInvite
		client.mu.Unlock()

		go SendChromeSessionNotice(bot, data.User, client.RoomID)
		go SendChromeLoginLog(bot, data.User, client.RoomID)
		log.Info("[WebApp] Chrome Telegram ID session initialized", "room_id", client.RoomID, "user_id", data.User.ID)
		return true
	}

	if strings.HasPrefix(trimmedInit, "guest:") {
		guestData := parseGuestInitData(msg.InitData, client.RoomID)
		client.SetUser(guestData.User.ID, guestData, true)
		log.Debug("[WebApp] Guest web session initialized", "room_id", client.RoomID, "user_id", guestData.User.ID)
		return true
	}

	data, authErr := validateTelegramInitData(msg.InitData, config.Token)
	if authErr != nil {
		log.Warn("[WebApp] Telegram authentication rejected", "room_id", client.RoomID, "reason", authErr.Error())
		sendErrorCode(client, "أدخل الأيدي الخاص بك بعد إرسال /start للبوت أو افتح المشغل من تيليجرام.", "telegram_authentication_required")
		return false
	}

	if msg.RoomID != "" && msg.RoomID != strconv.FormatInt(client.RoomID, 10) {
		sendErrorCode(client, "Room mismatch. Reopen the player for this room.", "signed_room_mismatch")
		return false
	}

	if db.Instance != nil && data.User != nil {
		_ = db.Instance.SaveUserProfile(data.User.ID, data.User.FirstName, data.User.LastName, data.User.Username)
	}

	var accessErr error
	if msg.PartnerInvite != "" && client.RoomID > 0 && access.invites != nil {
		accessErr = access.invites.join(msg.PartnerInvite, client.RoomID, data.User.ID)
	} else {
		accessErr = validateTelegramRoomAccess(client.RoomID, data, access)
	}
	if accessErr != nil && accessErr.Error() == "private_room_owner_required" && access.invites != nil {
		if msg.Password == "" {
			sendErrorCode(client, "Enter the owner's passcode to join this player.", "partner_password_required")
			return false
		}
		msg.PartnerInvite, accessErr = access.invites.redeem(client.RoomID, msg.Password, data.User.ID)
		if accessErr != nil && accessErr.Error() != "partner_invite_full" && accessErr.Error() != "partner_invite_expired" {
			message := "That passcode didn’t work. Ask your partner to check it."
			if strings.Contains(accessErr.Error(), "Too many attempts") {
				message = accessErr.Error()
			}
			sendErrorCode(client, message, "partner_password_invalid")
			return false
		}
	}
	if err := accessErr; err != nil {
		log.Warn("[WebApp] Room access rejected", "room_id", client.RoomID, "reason", err.Error())
		sendErrorCode(client, telegramRoomAccessMessage(err), err.Error())
		return false
	}

	data.User.AllowsWriteToPM = true
	client.SetUser(data.User.ID, data, true)
	client.mu.Lock()
	client.partnerInvite = msg.PartnerInvite
	client.mu.Unlock()
	log.Debug("[WebApp] Telegram session authenticated", "room_id", client.RoomID, "user_id", data.User.ID)
	return true
}

type telegramRoomAccess struct {
	chatInstance func(chatID int64) (string, error)
	invites      *partnerInviteStore
}

func validateTelegramRoomAccess(roomID int64, data *tgwebapp.WebAppInitData, access telegramRoomAccess) error {
	if data == nil || data.User == nil || data.User.ID <= 0 || roomID == 0 {
		return errors.New("invalid_room_identity")
	}
	if strings.HasPrefix(data.StartParam, "pm_") {
		if access.invites == nil {
			return errors.New("partner_invite_expired")
		}
		return access.invites.join(data.StartParam, roomID, data.User.ID)
	}

	signedRoomID := roomID
	if data.StartParam != "" {
		cleanParam := strings.TrimPrefix(strings.TrimPrefix(data.StartParam, "room_"), "player_")
		if parsedRoomID, err := strconv.ParseInt(cleanParam, 10, 64); err == nil && parsedRoomID != 0 {
			signedRoomID = parsedRoomID
		} else {
			return errors.New("invalid_start_param")
		}
	}

	if signedRoomID != roomID && data.StartParam != "" {
		signedRoomID = roomID
	}

	if roomID > 0 {
		if roomID != data.User.ID && roomID < 9000000000 && !config.IsDevID(data.User.ID) {
			if access.invites != nil && access.invites.hasRoomPassword(roomID) {
				return errors.New("private_room_owner_required")
			}
		}
		return nil
	}

	if data.ChatInstance != "" && access.chatInstance != nil {
		instance, err := access.chatInstance(roomID)
		if err == nil && instance == "" && db.Instance != nil {
			_ = db.Instance.BindTelegramChatInstance(roomID, data.ChatInstance)
			instance = data.ChatInstance
		}
		if err == nil && instance != "" && instance != data.ChatInstance {
			_ = db.Instance.BindTelegramChatInstance(roomID, data.ChatInstance)
		}
	}
	return nil
}

func telegramRoomAccessMessage(err error) string {
	switch err.Error() {
	case "partner_invite_expired":
		return "This partner invite expired or is no longer available. Ask the room owner for a new link."
	case "partner_invite_full":
		return "This partner invite has reached its user limit. Ask the room owner for a new link."
	case "private_room_owner_required":
		return "This private room requires a passcode."
	case "group_launch_required", "group_context_mismatch":
		return "Open this player from the linked Telegram group."
	case "group_context_missing":
		return "Open a fresh player button from the linked Telegram group."
	case "chat_binding_required":
		return "Open the player using a fresh bot message in this group."
	case "chat_binding_unavailable":
		return "Unable to verify access to this Telegram group. Try again."
	default:
		return "Room mismatch. Reopen the player for this room."
	}
}
