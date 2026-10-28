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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type apiData struct {
	Query    string
	ApiUrl   string
	APIKey   string
	Patterns map[string]*regexp.Regexp
}

var (
	ogTitleRe       = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']+)["']`)
	ogDescRe        = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:description["'][^>]+content=["']([^"']+)["']`)
	htmlTitleRe     = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	appleSongMetaRe = regexp.MustCompile(`(?i)<meta[^>]+property=["']music:song["'][^>]+content=["']([^"']+)["']`)
	spotifyTrackRe  = regexp.MustCompile(`"name"\s*:\s*"([^"]{2,100})"\s*,\s*"uri"\s*:\s*"spotify:track:[A-Za-z0-9]+"`)
)

var apiPatterns = map[string]*regexp.Regexp{
	utils.Apple:      regexp.MustCompile(`(?i)^https?:\/\/music\.apple\.com\/[a-zA-Z-]+\/(?:song\/(?:[^\/]+\/)?\d+|album\/[^\/]+\/\d+(?:\?i=\d+)?|playlist\/[^\/]+\/pl\.[\w.-]+|artist\/[^\/]+\/\d+)(?:\?.*)?$`),
	utils.Spotify:    regexp.MustCompile(`(?i)^(https?://)?([a-z0-9-]+\.)*(?:spotify\.com/(?:intl-[a-z-]+/)?(track|playlist|album|artist)/[a-zA-Z0-9]+|spotify\.link/[a-zA-Z0-9]+)(\?.*)?$`),
	utils.JioSaavn:   regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?(?:jiosaavn|saavn)\.com\/(?:s\/)?(song|album|playlist|featured)(?:\/[^\/]+)*\/([A-Za-z0-9_,-]+)(?:\/)?(?:\?.*)?$`),
	utils.Deezer:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.|deezer\.page\.link\/)?(?:deezer\.com\/(?:[a-z]{2}\/)?(track|album|playlist)\/(\d+)|[A-Za-z0-9]+)`),
	utils.SoundCloud: regexp.MustCompile(`(?i)^(https?://)?(www\.|m\.|on\.)?soundcloud\.com/[a-zA-Z0-9_-]+(/[a-zA-Z0-9._-]+)?(\?.*)?$`),
	"resso":          regexp.MustCompile(`(?i)^https?:\/\/(?:m\.|www\.)?resso\.com\/.+$`),
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

// resolveExternalMusicQueries extracts one or multiple song search queries (for tracks, playlists, or albums) from Spotify, Apple Music, Resso, SoundCloud, or Deezer.
func resolveExternalMusicQueries(rawURL string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	lowerURL := strings.ToLower(rawURL)

	// 1. Spotify fast public oEmbed + Embed Playlist/Album parser (Ported from Spotify.py)
	if strings.Contains(lowerURL, "spotify.com") || strings.Contains(lowerURL, "spotify.link") {
		if strings.Contains(lowerURL, "/playlist/") || strings.Contains(lowerURL, "/album/") || strings.Contains(lowerURL, "/artist/") {
			embedURL := strings.Replace(rawURL, "open.spotify.com/", "open.spotify.com/embed/", 1)
			if req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil); err == nil {
				req.Header.Set("User-Agent", defaultUserAgent)
				if resp, doErr := client.Do(req); doErr == nil {
					bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
					_ = resp.Body.Close()
					matches := spotifyTrackRe.FindAllStringSubmatch(string(bodyBytes), 15)
					var queries []string
					seen := make(map[string]bool)
					for _, m := range matches {
						if len(m) > 1 {
							q := strings.TrimSpace(html.UnescapeString(m[1]))
							if q != "" && !seen[q] {
								seen[q] = true
								queries = append(queries, q)
							}
						}
					}
					if len(queries) > 0 {
						return queries, nil
					}
				}
			}
		}

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
					return []string{strings.TrimSpace(odata.Title)}, nil
				}
			}
		}
	}

	// 2. SoundCloud fast public oEmbed (Ported from Soundcloud.py)
	if strings.Contains(lowerURL, "soundcloud.com") {
		scOembed := "https://soundcloud.com/oembed?format=json&url=" + url.QueryEscape(rawURL)
		if req, err := http.NewRequestWithContext(ctx, http.MethodGet, scOembed, nil); err == nil {
			req.Header.Set("User-Agent", defaultUserAgent)
			if resp, doErr := client.Do(req); doErr == nil {
				var scData struct {
					Title      string `json:"title"`
					AuthorName string `json:"author_name"`
				}
				_ = json.NewDecoder(io.LimitReader(resp.Body, 512*1024)).Decode(&scData)
				_ = resp.Body.Close()
				title := strings.TrimSpace(scData.Title)
				if title != "" {
					if scData.AuthorName != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(scData.AuthorName)) {
						title = strings.TrimSpace(title + " " + scData.AuthorName)
					}
					return []string{title}, nil
				}
			}
		}
	}

	// 3. Scrape OpenGraph & Apple Music / Resso tags (Ported from Apple.py & Resso.py)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	_ = resp.Body.Close()
	page := string(bodyBytes)

	// Check for Apple Music Playlist (<meta property="music:song" content="..."> from Apple.py)
	if strings.Contains(lowerURL, "music.apple.com") && strings.Contains(lowerURL, "/playlist/") {
		songLinks := appleSongMetaRe.FindAllStringSubmatch(page, 15)
		var queries []string
		for _, item := range songLinks {
			if len(item) < 2 {
				continue
			}
			content := strings.TrimRight(item[1], "/")
			var slug string
			if strings.Contains(content, "album/") {
				parts := strings.SplitN(content, "album/", 2)
				slug = strings.Split(parts[1], "/")[0]
			} else {
				parts := strings.Split(content, "/")
				if len(parts) >= 2 {
					slug = parts[len(parts)-2]
				}
			}
			cleanName := strings.TrimSpace(strings.ReplaceAll(slug, "-", " "))
			if cleanName != "" {
				queries = append(queries, cleanName)
			}
		}
		if len(queries) > 0 {
			return queries, nil
		}
	}

	var title, desc string
	if m := ogTitleRe.FindStringSubmatch(page); len(m) > 1 {
		title = html.UnescapeString(strings.TrimSpace(m[1]))
	} else if m := htmlTitleRe.FindStringSubmatch(page); len(m) > 1 {
		title = html.UnescapeString(strings.TrimSpace(m[1]))
	}
	if m := ogDescRe.FindStringSubmatch(page); len(m) > 1 {
		desc = html.UnescapeString(strings.TrimSpace(m[1]))
	}

	for _, suffix := range []string{" on Apple Music", " | Spotify", " - song and lyrics by ", " - song by ", " | SoundCloud"} {
		title = strings.ReplaceAll(title, suffix, " ")
	}
	// Resso & Spotify description artist extraction (`og:description` split on `·`)
	if desc != "" && strings.Contains(desc, "·") {
		parts := strings.Split(desc, "·")
		if len(parts) > 0 {
			artistPart := strings.TrimSpace(parts[0])
			artistPart = strings.TrimPrefix(artistPart, "Listen to ")
			if artistPart != "" && len(artistPart) < 60 && !strings.Contains(strings.ToLower(title), strings.ToLower(artistPart)) {
				title = strings.TrimSpace(title + " " + artistPart)
			}
		}
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("could not extract track title from URL")
	}
	return []string{title}, nil
}

// downloadSoundCloudDirect resolves or downloads SoundCloud links in a single step (Ported from Soundcloud.py).
func downloadSoundCloudDirect(rawURL string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := os.MkdirAll(config.DownloadsDir, 0755); err != nil {
		return "", err
	}
	outTmpl := filepath.Join(config.DownloadsDir, "sc_%(id)s.%(ext)s")
	cmd := exec.CommandContext(
		ctx,
		"yt-dlp",
		"--no-warnings",
		"--quiet",
		"--no-playlist",
		"--force-ipv4",
		"--no-check-certificates",
		"-f", "bestaudio/best",
		"-o", outTmpl,
		rawURL,
		"--print", "after_move:filepath",
	)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path != "" {
		if info, statErr := os.Stat(path); statErr == nil && info.Size() > 8192 {
			return path, nil
		}
	}
	return "", errors.New("soundcloud direct download empty")
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

	// Cookie-free / Key-free resolution via YouTube Innertube (parallelized for playlists/albums like stream.py)
	queries, err := resolveExternalMusicQueries(a.Query)
	if err != nil || len(queries) == 0 {
		return nil, fmt.Errorf("failed to resolve external music link: %w", err)
	}

	if len(queries) == 1 {
		tracks, sErr := searchYouTube(queries[0], 1)
		if sErr != nil || len(tracks) == 0 {
			return nil, errors.New("no matching track found on YouTube")
		}
		return &utils.PlatformTracks{Results: tracks[:1]}, nil
	}

	results := make([]utils.GetUrlTrack, len(queries))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i, q := range queries {
		wg.Add(1)
		go func(idx int, queryStr string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if found, sErr := searchYouTube(queryStr, 1); sErr == nil && len(found) > 0 {
				results[idx] = found[0]
			}
		}(i, q)
	}
	wg.Wait()

	var validTracks []utils.GetUrlTrack
	for _, tr := range results {
		if tr.Id != "" {
			validTracks = append(validTracks, tr)
		}
	}
	if len(validTracks) == 0 {
		return nil, errors.New("no matching tracks found on YouTube")
	}
	return &utils.PlatformTracks{Results: validTracks}, nil
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
	if strings.EqualFold(info.Platform, utils.SoundCloud) && strings.Contains(strings.ToLower(info.URL), "soundcloud.com") {
		if scPath, scErr := downloadSoundCloudDirect(info.URL); scErr == nil && scPath != "" {
			return scPath, nil
		}
	}

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
