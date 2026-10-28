package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

	_ "github.com/joho/godotenv/autoload"
)

const (
	defaultDBName         = "Anon"
	defaultAPIURL         = "https://api.onegrab.fun"
	defaultService        = "youtube"
	defaultDownloadsDir   = "downloads"
	defaultSupportGroup   = "https://t.me/FallenSupport"
	defaultSupportChannel = "https://t.me/FallenProjects"
	defaultStartImage     = "https://i.pinimg.com/736x/0d/f4/65/0df465d1e98239ecb6283400605fc813.jpg"
	defaultMaxFileSize    = int64(500 * 1024 * 1024)
	defaultSongDuration   = int32(3600)
	defaultAutoPlay       = int32(10)
)

func detectDefaultDownloadsDir() string {
	if d := strings.TrimSpace(os.Getenv("DOWNLOADS_DIR")); d != "" {
		return d
	}
	if d := strings.TrimSpace(os.Getenv("DOWNLOAD_DIR")); d != "" {
		return d
	}
	if info, err := os.Stat("/dev/shm"); err == nil && info.IsDir() {
		return "/dev/shm/telemusic_downloads"
	}
	return defaultDownloadsDir
}

func detectDefaultWebAppURL() string {
	flyApp := strings.TrimSpace(os.Getenv("FLY_APP_NAME"))
	u := strings.TrimRight(strings.TrimSpace(os.Getenv("WEBAPP_URL")), "/")
	// Always prefer the live FLY_APP_NAME domain on Fly.io so deleted/old Fly app names never cause ERR_NAME_NOT_RESOLVED
	if flyApp != "" && (u == "" || strings.HasSuffix(u, ".fly.dev")) {
		return "https://" + flyApp + ".fly.dev"
	}
	if u != "" {
		return u
	}
	if flyApp != "" {
		return "https://" + flyApp + ".fly.dev"
	}
	if renderURL := strings.TrimSpace(os.Getenv("RENDER_EXTERNAL_URL")); renderURL != "" {
		return strings.TrimRight(renderURL, "/")
	}
	return "https://telegram-web-osliyw.fly.dev"
}

var (
	ApiId      = getEnvInt32("API_ID", 0)
	ApiHash    = os.Getenv("API_HASH")
	Token      = os.Getenv("TOKEN")
	DlBotToken = os.Getenv("DL_BOT_TOKEN")

	MongoUri = os.Getenv("MONGO_URI")
	DbName   = getEnv("DB_NAME", defaultDBName)

	ApiUrl = getEnv("API_URL", defaultAPIURL)
	ApiKey = os.Getenv("API_KEY")

	OwnerId  = getEnvInt64("OWNER_ID", 0)
	LoggerId = getEnvInt64("LOGGER_ID", 0)

	Proxy             = os.Getenv("PROXY")
	DefaultService    = strings.ToLower(getEnv("DEFAULT_SERVICE", defaultService))
	AutoPlayLimit     = getEnvInt32("AUTO_PLAY_LIMIT", defaultAutoPlay)
	MaxFileSize       = getEnvInt64("MAX_FILE_SIZE", defaultMaxFileSize)
	SongDurationLimit = getEnvInt32("SONG_DURATION_LIMIT", defaultSongDuration)
	DownloadsDir      = detectDefaultDownloadsDir()

	SupportGroup     = getEnv("SUPPORT_GROUP", defaultSupportGroup)
	SupportChannel   = getEnv("SUPPORT_CHANNEL", defaultSupportChannel)
	StartImg         = getEnv("START_IMG", defaultStartImage)
	PrivacyPolicyURL = getEnv("PRIVACY_POLICY_URL", "")

	Port            = getEnv("PORT", "8080")
	WebAppURL       = detectDefaultWebAppURL()
	WebAppShortName = getEnv("WEBAPP_SHORT_NAME", "web")

	DEVS        []int64
	CookiesPath []string
	cookiesUrl  = processCookieURLs(os.Getenv("COOKIES_URL"))
)

func IsDevID(userID int64) bool {
	if userID == 0 {
		return false
	}
	if OwnerId != 0 && userID == OwnerId {
		return true
	}
	return slices.Contains(DEVS, userID)
}

func LoadEnv() error {
	ApiId = getEnvInt32("API_ID", ApiId)
	ApiHash = getEnv("API_HASH", ApiHash)
	Token = getEnv("TOKEN", Token)
	DlBotToken = getEnv("DL_BOT_TOKEN", DlBotToken)
	MongoUri = getEnv("MONGO_URI", MongoUri)
	DbName = getEnv("DB_NAME", DbName)
	ApiUrl = getEnv("API_URL", ApiUrl)
	ApiKey = getEnv("API_KEY", ApiKey)
	OwnerId = getEnvInt64("OWNER_ID", OwnerId)
	LoggerId = getEnvInt64("LOGGER_ID", LoggerId)
	Proxy = getEnv("PROXY", Proxy)
	DefaultService = strings.ToLower(getEnv("DEFAULT_SERVICE", DefaultService))
	AutoPlayLimit = getEnvInt32("AUTO_PLAY_LIMIT", AutoPlayLimit)
	MaxFileSize = getEnvInt64("MAX_FILE_SIZE", MaxFileSize)
	SongDurationLimit = getEnvInt32("SONG_DURATION_LIMIT", SongDurationLimit)
	DownloadsDir = detectDefaultDownloadsDir()
	Port = getEnv("PORT", Port)
	WebAppURL = detectDefaultWebAppURL()
	WebAppShortName = getEnv("WEBAPP_SHORT_NAME", WebAppShortName)

	loadDevelopers()

	if OwnerId != 0 && !slices.Contains(DEVS, OwnerId) {
		DEVS = append(DEVS, OwnerId)
	}

	if err := validate(); err != nil {
		return fmt.Errorf("config is invalid: %w", err)
	}

	if err := os.MkdirAll(DownloadsDir, 0o755); err != nil {
		return errors.New("failed to create downloads directory")
	}

	if len(cookiesUrl) == 0 {
		return nil
	}

	if err := os.MkdirAll(cookiesDr, 0o750); err != nil {
		return errors.New("failed to create cookies directory")
	}

	go saveAllCookies(cookiesUrl)
	return nil
}

func validate() error {
	required := []struct {
		name  string
		valid bool
	}{
		{"API_ID", ApiId > 0},
		{"API_HASH", ApiHash != ""},
		{"TOKEN", Token != ""},
		{"MONGO_URI", MongoUri != ""},
		{"OWNER_ID", OwnerId > 0},
	}

	var missing []string

	for _, field := range required {
		if !field.valid {
			missing = append(missing, field.name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf(
			"missing required configuration: %s",
			strings.Join(missing, ", "),
		)
	}

	if MaxFileSize <= 0 {
		return fmt.Errorf("MAX_FILE_SIZE must be greater than 0")
	}

	if SongDurationLimit <= 0 {
		return fmt.Errorf("SONG_DURATION_LIMIT must be greater than 0")
	}

	if !isValidService(DefaultService) {
		slog.Warn("invalid default service, falling back to default",
			"service", DefaultService,
			"default", defaultService,
		)

		DefaultService = defaultService
	}

	if AutoPlayLimit <= 0 {
		AutoPlayLimit = 5
	}

	return nil
}

func isValidService(service string) bool {
	switch strings.ToLower(strings.TrimSpace(service)) {
	case "youtube", "spotify":
		return true
	default:
		return false
	}
}
