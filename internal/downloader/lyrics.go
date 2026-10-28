package downloader

import (
	"ashokshau/tg-web/internal/config"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var ErrLyricsUnsupported = errors.New("lyrics source is not supported")
var ErrLyricsUnavailable = errors.New("lyrics are unavailable")

type Lyrics struct {
	Text string `json:"text"`
	LRC  string `json:"lrc"`
}

type LyricsQuery struct {
	URL      string
	Title    string
	Artist   string
	TrackID  string
	Duration int32
}

var (
	lyricsCacheMu sync.RWMutex
	lyricsCache   = make(map[string]*Lyrics)

	noiseBracketRe = regexp.MustCompile(`(?i)[\(\[\{][^\)\]\}]*(?:official|video|audio|lyric|lyrics|clip|hd|4k|hq|visualizer|remaster|live|mv|كليب|رسمي|فيديو|كلمات|حصري|حصريا|نسخة|أغنية|اغنية)[^\)\]\}]*[\)\]\}]`)
	noiseTailRe    = regexp.MustCompile(`(?i)(?:official\s+(?:music\s+)?video|official\s+audio|lyric\s+video|lyrics|prod\.?\s+by.*|ft\.?.*|feat\.?.*|#\S+)`)
	artistNoiseRe  = regexp.MustCompile(`(?i)(?:\s*-\s*topic|\s*vevo|\s*official(?:\s+channel)?)$`)
	multiSpaceRe   = regexp.MustCompile(`\s{2,}`)
)

func LyricsURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.User != nil || u.Port() != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", ErrLyricsUnsupported
	}
	u.Fragment = ""
	return u.String(), nil
}

func cleanTrackMetadata(rawTitle, rawArtist string) (cleanTitle, cleanArtist, fullQuery string) {
	t := strings.TrimSpace(rawTitle)
	a := strings.TrimSpace(rawArtist)

	t = noiseBracketRe.ReplaceAllString(t, " ")
	t = noiseTailRe.ReplaceAllString(t, " ")
	if idx := strings.Index(t, "|"); idx > 3 {
		t = t[:idx]
	}
	t = strings.Trim(t, " -–—|•:")
	t = multiSpaceRe.ReplaceAllString(t, " ")

	a = artistNoiseRe.ReplaceAllString(a, "")
	if idx := strings.Index(a, "|"); idx > 2 {
		a = a[:idx]
	}
	a = strings.TrimSpace(multiSpaceRe.ReplaceAllString(a, " "))

	for _, sep := range []string{" - ", " – ", " — "} {
		if parts := strings.SplitN(t, sep, 2); len(parts) == 2 {
			p0 := strings.TrimSpace(parts[0])
			p1 := strings.TrimSpace(parts[1])
			if p0 != "" && p1 != "" {
				if a == "" || strings.Contains(strings.ToLower(a), strings.ToLower(p0)) {
					a = p0
					t = p1
				} else if strings.Contains(strings.ToLower(a), strings.ToLower(p1)) {
					a = p1
					t = p0
				} else {
					a = p0
					t = p1
				}
				break
			}
		}
	}

	cleanTitle = strings.TrimSpace(t)
	cleanArtist = strings.TrimSpace(a)
	if cleanArtist != "" && !strings.Contains(strings.ToLower(cleanTitle), strings.ToLower(cleanArtist)) {
		fullQuery = strings.TrimSpace(cleanArtist + " " + cleanTitle)
	} else {
		fullQuery = cleanTitle
	}
	return cleanTitle, cleanArtist, fullQuery
}

func synthesizeLRCFromPlain(plain string, durationSec int32) string {
	rawLines := strings.Split(strings.ReplaceAll(plain, "
", "
"), "
")
	var lines []string
	for _, l := range rawLines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	dur := float64(durationSec)
	if dur <= 15 {
		dur = float64(len(lines)) * 3.8
	}
	startOffset := 2.0
	usable := math.Max(10.0, dur-5.0)
	step := usable / float64(len(lines))

	var sb strings.Builder
	for i, line := range lines {
		sec := startOffset + float64(i)*step
		mins := int(sec) / 60
		rem := sec - float64(mins*60)
		fmt.Fprintf(&sb, "[%02d:%05.2f] %s
", mins, rem, line)
	}
	return strings.TrimSpace(sb.String())
}

type lrclibItem struct {
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

func fetchFromLRCLIB(ctx context.Context, cleanTitle, cleanArtist, fullQuery, rawTitle string, durationSec int32) *Lyrics {
	var endpoints []string
	if cleanTitle != "" && cleanArtist != "" {
		v := url.Values{}
		v.Set("track_name", cleanTitle)
		v.Set("artist_name", cleanArtist)
		endpoints = append(endpoints, "https://lrclib.net/api/search?"+v.Encode())
	}
	if fullQuery != "" {
		v := url.Values{}
		v.Set("q", fullQuery)
		endpoints = append(endpoints, "https://lrclib.net/api/search?"+v.Encode())
	}
	if cleanTitle != "" && cleanTitle != fullQuery {
		v := url.Values{}
		v.Set("q", cleanTitle)
		endpoints = append(endpoints, "https://lrclib.net/api/search?"+v.Encode())
	}
	if rawTitle != "" && rawTitle != fullQuery && rawTitle != cleanTitle {
		v := url.Values{}
		v.Set("q", rawTitle)
		endpoints = append(endpoints, "https://lrclib.net/api/search?"+v.Encode())
	}

	var bestPlain string
	for _, ep := range endpoints {
		reqCtx, cancel := context.WithTimeout(ctx, 4500*time.Millisecond)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, ep, nil)
		if err != nil {
			cancel()
			continue
		}
		req.Header.Set("User-Agent", "TeleMusic-WebPlayer/2.0 (https://github.com/AshokShau/telegram-web-player)")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			cancel()
			continue
		}
		var items []lrclibItem
		decErr := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&items)
		_ = resp.Body.Close()
		cancel()
		if decErr != nil || resp.StatusCode != http.StatusOK || len(items) == 0 {
			continue
		}

		bestIdx := -1
		bestDiff := 999999.0
		for i, it := range items {
			if strings.TrimSpace(it.SyncedLyrics) != "" {
				diff := 0.0
				if durationSec > 0 && it.Duration > 0 {
					diff = math.Abs(it.Duration - float64(durationSec))
				}
				if bestIdx == -1 || diff < bestDiff {
					bestIdx = i
					bestDiff = diff
				}
			} else if bestPlain == "" && strings.TrimSpace(it.PlainLyrics) != "" {
				bestPlain = strings.TrimSpace(it.PlainLyrics)
			}
		}

		if bestIdx >= 0 {
			chosen := items[bestIdx]
			return &Lyrics{
				Text: strings.TrimSpace(chosen.PlainLyrics),
				LRC:  strings.TrimSpace(chosen.SyncedLyrics),
			}
		}
	}

	if bestPlain != "" {
		return &Lyrics{
			Text: bestPlain,
			LRC:  synthesizeLRCFromPlain(bestPlain, durationSec),
		}
	}
	return nil
}

func fetchFromYouTubeCaptions(ctx context.Context, videoID string, durationSec int32) (*Lyrics, string, string) {
	if len(videoID) != 11 {
		return nil, "", ""
	}
	payload := map[string]any{
		"videoId": videoID,
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "WEB",
				"clientVersion": "2.20250101.01.00",
				"hl":            "ar",
				"gl":            "EG",
			},
		},
		"contentCheckOk": true,
		"racyCheckOk":    true,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, "", ""
	}

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, "https://www.youtube.com/youtubei/v1/player?prettyPrint=false", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, "", ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", ""
	}
	var data map[string]any
	decErr := json.NewDecoder(io.LimitReader(resp.Body, 4*1024*1024)).Decode(&data)
	_ = resp.Body.Close()
	if decErr != nil || resp.StatusCode != http.StatusOK {
		return nil, "", ""
	}

	vTitle := digStr(data, "videoDetails", "title")
	vAuthor := digStr(data, "videoDetails", "author")
	vDesc := digStr(data, "videoDetails", "shortDescription")
	if durationSec <= 0 {
		durationSec = atoi(digStr(data, "videoDetails", "lengthSeconds"))
	}

	tracks := digArray(data, "captions", "playerCaptionsTracklistRenderer", "captionTracks")
	if len(tracks) > 0 {
		// Pick manual caption track first (preferring ar or en), otherwise first available
		var chosenURL string
		for _, ct := range tracks {
			u := safeString(ct["baseUrl"])
			kind := safeString(ct["kind"])
			lang := safeString(ct["languageCode"])
			if u == "" {
				continue
			}
			if kind != "asr" && (strings.HasPrefix(lang, "ar") || strings.HasPrefix(lang, "en")) {
				chosenURL = u
				break
			}
			if chosenURL == "" && kind != "asr" {
				chosenURL = u
			}
		}
		if chosenURL == "" {
			chosenURL = safeString(tracks[0]["baseUrl"])
		}

		if chosenURL != "" {
			if !strings.Contains(chosenURL, "fmt=") {
				chosenURL += "&fmt=json3"
			}
			capReq, cErr := http.NewRequestWithContext(reqCtx, http.MethodGet, chosenURL, nil)
			if cErr == nil {
				capReq.Header.Set("User-Agent", defaultUserAgent)
				if capResp, doErr := client.Do(capReq); doErr == nil {
					var capData struct {
						Events []struct {
							TStartMs int64 `json:"tStartMs"`
							Segs     []struct {
								UTF8 string `json:"utf8"`
							} `json:"segs"`
						} `json:"events"`
					}
					_ = json.NewDecoder(io.LimitReader(capResp.Body, 2*1024*1024)).Decode(&capData)
					_ = capResp.Body.Close()

					var lrcBuf, txtBuf strings.Builder
					lastLine := ""
					lineCount := 0
					for _, ev := range capData.Events {
						var lineBuilder strings.Builder
						for _, seg := range ev.Segs {
							lineBuilder.WriteString(seg.UTF8)
						}
						line := strings.TrimSpace(strings.ReplaceAll(lineBuilder.String(), "
", " "))
						if line == "" || line == lastLine || line == "[Music]" || line == "[موسيقى]" {
							continue
						}
						lastLine = line
						sec := float64(ev.TStartMs) / 1000.0
						mins := int(sec) / 60
						rem := sec - float64(mins*60)
						fmt.Fprintf(&lrcBuf, "[%02d:%05.2f] %s
", mins, rem, line)
						txtBuf.WriteString(line + "
")
						lineCount++
					}
					if lineCount >= 3 {
						return &Lyrics{
							Text: strings.TrimSpace(txtBuf.String()),
							LRC:  strings.TrimSpace(lrcBuf.String()),
						}, vTitle, vAuthor
					}
				}
			}
		}
	}

	// Check if shortDescription contains song lyrics
	if vDesc != "" {
		var descLines []string
		for _, line := range strings.Split(strings.ReplaceAll(vDesc, "
", "
"), "
") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.Contains(trimmed, "http://") || strings.Contains(trimmed, "https://") ||
				strings.HasPrefix(trimmed, "#") || strings.Contains(strings.ToLower(trimmed), "subscribe") ||
				strings.Contains(strings.ToLower(trimmed), "follow") || strings.Contains(strings.ToLower(trimmed), "instagram") ||
				strings.Contains(strings.ToLower(trimmed), "tiktok") || strings.Contains(strings.ToLower(trimmed), "facebook") {
				continue
			}
			if len([]rune(trimmed)) >= 4 && len([]rune(trimmed)) <= 90 {
				descLines = append(descLines, trimmed)
			}
		}
		if len(descLines) >= 6 {
			plain := strings.Join(descLines, "
")
			return &Lyrics{
				Text: plain,
				LRC:  synthesizeLRCFromPlain(plain, durationSec),
			}, vTitle, vAuthor
		}
	}

	return nil, vTitle, vAuthor
}

func FetchLyricsWithMeta(ctx context.Context, q LyricsQuery) (*Lyrics, error) {
	videoID := strings.TrimSpace(q.TrackID)
	if len(videoID) != 11 && q.URL != "" {
		videoID = extractVideoID(q.URL)
	}

	cacheKey := fmt.Sprintf("%s|%s|%s", videoID, strings.TrimSpace(q.Title), strings.TrimSpace(q.Artist))
	if cacheKey != "||" {
		lyricsCacheMu.RLock()
		if cached, ok := lyricsCache[cacheKey]; ok && cached != nil {
			lyricsCacheMu.RUnlock()
			return cached, nil
		}
		lyricsCacheMu.RUnlock()
	}

	saveCache := func(res *Lyrics) *Lyrics {
		if res != nil && (res.LRC != "" || res.Text != "") && cacheKey != "||" {
			lyricsCacheMu.Lock()
			if len(lyricsCache) > 200 {
				lyricsCache = make(map[string]*Lyrics)
			}
			lyricsCache[cacheKey] = res
			lyricsCacheMu.Unlock()
		}
		return res
	}

	rawTitle := strings.TrimSpace(q.Title)
	rawArtist := strings.TrimSpace(q.Artist)

	// If title is missing and we have a YouTube video ID, fetch title via oEmbed or Innertube
	var ytCaptionResult *Lyrics
	if len(videoID) == 11 {
		var ytTitle, ytAuthor string
		ytCaptionResult, ytTitle, ytAuthor = fetchFromYouTubeCaptions(ctx, videoID, q.Duration)
		if rawTitle == "" && ytTitle != "" {
			rawTitle = ytTitle
		}
		if rawArtist == "" && ytAuthor != "" {
			rawArtist = ytAuthor
		}
	}

	cleanTitle, cleanArtist, fullQuery := cleanTrackMetadata(rawTitle, rawArtist)

	// 1. Try LRCLIB for studio-grade synced LRC lyrics
	if cleanTitle != "" || fullQuery != "" {
		if lrcRes := fetchFromLRCLIB(ctx, cleanTitle, cleanArtist, fullQuery, rawTitle, q.Duration); lrcRes != nil && (lrcRes.LRC != "" || lrcRes.Text != "") {
			return saveCache(lrcRes), nil
		}
	}

	// 2. Use YouTube Innertube Timed Captions / Description if available
	if ytCaptionResult != nil && (ytCaptionResult.LRC != "" || ytCaptionResult.Text != "") {
		return saveCache(ytCaptionResult), nil
	}

	// 3. Optional external API fallback if APIKey is configured
	if q.URL != "" && config.ApiUrl != "" && config.ApiKey != "" {
		if canonical, err := LyricsURL(q.URL); err == nil {
			reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			endpoint := strings.TrimRight(config.ApiUrl, "/") + "/api/lyrics?" + url.Values{"url": {canonical}}.Encode()
			if req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil); reqErr == nil {
				req.Header.Set("X-API-Key", config.ApiKey)
				req.Header.Set("Accept", "application/json")
				if resp, doErr := client.Do(req); doErr == nil {
					body, _ := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
					_ = resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						var apiLyrics Lyrics
						if json.Unmarshal(body, &apiLyrics) == nil && (apiLyrics.LRC != "" || apiLyrics.Text != "") {
							if apiLyrics.LRC == "" && apiLyrics.Text != "" {
								apiLyrics.LRC = synthesizeLRCFromPlain(apiLyrics.Text, q.Duration)
							}
							return saveCache(&apiLyrics), nil
						}
					}
				}
			}
		}
	}

	return &Lyrics{}, nil
}

// FetchLyrics keeps backwards compatibility with callers passing only trackURL.
func FetchLyrics(ctx context.Context, trackURL string) (*Lyrics, error) {
	return FetchLyricsWithMeta(ctx, LyricsQuery{URL: trackURL})
}
