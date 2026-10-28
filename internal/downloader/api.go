/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package downloader

import (
	"ashokshau/tg-web/internal/config"
	"ashokshau/tg-web/internal/utils"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type apiData struct {
	Query    string
	ApiUrl   string
	APIKey   string
	Patterns map[string]*regexp.Regexp
}

var (
	ogTitleRe   = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']+)["']`)
	ogDescRe    = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:description["'][^>]+content=["']([^"']+)["']`)
	htmlTitleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

var apiPatterns = map[string]*regexp.Regexp{
	utils.Apple:      regexp.MustCompile(`(?i)^https?:\/\/music\.apple\.com\/[a-zA-Z-]+\/(?:song\/(?:[^\/]+\/)?\d+|album\/[^\/]+\/\d+(?:\?i=\d+)?|playlist\/[^\/]+\/pl\.[\w.-]+|artist\/[^\/]+\/\d+)(?:\?.*)?$`),
	utils.Spotify:    regexp.MustCompile(`(?i)^(https?://)?([a-z0-9-]+\.)*spotify\.com/(track|playlist|album|artist)/[a-zA-Z0-9]+(\?.*)?$`),
	utils.JioSaavn:   regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?(?:jiosaavn|saavn)\.com\/(?:s\/)?(song|album|playlist|featured)(?:\/[^\/]+)*\/([A-Za-z0-9_,-]+)(?:\/)?(?:\?.*)?$`),
	utils.Deezer:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?deezer\.com\/(?:[a-z]{2}\/)?(track|album|playlist)\/(\d+)`),
	utils.SoundCloud: regexp.MustCompile(`(?i)^(https?://)?(www\.|m\.|on\.)?soundcloud\.com/[a-zA-Z0-9_-]+(/[a-zA-Z0-9._-]+)?(\?.*)?$`),
	utils.Gaana:      regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?gaana\.com\/(song|album|playlist|artist)\/([A-Za-z0-9\-]+)`),
	utils.Tidal:      regexp.MustCompile(`(?i)https?:\/\/(?:www\.|listen\.)?tidal\.com\/(?:browse\/)?(track|album|playlist)\/([a-zA-Z0-9-]+)(?:[\/?].*)?`),
	utils.MXPlayer:   regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?mxplayer\.in\/(?:show|movie)\/.*`),
	utils.Twitch:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.|m\.)?twitch\.tv\/(?:videos|[\w._-]+\/video)\/\d+`),
	utils.TwitchClip: regexp.MustCompile(
		`(?i)https?:\/\/(?:www\.|m\.)?(?:` +
			`twitch\.tv\/clip\/[\w-]+|` +
			`clips\.twitch\.tv\/[\w-]+|` +
			`twitch\.tv\/[\w-]+\/clip\/[\w-]+` +
			`)`,
	),
	utils.Kick:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?kick\.com\/[\w._-]+\/videos\/[a-fA-F0-9-]+`),
	utils.KickClip: regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?kick\.com\/[\w._-]+\/clips\/[\w-]+`),
}

func newApiData(query string) *apiData {
	return &apiData{
		Query:    strings.TrimSpace(query),
		ApiUrl:   strings.TrimRight(config.ApiUrl, "/"),
		APIKey:   config.ApiKey,
		Patterns: apiPatterns,
	}
}

func (a *apiData) isValid() bool {
	if a.Query == "" {
		return false
	}
	for _, pattern := range a.Patterns {
		if pattern.MatchString(a.Query) {
			return true
		}
	}
	return false
}

func resolveExternalMusicTitle(rawURL string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	// 1. Spotify fast public oEmbed
	if strings.Contains(strings.ToLower(rawURL), "spotify.com") {
		oembedURL := "https://open.spotify.com/oembed?url=" + url.QueryEscape(rawURL)
		if req, err := http.NewRequestWithContext(ctx, http.MethodGet, oembedURL, nil); err == nil {
			req.Header.Set("User-Agent", defaultUserAgent)
			if resp, doErr := client.Do(req); doErr == nil {
				var odata struct {
					Title string `json:"title"`
				}
				_ = json.NewDecoder(io.LimitReader(resp.Body, 512*1024)).Decode(&odata)
				_ = resp.Body.Close()
				if strings.TrimSpace(odata.Title) != "" {
					return strings.TrimSpace(odata.Title), nil
				}
			}
		}
	}

	// 2. Scrape OpenGraph title/description for Spotify, Apple Music, SoundCloud, Deezer, etc.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	_ = resp.Body.Close()
	page := string(bodyBytes)

	var title, desc string
	if m := ogTitleRe.FindStringSubmatch(page); len(m) > 1 {
		title = html.UnescapeString(strings.TrimSpace(m[1]))
	} else if m := htmlTitleRe.FindStringSubmatch(page); len(m) > 1 {
		title = html.UnescapeString(strings.TrimSpace(m[1]))
	}
	if m := ogDescRe.FindStringSubmatch(page); len(m) > 1 {
		desc = html.UnescapeString(strings.TrimSpace(m[1]))
	}

	for _, suffix := range []string{" on Apple Music", " | Spotify", " - song and lyrics by ", " - song by "} {
		title = strings.ReplaceAll(title, suffix, " ")
	}
	if desc != "" && strings.Contains(desc, "·") {
		parts := strings.Split(desc, "·")
		if len(parts) > 0 && !strings.Contains(strings.ToLower(title), strings.ToLower(strings.TrimSpace(parts[0]))) {
			title = strings.TrimSpace(title + " " + strings.TrimSpace(parts[0]))
		}
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return "", errors.New("could not extract track title from URL")
	}
	return title, nil
}

func (a *apiData) getInfo() (*utils.PlatformTracks, error) {
	if !a.isValid() {
		return nil, errors.New("the provided URL is invalid or the platform is not supported")
	}

	if a.ApiUrl != "" && a.APIKey != "" {
		fullURL := fmt.Sprintf("%s/api/get_url?%s", a.ApiUrl, url.Values{"url": {a.Query}}.Encode())
		if resp, err := sendRequest(http.MethodGet, fullURL, nil, map[string]string{"X-API-Key": a.APIKey}); err == nil {
			defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)
			if resp.StatusCode == http.StatusOK {
				var data utils.PlatformTracks
				if json.NewDecoder(resp.Body).Decode(&data) == nil && len(data.Results) > 0 {
					return &data, nil
				}
			}
		}
	}

	// Cookie-free / Key-free fallback via YouTube Innertube
	searchQuery, err := resolveExternalMusicTitle(a.Query)
	if err != nil || searchQuery == "" {
		return nil, fmt.Errorf("failed to resolve external music link: %w", err)
	}
	tracks, err := searchYouTube(searchQuery, 1)
	if err != nil || len(tracks) == 0 {
		return nil, errors.New("no matching track found on YouTube")
	}
	return &utils.PlatformTracks{Results: tracks[:1]}, nil
}

func (a *apiData) search() (*utils.PlatformTracks, error) {
	if a.isValid() {
		return a.getInfo()
	}

	if a.ApiUrl == "" || a.APIKey == "" {
		tracks, err := searchYouTube(a.Query, 10)
		if err != nil {
			return nil, err
		}
		return &utils.PlatformTracks{Results: tracks}, nil
	}

	fullURL := fmt.Sprintf("%s/api/search?%s", a.ApiUrl, url.Values{
		"query": {a.Query},
		"limit": {"5"},
	}.Encode())

	resp, err := sendRequest(http.MethodGet, fullURL, nil, map[string]string{"X-API-Key": a.APIKey})
	if err != nil {
		return nil, fmt.Errorf("the search request failed: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code during search: %s", resp.Status)
	}

	var data utils.PlatformTracks
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		slog.Warn("Failed to decode search response", "error", err)
		return nil, fmt.Errorf("failed to decode the search response: %w", err)
	}

	return &data, nil
}

func (a *apiData) getTrack() (*utils.TrackInfo, error) {
	if a.ApiUrl != "" && a.APIKey != "" {
		fullURL := fmt.Sprintf("%s/api/track?%s", a.ApiUrl, url.Values{"url": {a.Query}}.Encode())
		if resp, err := sendRequest(http.MethodGet, fullURL, nil, map[string]string{"X-API-Key": a.APIKey}); err == nil {
			defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)
			if resp.StatusCode == http.StatusOK {
				var data utils.TrackInfo
				if json.NewDecoder(resp.Body).Decode(&data) == nil && data.Id != "" {
					return &data, nil
				}
			}
		}
	}

	info, err := a.getInfo()
	if err != nil || info == nil || len(info.Results) == 0 {
		return nil, fmt.Errorf("getTrack fallback failed: %w", err)
	}
	first := info.Results[0]
	return &utils.TrackInfo{
		Id:       first.Id,
		URL:      first.Url,
		Platform: utils.YouTube,
	}, nil
}

func (a *apiData) downloadTrack(info *utils.TrackInfo, video bool) (string, error) {
	yt := newYouTubeData(info.URL)
	if info.Platform == utils.YouTube || len(info.Id) == 11 {
		return yt.downloadTrack(info, video)
	}

	filePath, err := processDownload(info)
	if err != nil {
		return yt.downloadTrack(info, video)
	}

	if strings.EqualFold(info.Platform, utils.Spotify) && info.Key == "" {
		return DlSpotifyAudio(info.Id, filePath)
	}

	if a.ApiUrl != "" && strings.Contains(a.ApiUrl, filePath) {
		return downloadFile(filePath, "", false)
	}

	return filePath, nil
}

func (a *apiData) getRecommendations(ctx context.Context, rawURL string) (*utils.PlatformTracks, error) {
	if a.ApiUrl != "" && a.APIKey != "" {
		urlToUse := rawURL
		if urlToUse == "" {
			urlToUse = a.Query
		}
		if urlToUse != "" {
			fullURL := fmt.Sprintf("%s/api/recommendations?%s", a.ApiUrl, url.Values{"url": {urlToUse}}.Encode())
			if resp, err := sendRequest(http.MethodGet, fullURL, nil, map[string]string{"X-API-Key": a.APIKey}); err == nil {
				defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)
				if resp.StatusCode == http.StatusOK {
					var data utils.PlatformTracks
					if json.NewDecoder(resp.Body).Decode(&data) == nil && len(data.Results) > 0 {
						return &data, nil
					}
				}
			}
		}
	}

	info, err := a.getInfo()
	if err != nil || info == nil || len(info.Results) == 0 {
		return nil, errors.New("recommendations unavailable")
	}
	return GetYouTubeMixPlaylist(ctx, "RD"+info.Results[0].Id)
}
