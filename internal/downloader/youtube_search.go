/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package downloader

import (
	"ashokshau/tg-web/internal/utils"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ytBaseURL         = "https://www.youtube.com"
	ytWatchURL        = ytBaseURL + "/watch?v="
	ytAPIKey          = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
	ytClientVer       = "2.20260708.00.00"
	defaultUserAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	requestRetryDelay = 80 * time.Millisecond
)

type cachedTrackEntry struct {
	track     utils.GetUrlTrack
	expiresAt time.Time
}

var (
	visitorDataMu sync.RWMutex
	visitorData   string

	metaCacheMu sync.RWMutex
	metaCache   = make(map[string]cachedTrackEntry)
)

func getVisitorData() string {
	visitorDataMu.RLock()
	defer visitorDataMu.RUnlock()
	return visitorData
}

func setVisitorData(v string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return
	}
	visitorDataMu.Lock()
	visitorData = v
	visitorDataMu.Unlock()
}

// ensureVisitorData fetches a fresh cookie-free visitorData token from /youtubei/v1/guide (2026 AnnieXMedia engine)
func ensureVisitorData(ctx context.Context) string {
	if v := getVisitorData(); v != "" {
		return v
	}
	reqCtx, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
	defer cancel()

	payload := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "WEB",
				"clientVersion": ytClientVer,
				"hl":            "en",
				"gl":            "US",
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, ytBaseURL+"/youtubei/v1/guide?prettyPrint=false", bytes.NewReader(body))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Origin", ytBaseURL)

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	var data map[string]any
	_ = json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&data)
	_ = resp.Body.Close()

	if v := digStr(data, "responseContext", "visitorData"); v != "" {
		setVisitorData(v)
		return v
	}
	return ""
}

// GetCachedTrackMeta returns a cached GetUrlTrack if available and not expired.
func GetCachedTrackMeta(videoID string) (utils.GetUrlTrack, bool) {
	if len(videoID) != 11 {
		return utils.GetUrlTrack{}, false
	}
	metaCacheMu.RLock()
	entry, ok := metaCache[videoID]
	metaCacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return utils.GetUrlTrack{}, false
	}
	return entry.track, true
}

// StoreTrackMeta caches a GetUrlTrack in memory for 1 hour.
func StoreTrackMeta(track utils.GetUrlTrack) {
	if len(track.Id) != 11 || track.Title == "" {
		return
	}
	metaCacheMu.Lock()
	if len(metaCache) > 2000 {
		metaCache = make(map[string]cachedTrackEntry)
	}
	metaCache[track.Id] = cachedTrackEntry{
		track:     track,
		expiresAt: time.Now().Add(time.Hour),
	}
	metaCacheMu.Unlock()
}

type ytClientProfile struct {
	ClientName    string
	ClientVersion string
	ClientID      string
	Origin        string
	UserAgent     string

	OsName      string
	OsVersion   string
	AndroidSDK  int
	DeviceMake  string
	DeviceModel string

	EmbedURL string
}

var ytProfiles = []ytClientProfile{
	{
		ClientName:    "WEB",
		ClientVersion: ytClientVer,
		ClientID:      "1",
		Origin:        ytBaseURL,
		UserAgent:     defaultUserAgent,
	},
	{
		ClientName:    "VISIONOS",
		ClientVersion: "1.02",
		ClientID:      "101",
		DeviceMake:    "Apple",
		DeviceModel:   "RealityDevice17,1",
		OsName:        "visionOS",
		OsVersion:     "26.5.23O471",
		UserAgent:     fastVisionUA,
	},
	{
		ClientName:    "ANDROID",
		ClientVersion: "21.26.364",
		ClientID:      "3",
		OsName:        "Android",
		OsVersion:     "11",
		AndroidSDK:    30,
		UserAgent:     fastAndroidUA,
	},
	{
		ClientName:    "WEB_REMIX",
		ClientVersion: "1.20260101.01.00",
		ClientID:      "67",
		Origin:        "https://music.youtube.com",
		UserAgent:     defaultUserAgent,
	},
	{
		ClientName:    "MWEB",
		ClientVersion: "2.20260101.01.00",
		ClientID:      "2",
		Origin:        "https://m.youtube.com",
		UserAgent:     "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
	},
	{
		ClientName:    "WEB_EMBEDDED_PLAYER",
		ClientVersion: "1.20260101.01.00",
		ClientID:      "56",
		Origin:        ytBaseURL,
		UserAgent:     fastWebEmbeddedUA,
		EmbedURL:      "https://www.reddit.com/",
	},
	{
		ClientName:    "TVHTML5",
		ClientVersion: "7.20260101.00.00",
		ClientID:      "7",
		Origin:        ytBaseURL,
		UserAgent:     fastTVSamsungUA,
	},
	{
		ClientName:    "TVHTML5_SIMPLY_EMBEDDED_PLAYER",
		ClientVersion: "2.0",
		ClientID:      "85",
		Origin:        ytBaseURL,
		UserAgent:     fastTVSamsungUA,
		EmbedURL:      "https://www.reddit.com/",
	},
	{
		ClientName:    "IOS",
		ClientVersion: "19.45.4",
		ClientID:      "5",
		OsName:        "iPhone OS",
		OsVersion:     "17.5.1.21F90",
		DeviceMake:    "Apple",
		DeviceModel:   "iPhone16,2",
		UserAgent:     "com.google.ios.youtube/19.45.4 (iPhone16,2; U; CPU iOS 17_5_1 like Mac OS X; en_US)",
	},
}

var (
	errYouTubeSearchNoResults      = errors.New("youtube search returned no results")
	errYouTubeSearchAgeRequired    = errors.New("YouTube requires age verification for this search. Try a different query.")
	errYouTubeSearchSignInRequired = errors.New("YouTube requires sign-in for this search. Try a different query.")
	labelDurationRe                = regexp.MustCompile(`(\d+)\s*(hours?|minutes?|seconds?)`)
	videoIDRe1                     = regexp.MustCompile(`(?i)(?:youtube\.com/(?:watch\?v=|embed/|shorts/|live/)|youtu\.be/)([A-Za-z0-9_-]{11})`)
	videoIDRe2                     = regexp.MustCompile(`(?:v=|\/)([0-9A-Za-z_-]{11})`)
	playlistIDRe1                  = regexp.MustCompile(`(?i)(?:youtube\.com|music\.youtube\.com).*(?:\?|&)list=([A-Za-z0-9_-]+)`)
	playlistIDRe2                  = regexp.MustCompile(`list=([0-9A-Za-z_-]+)`)
)

// setYTHeadersWithProfile sets standard headers for YouTube InnerTube API including X-Goog-Visitor-Id
func setYTHeadersWithProfile(req *http.Request, prof ytClientProfile) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", prof.UserAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,ar;q=0.8")
	req.Header.Set("X-YouTube-Client-Name", prof.ClientID)
	req.Header.Set("X-YouTube-Client-Version", prof.ClientVersion)
	if v := getVisitorData(); v != "" {
		req.Header.Set("X-Goog-Visitor-Id", v)
	}

	if prof.Origin != "" {
		req.Header.Set("Origin", prof.Origin)
		req.Header.Set("Referer", prof.Origin+"/")
		req.Header.Set("Sec-Fetch-Mode", "cors")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Sec-Fetch-Dest", "empty")
	}
}

func ytContextWithProfile(prof ytClientProfile) map[string]any {
	clientCtx := map[string]any{
		"clientName":       prof.ClientName,
		"clientVersion":    prof.ClientVersion,
		"hl":               "en",
		"gl":               "US",
		"timeZone":         "UTC",
		"utcOffsetMinutes": 0,
	}
	if v := getVisitorData(); v != "" {
		clientCtx["visitorData"] = v
	}
	if prof.OsName != "" {
		clientCtx["osName"] = prof.OsName
	}
	if prof.OsVersion != "" {
		clientCtx["osVersion"] = prof.OsVersion
	}
	if prof.AndroidSDK > 0 {
		clientCtx["androidSdkVersion"] = prof.AndroidSDK
	}
	if prof.DeviceMake != "" {
		clientCtx["deviceMake"] = prof.DeviceMake
	}
	if prof.DeviceModel != "" {
		clientCtx["deviceModel"] = prof.DeviceModel
	}

	innerContext := map[string]any{"client": clientCtx}
	if prof.EmbedURL != "" {
		innerContext["thirdParty"] = map[string]any{"embedUrl": prof.EmbedURL}
	}

	return map[string]any{
		"context":        innerContext,
		"contentCheckOk": true,
		"racyCheckOk":    true,
	}
}

func formatYTErr(action string, statusCode int, statusText string, body []byte) error {
	bodyStr := string(body)
	if statusCode == http.StatusForbidden || statusCode == http.StatusTooManyRequests ||
		strings.Contains(strings.ToLower(bodyStr), "<html") || strings.Contains(bodyStr, "automated queries") {
		return fmt.Errorf("%s failed: status=%d %s (rate limited / automated query block)", action, statusCode, statusText)
	}

	if len(bodyStr) > 200 {
		bodyStr = bodyStr[:200] + "..."
	}
	return fmt.Errorf("%s failed: status=%d %s body=%q", action, statusCode, statusText, bodyStr)
}

func ytRequest(ctx context.Context, path string, extraFields map[string]any, accept func(map[string]any) bool) (map[string]any, error) {
	if getVisitorData() == "" {
		ensureVisitorData(ctx)
	}

	var lastErr error
	searchRequest := path == "/youtubei/v1/search"
	profiles := ytProfiles
	if searchRequest {
		profiles = slices.DeleteFunc(slices.Clone(ytProfiles), func(prof ytClientProfile) bool { return prof.EmbedURL != "" })
	}

	for i, prof := range profiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(requestRetryDelay):
			}
		}

		payload := ytContextWithProfile(prof)
		maps.Copy(payload, extraFields)

		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}

		endpoint := ytBaseURL + path + "?prettyPrint=false&key=" + ytAPIKey
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		setYTHeadersWithProfile(req, prof)

		res, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s (%s): %w", path, prof.ClientName, err)
			continue
		}

		if res.StatusCode < 200 || res.StatusCode >= 300 {
			snippet, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
			_ = res.Body.Close()
			lastErr = formatYTErr(fmt.Sprintf("youtube %s (%s)", path, prof.ClientName), res.StatusCode, res.Status, snippet)
			continue
		}

		var out map[string]any
		err = json.NewDecoder(io.LimitReader(res.Body, 10*1024*1024)).Decode(&out)
		_ = res.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("decode response (%s): %w", prof.ClientName, err)
			continue
		}

		if v := digStr(out, "responseContext", "visitorData"); v != "" && getVisitorData() == "" {
			setVisitorData(v)
		}

		if accept != nil && accept(out) {
			return out, nil
		}
		if searchRequest {
			if err := youtubeSearchResponseError(out); err != nil {
				return nil, err
			}
		}

		if accept != nil {
			lastErr = fmt.Errorf("youtube %s (%s): response rejected (empty/invalid)", path, prof.ClientName)
			continue
		}

		return out, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("youtube %s failed: no client profiles succeeded", path)
	}
	return nil, lastErr
}

func ytPost(ctx context.Context, path string, extraFields map[string]any) (map[string]any, error) {
	return ytRequest(ctx, path, extraFields, nil)
}

func searchYouTube(query string, limit int, parentContext ...context.Context) ([]utils.GetUrlTrack, error) {
	parent := context.Background()
	if len(parentContext) > 0 {
		parent = parentContext[0]
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()

	var tracks []utils.GetUrlTrack
	_, err := ytRequest(ctx, "/youtubei/v1/search", map[string]any{"query": query}, func(data map[string]any) bool {
		var found []utils.GetUrlTrack
		parseResults(data, &found, limit)
		if len(found) == 0 {
			return false
		}
		tracks = found
		return true
	})

	if len(tracks) > 0 {
		for _, tr := range tracks {
			StoreTrackMeta(tr)
		}
		if tracks[0].Duration > 0 && tracks[0].Duration <= 900 {
			go PrefetchYouTubeAudio(tracks[0].Id)
		}
		return tracks, nil
	}
	if errors.Is(err, errYouTubeSearchNoResults) {
		return []utils.GetUrlTrack{}, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, errors.New("youtube search returned no results across client profiles")
}

func youtubeSearchResponseError(data map[string]any) error {
	stack := []any{data["contents"], data["alerts"]}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch value := node.(type) {
		case []any:
			stack = append(stack, value...)
		case map[string]any:
			for _, key := range []string{"backgroundPromoRenderer", "messageRenderer", "alertRenderer"} {
				message, ok := value[key].(map[string]any)
				if !ok {
					continue
				}
				copy := strings.ToLower(strings.Join([]string{youtubeMessageText(message["title"]), youtubeMessageText(message["bodyText"]), youtubeMessageText(message["text"])}, " "))
				switch {
				case strings.Contains(copy, "confirm your age"), strings.Contains(copy, "age-restricted"), strings.Contains(copy, "age restricted"):
					return errYouTubeSearchAgeRequired
				case dig(message, "ctaButton", "buttonRenderer", "navigationEndpoint", "signInEndpoint") != nil,
					strings.Contains(copy, "sign in"), strings.Contains(copy, "sign-in"):
					return errYouTubeSearchSignInRequired
				case strings.Contains(copy, "no results found"), strings.Contains(copy, "no results for"), strings.TrimSpace(copy) == "no results":
					return errYouTubeSearchNoResults
				}
			}
			for _, child := range value {
				stack = append(stack, child)
			}
		}
	}
	return nil
}

func youtubeMessageText(node any) string {
	if text, ok := node.(string); ok {
		return text
	}
	for _, field := range []string{"simpleText", "content"} {
		if text := digStr(node, field); text != "" {
			return text
		}
	}
	var text strings.Builder
	for _, run := range digArray(node, "runs") {
		text.WriteString(safeString(run["text"]))
	}
	return text.String()
}

func parseResults(node any, tracks *[]utils.GetUrlTrack, limit int) {
	stack := []any{node}
	for len(stack) > 0 && len(*tracks) < limit {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch v := curr.(type) {
		case []any:
			for _, v0 := range slices.Backward(v) {
				stack = append(stack, v0)
			}

		case map[string]any:
			if vr, ok := dig(v, "videoRenderer").(map[string]any); ok {
				if isLiveNow(vr) {
					continue
				}
				id := safeString(vr["videoId"])
				title := safeString(dig(vr, "title", "runs", 0, "text"))
				durationText := safeString(dig(vr, "lengthText", "simpleText"))
				if id == "" || title == "" || durationText == "" {
					continue
				}
				thumb := safeString(dig(vr, "thumbnail", "thumbnails", 0, "url"))
				if thumb == "" {
					thumb = "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"
				}
				tr := utils.GetUrlTrack{
					Id:        id,
					Url:       ytWatchURL + id,
					Title:     title,
					Thumbnail: thumb,
					Duration:  parseDuration(durationText),
					Views:     safeString(dig(vr, "viewCountText", "simpleText")),
					Channel:   safeString(dig(vr, "ownerText", "runs", 0, "text")),
					Platform:  utils.YouTube,
				}
				StoreTrackMeta(tr)
				*tracks = append(*tracks, tr)
				continue
			}

			for _, child := range v {
				stack = append(stack, child)
			}
		}
	}
}

func isLiveNow(vr map[string]any) bool {
	badges, ok := vr["badges"].([]any)
	if !ok {
		return false
	}
	for _, badge := range badges {
		meta, ok := dig(badge, "metadataBadgeRenderer").(map[string]any)
		if !ok {
			continue
		}
		if safeString(meta["style"]) == "BADGE_STYLE_TYPE_LIVE_NOW" {
			return true
		}
	}
	return false
}

func getYouTubeTitleFromOEmbed(videoID string) (string, error) {
	apiURL := fmt.Sprintf("https://www.youtube.com/oembed?url=https://www.youtube.com/watch?v=%s&format=json", videoID)

	resp, err := client.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("oEmbed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oEmbed returned status code: %d", resp.StatusCode)
	}

	var data struct {
		Title string `json:"title"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1*1024*1024)).Decode(&data); err != nil {
		return "", fmt.Errorf("failed to decode oEmbed response: %w", err)
	}

	if data.Title == "" {
		return "", errors.New("oEmbed response contained empty title")
	}

	return data.Title, nil
}

func getYouTubeVideo(ctx context.Context, videoID string) (*utils.PlatformTracks, error) {
	if cached, ok := GetCachedTrackMeta(videoID); ok {
		return &utils.PlatformTracks{Results: []utils.GetUrlTrack{cached}}, nil
	}

	resp, err := ytRequest(ctx, "/youtubei/v1/player", map[string]any{"videoId": videoID}, func(data map[string]any) bool {
		return digStr(data, "videoDetails", "videoId") != ""
	})
	if err != nil {
		return nil, err
	}

	video := mapPlayerToTrack(resp)
	if video.Id == "" {
		return nil, errors.New("video not found")
	}
	StoreTrackMeta(video)
	return &utils.PlatformTracks{Results: []utils.GetUrlTrack{video}}, nil
}

func getYouTubePlaylist(ctx context.Context, playlistID string) (*utils.PlatformTracks, error) {
	var tracks *utils.PlatformTracks
	_, err := ytRequest(ctx, "/youtubei/v1/browse", map[string]any{"browseId": "VL" + playlistID}, func(data map[string]any) bool {
		tracks = buildTrackList(extractPlaylistVideos(data), mapYTVideo)
		return len(tracks.Results) > 0
	})
	if err != nil {
		return nil, err
	}
	for _, tr := range tracks.Results {
		StoreTrackMeta(tr)
	}
	if len(tracks.Results) > 0 {
		go PrefetchYouTubeAudio(tracks.Results[0].Id)
	}
	return tracks, nil
}

func GetYouTubeMixPlaylist(ctx context.Context, playlistID string) (*utils.PlatformTracks, error) {
	resp, err := ytPost(ctx, "/youtubei/v1/next", map[string]any{"playlistId": playlistID})
	if err != nil {
		return nil, err
	}

	videos := extractMixPlaylistVideos(resp)
	list := buildTrackList(videos, mapMixVideo)
	for _, tr := range list.Results {
		StoreTrackMeta(tr)
	}
	return list, nil
}

func buildTrackList(videos []map[string]any, mapper func(map[string]any) utils.GetUrlTrack) *utils.PlatformTracks {
	out := make([]utils.GetUrlTrack, 0, len(videos))
	for _, v := range videos {
		if t := mapper(v); t.Id != "" {
			out = append(out, t)
		}
	}
	return &utils.PlatformTracks{Results: out}
}

func mapYTVideo(v map[string]any) utils.GetUrlTrack {
	id := digStr(v, "videoId")
	return utils.GetUrlTrack{
		Id:        id,
		Title:     digStr(v, "title", "runs", 0, "text"),
		Url:       ytWatchURL + id,
		Thumbnail: pickYTThumb(v),
		Channel:   digStr(v, "shortBylineText", "runs", 0, "text"),
		Duration:  parseYTDuration(v),
		Views:     digStr(v, "viewCountText", "simpleText"),
		Platform:  utils.YouTube,
	}
}

func mapMixVideo(v map[string]any) utils.GetUrlTrack {
	id := digStr(v, "videoId")
	return utils.GetUrlTrack{
		Id:        id,
		Title:     digStr(v, "title", "simpleText"),
		Url:       ytWatchURL + id,
		Thumbnail: pickYTThumb(v),
		Channel:   digStr(v, "shortBylineText", "runs", 0, "text"),
		Duration:  parseYTDuration(v),
		Platform:  utils.YouTube,
	}
}

func mapPlayerToTrack(src map[string]any) utils.GetUrlTrack {
	id := digStr(src, "videoDetails", "videoId")
	thumb := pickYTPlayerThumb(src)
	if thumb == "" && id != "" {
		thumb = "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"
	}
	return utils.GetUrlTrack{
		Id:        id,
		Title:     digStr(src, "videoDetails", "title"),
		Url:       ytWatchURL + id,
		Thumbnail: thumb,
		Channel:   digStr(src, "videoDetails", "author"),
		Duration:  atoi(digStr(src, "videoDetails", "lengthSeconds")),
		Views:     digStr(src, "videoDetails", "viewCount"),
		Platform:  utils.YouTube,
	}
}

func extractPlaylistVideos(src map[string]any) []map[string]any {
	var out []map[string]any
	var visit func(any)
	visit = func(node any) {
		switch value := node.(type) {
		case []any:
			for _, child := range value {
				visit(child)
			}
		case map[string]any:
			if video, ok := value["playlistVideoRenderer"].(map[string]any); ok {
				out = append(out, video)
				return
			}
			if video, ok := value["lockupViewModel"].(map[string]any); ok {
				if normalized := normalizePlaylistLockup(video); normalized != nil {
					out = append(out, normalized)
				}
				return
			}
			keys := slices.Sorted(maps.Keys(value))
			for _, key := range keys {
				visit(value[key])
			}
		}
	}
	visit(src["contents"])
	return out
}

func normalizePlaylistLockup(video map[string]any) map[string]any {
	if digStr(video, "contentType") != "LOCKUP_CONTENT_TYPE_VIDEO" || digStr(video, "contentId") == "" {
		return nil
	}
	metadata := dig(video, "metadata", "lockupMetadataViewModel")
	image := dig(video, "contentImage", "thumbnailViewModel")
	duration := ""
	for _, overlay := range digArray(image, "overlays") {
		for _, badge := range digArray(overlay, "thumbnailBottomOverlayViewModel", "badges") {
			text := digStr(badge, "thumbnailBadgeViewModel", "text")
			if strings.Contains(text, ":") {
				duration = text
			}
		}
	}
	return map[string]any{
		"videoId":         digStr(video, "contentId"),
		"title":           map[string]any{"runs": []any{map[string]any{"text": digStr(metadata, "title", "content")}}},
		"shortBylineText": map[string]any{"runs": []any{map[string]any{"text": digStr(metadata, "metadata", "contentMetadataViewModel", "metadataRows", 0, "metadataParts", 0, "text", "content")}}},
		"thumbnail":       map[string]any{"thumbnails": dig(image, "image", "sources")},
		"lengthText":      map[string]any{"simpleText": duration},
	}
}

func extractMixPlaylistVideos(src map[string]any) []map[string]any {
	contents := digArray(src,
		"contents",
		"twoColumnWatchNextResults",
		"playlist", "playlist", "contents",
	)
	var out []map[string]any
	for _, c := range contents {
		if v, ok := c["playlistPanelVideoRenderer"].(map[string]any); ok {
			out = append(out, v)
		}
	}
	return out
}

func pickYTThumb(v map[string]any) string {
	return lastThumbURL(digArray(v, "thumbnail", "thumbnails"))
}

func pickYTPlayerThumb(src map[string]any) string {
	return lastThumbURL(digArray(src, "videoDetails", "thumbnail", "thumbnails"))
}

func lastThumbURL(thumbs []map[string]any) string {
	if len(thumbs) == 0 {
		return ""
	}
	t, _ := thumbs[len(thumbs)-1]["url"].(string)
	return t
}

func normalizeYouTubeURL(rawURL string) string {
	var id string
	switch {
	case strings.Contains(rawURL, "youtu.be/"):
		id = extractSegment(rawURL, "youtu.be/")
	case strings.Contains(rawURL, "youtube.com/shorts/"):
		id = extractSegment(rawURL, "youtube.com/shorts/")
	case strings.Contains(rawURL, "youtube.com/live/"):
		id = extractSegment(rawURL, "youtube.com/live/")
	default:
		return rawURL
	}
	return ytWatchURL + id
}

func extractSegment(u, sep string) string {
	parts := strings.SplitN(u, sep, 2)
	if len(parts) < 2 {
		return u
	}
	after := parts[1]
	after = strings.SplitN(after, "?", 2)[0]
	after = strings.SplitN(after, "#", 2)[0]
	return after
}

func extractVideoID(u string) string {
	s := strings.TrimSpace(u)
	if len(s) == 11 && !strings.Contains(s, "/") && !strings.Contains(s, " ") {
		return s
	}
	if m := videoIDRe1.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	if m := videoIDRe2.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return ""
}

func extractPlaylistID(u string) string {
	if m := playlistIDRe1.FindStringSubmatch(u); len(m) > 1 {
		return m[1]
	}
	if m := playlistIDRe2.FindStringSubmatch(u); len(m) > 1 {
		return m[1]
	}
	return ""
}

func parseYTDuration(v map[string]any) int32 {
	if txt := digStr(v, "lengthText", "simpleText"); txt != "" {
		return parseTimeToSeconds(txt)
	}
	if label := digStr(v, "lengthText", "accessibility", "accessibilityData", "label"); label != "" {
		return parseLabelDuration(label)
	}
	return 0
}

func parseDuration(s string) int32 {
	parts := strings.Split(s, ":")
	var total, mul int32 = 0, 1
	for _, part := range slices.Backward(parts) {
		total += atoi(part) * mul
		mul *= 60
	}
	return total
}

func parseTimeToSeconds(s string) int32 {
	parts := strings.Split(s, ":")
	total := 0
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return int32(total)
}

func parseLabelDuration(s string) int32 {
	total := 0
	for _, m := range labelDurationRe.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(m[1])
		switch {
		case strings.HasPrefix(m[2], "hour"):
			total += n * 3600
		case strings.HasPrefix(m[2], "minute"):
			total += n * 60
		default:
			total += n
		}
	}
	return int32(total)
}

func dig(v any, path ...any) any {
	cur := v
	for _, p := range path {
		if cur == nil {
			return nil
		}
		switch k := p.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				return nil
			}
			cur = m[k]
		case int:
			a, ok := cur.([]any)
			if !ok || k < 0 || k >= len(a) {
				return nil
			}
			cur = a[k]
		}
	}
	return cur
}

func digStr(src any, path ...any) string {
	s, _ := dig(src, path...).(string)
	return s
}

func digArray(src any, path ...any) []map[string]any {
	arr, ok := dig(src, path...).([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func safeString(v any) string {
	s, _ := v.(string)
	return s
}

func atoi(s string) int32 {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		}
	}
	return int32(n)
}
