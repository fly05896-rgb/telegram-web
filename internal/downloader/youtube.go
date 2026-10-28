package downloader

import (
	"ashokshau/tg-web/internal/config"
	"ashokshau/tg-web/internal/utils"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type youTubeData struct {
	Query    string
	ApiUrl   string
	APIKey   string
	Patterns map[string]*regexp.Regexp
}

type inflightDl struct {
	done chan struct{}
	path string
	err  error
}

var (
	inflightMu        sync.Mutex
	inflightDownloads = make(map[string]*inflightDl)
)

const (
	fastAndroidUA     = "com.google.android.youtube/21.26.364 (Linux; U; Android 11) gzip"
	fastVisionUA      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 15_7_3) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Safari/605.1.15"
	fastWebEmbeddedUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.5 Safari/605.1.15,gzip(gfe)"
	fastTVSamsungUA   = "Mozilla/5.0 (SMART-TV; Linux; Tizen 2.4.0) AppleWebKit/538.1 (KHTML, like Gecko) Version/2.4.0 TV Safari/538.1"
)

var youtubePatterns = map[string]*regexp.Regexp{
	"youtube":   regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?youtube\.com/.*`),
	"youtu_be":  regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?youtu\.be/.*`),
	"yt_music":  regexp.MustCompile(`(?i)^(?:https?://)?music\.youtube\.com/.*`),
	"yt_shorts": regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?youtube\.com/shorts/.*`),
}

func newYouTubeData(query string) *youTubeData {
	return &youTubeData{
		Query:    strings.TrimSpace(query),
		ApiUrl:   strings.TrimRight(config.ApiUrl, "/"),
		APIKey:   config.ApiKey,
		Patterns: youtubePatterns,
	}
}

func (y *youTubeData) isValid() bool {
	if y.Query == "" {
		slog.Info("The query or patterns are empty.")
		return false
	}

	for _, pattern := range y.Patterns {
		if pattern.MatchString(y.Query) {
			return true
		}
	}
	return false
}

func findCachedYouTubeFile(videoID string) string {
	if videoID == "" {
		return ""
	}
	for _, ext := range []string{".m4a", ".mp4", ".webm", ".opus", ".mp3"} {
		candidate := filepath.Join(config.DownloadsDir, videoID+ext)
		if info, err := os.Stat(candidate); err == nil && info.Size() > 32768 {
			return candidate
		}
	}
	return ""
}

// deduplicateYouTubeDownload ensures concurrent requests for the same videoID share a single download.
func deduplicateYouTubeDownload(key string, fn func() (string, error)) (string, error) {
	inflightMu.Lock()
	if existing, ok := inflightDownloads[key]; ok {
		inflightMu.Unlock()
		<-existing.done
		return existing.path, existing.err
	}
	call := &inflightDl{done: make(chan struct{})}
	inflightDownloads[key] = call
	inflightMu.Unlock()

	defer func() {
		close(call.done)
		inflightMu.Lock()
		delete(inflightDownloads, key)
		inflightMu.Unlock()
	}()

	call.path, call.err = fn()
	return call.path, call.err
}

// PrefetchYouTubeAudio downloads a YouTube track into the RAM disk in the background.
func PrefetchYouTubeAudio(videoID string) {
	if len(videoID) != 11 || findCachedYouTubeFile(videoID) != "" {
		return
	}
	_, _ = deduplicateYouTubeDownload("yt:"+videoID+":audio", func() (string, error) {
		return downloadFastInnertube(videoID, false)
	})
}

type fastStreamFormat struct {
	URL           string
	UserAgent     string
	Ext           string
	ContentLength int64
}

func parseClenFromURL(rawURL string) int64 {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}
	if clenStr := u.Query().Get("clen"); clenStr != "" {
		if n, pErr := strconv.ParseInt(clenStr, 10, 64); pErr == nil && n > 0 {
			return n
		}
	}
	return 0
}

func extractFastInnertubeStream(ctx context.Context, videoID string, video bool) (*fastStreamFormat, error) {
	if getVisitorData() == "" {
		ensureVisitorData(ctx)
	}
	vData := getVisitorData()

	type clientSpec struct {
		name               string
		clientID           string
		clientVer          string
		userAgent          string
		preferProgressive  bool
		context            map[string]any
	}
	clients := []clientSpec{
		{
			name:              "VISIONOS",
			clientID:          "101",
			clientVer:         "1.02",
			userAgent:         fastVisionUA,
			preferProgressive: false,
			context: map[string]any{
				"client": map[string]any{
					"clientName":       "VISIONOS",
					"clientVersion":    "1.02",
					"deviceMake":       "Apple",
					"deviceModel":      "RealityDevice17,1",
					"osName":           "visionOS",
					"osVersion":        "26.5.23O471",
					"hl":               "en",
					"gl":               "US",
					"timeZone":         "UTC",
					"utcOffsetMinutes": 0,
				},
			},
		},
		{
			name:              "ANDROID",
			clientID:          "3",
			clientVer:         "21.26.364",
			userAgent:         fastAndroidUA,
			preferProgressive: true, // Golden Rule from Youtube.py: itag=18 in formats is 100% exempt from GVS PO Token on ANDROID!
			context: map[string]any{
				"client": map[string]any{
					"clientName":        "ANDROID",
					"clientVersion":     "21.26.364",
					"androidSdkVersion": 30,
					"osName":            "Android",
					"osVersion":         "11",
					"hl":                "en",
					"gl":                "US",
					"timeZone":          "UTC",
					"utcOffsetMinutes":  0,
				},
			},
		},
		{
			name:              "WEB_EMBEDDED_PLAYER",
			clientID:          "56",
			clientVer:         "1.20260101.01.00",
			userAgent:         fastWebEmbeddedUA,
			preferProgressive: true,
			context: map[string]any{
				"client": map[string]any{
					"clientName":    "WEB_EMBEDDED_PLAYER",
					"clientVersion": "1.20260101.01.00",
					"hl":            "en",
					"gl":            "US",
				},
				"thirdParty": map[string]any{
					"embedUrl": "https://www.reddit.com/",
				},
			},
		},
	}

	endpoint := "https://www.youtube.com/youtubei/v1/player?prettyPrint=false"
	for _, spec := range clients {
		if vData != "" {
			if cMap, ok := spec.context["client"].(map[string]any); ok {
				cMap["visitorData"] = vData
			}
		}
		payload := map[string]any{
			"videoId": videoID,
			"context": spec.context,
			"playbackContext": map[string]any{
				"contentPlaybackContext": map[string]any{
					"html5Preference": "HTML5_PREF_WANTS",
				},
			},
			"contentCheckOk": true,
			"racyCheckOk":    true,
		}
		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			continue
		}

		reqCtx, cancel := context.WithTimeout(ctx, 4500*time.Millisecond)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			cancel()
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", spec.userAgent)
		req.Header.Set("X-YouTube-Client-Name", spec.clientID)
		req.Header.Set("X-YouTube-Client-Version", spec.clientVer)
		req.Header.Set("Origin", "https://www.youtube.com")
		if vData != "" {
			req.Header.Set("X-Goog-Visitor-Id", vData)
		}

		resp, err := client.Do(req)
		if err != nil {
			cancel()
			continue
		}
		var data map[string]any
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 4*1024*1024)).Decode(&data)
		_ = resp.Body.Close()
		cancel()
		if decodeErr != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		if newVData := digStr(data, "responseContext", "visitorData"); newVData != "" && vData == "" {
			vData = newVData
			setVisitorData(newVData)
		}

		if tr := mapPlayerToTrack(data); tr.Id != "" && tr.Title != "" {
			StoreTrackMeta(tr)
		}

		if digStr(data, "playabilityStatus", "status") != "OK" {
			continue
		}

		streamingData, ok := data["streamingData"].(map[string]any)
		if !ok {
			continue
		}

		progFormats := digArray(streamingData, "formats")
		adaptFormats := digArray(streamingData, "adaptiveFormats")

		var allFormats []map[string]any
		if video || spec.preferProgressive {
			allFormats = append(allFormats, progFormats...)
			allFormats = append(allFormats, adaptFormats...)
		} else {
			allFormats = append(allFormats, adaptFormats...)
			allFormats = append(allFormats, progFormats...)
		}

		var best *fastStreamFormat
		for _, f := range allFormats {
			u := safeString(f["url"])
			if u == "" {
				continue
			}
			itagFloat, _ := f["itag"].(float64)
			itag := int(itagFloat)
			mimeType := safeString(f["mimeType"])
			clen, _ := strconv.ParseInt(safeString(f["contentLength"]), 10, 64)
			if clen == 0 {
				clen = parseClenFromURL(u)
			}

			if !video {
				if spec.preferProgressive && itag == 18 {
					return &fastStreamFormat{URL: u, UserAgent: spec.userAgent, Ext: ".mp4", ContentLength: clen}, nil
				}
				if !spec.preferProgressive && itag == 140 {
					return &fastStreamFormat{URL: u, UserAgent: spec.userAgent, Ext: ".m4a", ContentLength: clen}, nil
				}
				if itag == 18 || itag == 140 {
					ext := ".m4a"
					if itag == 18 {
						ext = ".mp4"
					}
					best = &fastStreamFormat{URL: u, UserAgent: spec.userAgent, Ext: ext, ContentLength: clen}
				} else if best == nil && (itag == 251 || itag == 250 || itag == 139 || strings.HasPrefix(mimeType, "audio/")) {
					ext := ".m4a"
					if strings.Contains(mimeType, "webm") {
						ext = ".webm"
					}
					best = &fastStreamFormat{URL: u, UserAgent: spec.userAgent, Ext: ext, ContentLength: clen}
				}
			} else {
				if itag == 22 || itag == 18 {
					return &fastStreamFormat{URL: u, UserAgent: spec.userAgent, Ext: ".mp4", ContentLength: clen}, nil
				}
			}
		}
		if best != nil {
			return best, nil
		}
	}

	return nil, errors.New("no direct innertube stream available")
}

func downloadFastInnertube(videoID string, video bool) (string, error) {
	if cached := findCachedYouTubeFile(videoID); cached != "" {
		return cached, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	stream, err := extractFastInnertubeStream(ctx, videoID, video)
	if err != nil || stream == nil || stream.URL == "" {
		return "", err
	}

	if err := os.MkdirAll(config.DownloadsDir, 0755); err != nil {
		return "", err
	}

	targetPath := filepath.Join(config.DownloadsDir, videoID+stream.Ext)
	tempPath := fmt.Sprintf("%s.%d.part", targetPath, time.Now().UnixNano())
	defer func() { _ = os.Remove(tempPath) }()

	if stream.ContentLength >= 512*1024 && stream.ContentLength <= 150*1024*1024 {
		if err := downloadParallelRanges(ctx, stream.URL, stream.UserAgent, tempPath, stream.ContentLength); err == nil {
			if err := os.Rename(tempPath, targetPath); err == nil {
				return targetPath, nil
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stream.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", stream.UserAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return "", fmt.Errorf("stream status %d", resp.StatusCode)
	}

	out, err := os.Create(tempPath)
	if err != nil {
		return "", err
	}
	buf := make([]byte, 256*1024)
	_, copyErr := io.CopyBuffer(out, resp.Body, buf)
	_ = out.Close()
	if copyErr != nil {
		return "", copyErr
	}

	if info, statErr := os.Stat(tempPath); statErr != nil || info.Size() < 16384 {
		return "", errors.New("downloaded stream too small")
	}

	if err := os.Rename(tempPath, targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

func downloadParallelRanges(ctx context.Context, rawURL, userAgent, destPath string, totalSize int64) error {
	outFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = outFile.Close() }()

	if err := outFile.Truncate(totalSize); err != nil {
		return err
	}

	const chunkSize int64 = 1024 * 1024 // 1 MB chunks
	numChunks := int((totalSize + chunkSize - 1) / chunkSize)
	workers := 10
	if numChunks < workers {
		workers = numChunks
	}

	jobs := make(chan int, numChunks)
	for i := 0; i < numChunks; i++ {
		jobs <- i
	}
	close(jobs)

	ctxChunk, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 128*1024)
			for idx := range jobs {
				if ctxChunk.Err() != nil {
					return
				}
				start := int64(idx) * chunkSize
				end := start + chunkSize - 1
				if end >= totalSize {
					end = totalSize - 1
				}

				req, reqErr := http.NewRequestWithContext(ctxChunk, http.MethodGet, rawURL, nil)
				if reqErr != nil {
					errOnce.Do(func() { firstErr = reqErr; cancel() })
					return
				}
				req.Header.Set("User-Agent", userAgent)
				req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

				resp, doErr := client.Do(req)
				if doErr != nil {
					errOnce.Do(func() { firstErr = doErr; cancel() })
					return
				}
				if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
					_ = resp.Body.Close()
					errOnce.Do(func() { firstErr = fmt.Errorf("range status %d", resp.StatusCode); cancel() })
					return
				}

				offset := start
				for {
					n, rErr := resp.Body.Read(buf)
					if n > 0 {
						if _, wErr := outFile.WriteAt(buf[:n], offset); wErr != nil {
							_ = resp.Body.Close()
							errOnce.Do(func() { firstErr = wErr; cancel() })
							return
						}
						offset += int64(n)
					}
					if rErr == io.EOF {
						break
					}
					if rErr != nil {
						_ = resp.Body.Close()
						errOnce.Do(func() { firstErr = rErr; cancel() })
						return
					}
				}
				_ = resp.Body.Close()
			}
		}()
	}

	wg.Wait()
	return firstErr
}

func (y *youTubeData) getInfo() (*utils.PlatformTracks, error) {
	if !y.isValid() {
		return nil, errors.New("the provided URL is invalid or the platform is not supported")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	y.Query = normalizeYouTubeURL(y.Query)
	videoID := extractVideoID(y.Query)
	playlistID := extractPlaylistID(y.Query)

	switch {
	case playlistID != "" && videoID == "":
		if strings.HasPrefix(playlistID, "RD") && !strings.HasPrefix(playlistID, "RDCLAK") {
			return GetYouTubeMixPlaylist(ctx, playlistID)
		}
		return getYouTubePlaylist(ctx, playlistID)

	case videoID != "":
		if cached, ok := GetCachedTrackMeta(videoID); ok {
			go PrefetchYouTubeAudio(videoID)
			return &utils.PlatformTracks{Results: []utils.GetUrlTrack{cached}}, nil
		}

		if res, err := getYouTubeVideo(ctx, videoID); err == nil && res != nil && len(res.Results) > 0 {
			go PrefetchYouTubeAudio(videoID)
			return res, nil
		}

		for _, query := range []string{videoID, y.Query} {
			tracks, err := searchYouTube(query, 10)
			if err != nil {
				continue
			}

			for _, track := range tracks {
				if track.Id == videoID {
					go PrefetchYouTubeAudio(videoID)
					return &utils.PlatformTracks{Results: []utils.GetUrlTrack{track}}, nil
				}
			}
		}

		if title, err := getYouTubeTitleFromOEmbed(videoID); err == nil && title != "" {
			tracks, err := searchYouTube(title, 10)
			if err == nil {
				for _, track := range tracks {
					if track.Id == videoID {
						return &utils.PlatformTracks{Results: []utils.GetUrlTrack{track}}, nil
					}
				}
			}
		}

		slog.Warn("Video ID was extracted but no matching track was found in search results", "video_id", videoID)
		return getYouTubeVideo(ctx, videoID)

	case playlistID != "":
		if strings.HasPrefix(playlistID, "RD") && !strings.HasPrefix(playlistID, "RDCLAK") {
			return GetYouTubeMixPlaylist(ctx, playlistID)
		}
		return getYouTubePlaylist(ctx, playlistID)
	}

	return nil, errors.New("no video or playlist results were found")
}

func (y *youTubeData) search() (*utils.PlatformTracks, error) {
	if y.isValid() {
		return y.getInfo()
	}

	tracks, err := searchYouTube(y.Query, 10)
	if err != nil {
		return nil, err
	}

	return &utils.PlatformTracks{Results: tracks}, nil
}

func (y *youTubeData) getTrack() (*utils.TrackInfo, error) {
	if y.Query == "" {
		return nil, errors.New("the query is empty")
	}

	if !y.isValid() {
		return nil, errors.New("the provided URL is invalid or the platform is not supported")
	}

	if videoID := extractVideoID(normalizeYouTubeURL(y.Query)); len(videoID) == 11 {
		return &utils.TrackInfo{
			Id:       videoID,
			URL:      "https://www.youtube.com/watch?v=" + videoID,
			Platform: utils.YouTube,
		}, nil
	}

	if y.ApiUrl != "" && y.APIKey != "" {
		if trackInfo, err := newApiData(y.Query).getTrack(); err == nil {
			return trackInfo, nil
		}
	}

	getInfo, err := y.getInfo()
	if err != nil {
		return nil, err
	}
	if len(getInfo.Results) == 0 {
		return nil, errors.New("no video results were found")
	}

	track := getInfo.Results[0]
	trackInfo := &utils.TrackInfo{
		Id:       track.Id,
		URL:      track.Url,
		Platform: utils.YouTube,
	}

	return trackInfo, nil
}

func (y *youTubeData) downloadTrack(info *utils.TrackInfo, video bool) (string, error) {
	if cached := findCachedYouTubeFile(info.Id); cached != "" {
		return cached, nil
	}

	if !video && info.CdnURL != "" {
		return info.CdnURL, nil
	}

	mode := "audio"
	if video {
		mode = "video"
	}
	key := fmt.Sprintf("yt:%s:%s", info.Id, mode)

	return deduplicateYouTubeDownload(key, func() (string, error) {
		if cached := findCachedYouTubeFile(info.Id); cached != "" {
			return cached, nil
		}

		// 1. Ultra-fast direct Innertube parallel range download to RAM disk (/dev/shm)
		if fastPath, err := downloadFastInnertube(info.Id, video); err == nil && fastPath != "" {
			return fastPath, nil
		}

		// 2. Optional external API fallback if configured
		if !video && y.ApiUrl != "" && y.APIKey != "" {
			if filePath, err := y.downloadWithApi(info.Id, video); err == nil && filePath != "" {
				return filePath, nil
			}
		}

		// 3. 2026 Cookie-Free yt-dlp fallback
		return y.downloadWithYtDlp(info.Id, video)
	})
}

func (y *youTubeData) buildYtdlpParams(videoID string, video bool) ([]string, string) {
	outputTemplate := filepath.Join(config.DownloadsDir, "%(id)s.%(ext)s")
	cacheDir := "/dev/shm/ytdlp_cache"
	if _, err := os.Stat("/dev/shm"); err != nil {
		cacheDir = filepath.Join(os.TempDir(), "ytdlp_cache")
	}
	_ = os.MkdirAll(cacheDir, 0755)

	extractorArgs := "youtube:player_client=web_embedded,tv_downgraded,tv_samsung,visionos,android;player_skip=webpage,initial_data;skip=dash,translated_subs;fetch_pot=never"
	if vData := getVisitorData(); vData != "" {
		extractorArgs += ";visitor_data=" + vData
	}

	params := []string{
		"yt-dlp",
		"--no-warnings",
		"--quiet",
		"--no-progress",
		"--force-ipv4",
		"--no-check-certificates",
		"--no-playlist",
		"--retries", "2",
		"--fragment-retries", "2",
		"--continue",
		"--no-part",
		"--concurrent-fragments", "16",
		"--http-chunk-size", "10M",
		"--socket-timeout", "8",
		"--cache-dir", cacheDir,
		"--js-runtimes", "deno,node",
		"--remote-components", "ejs:github",
		"--no-write-thumbnail",
		"--no-write-info-json",
		"--no-embed-metadata",
		"--no-embed-chapters",
		"--no-embed-subs",
		"--extractor-args", extractorArgs,
		"-o", outputTemplate,
	}

	if video {
		formatSelector := "18/22/bestvideo[height<=720][ext=mp4]+bestaudio[ext=m4a]/best[height<=720][ext=mp4]/best"
		params = append(params, "-f", formatSelector, "--merge-output-format", "mp4")
	} else {
		params = append(params, "-f", "140/251/bestaudio[ext=m4a]/bestaudio/18/best")
	}

	if config.Proxy != "" {
		params = append(params, "--proxy", config.Proxy)
	}

	videoURL := "https://www.youtube.com/watch?v=" + videoID
	params = append(params, videoURL, "--print", "after_move:filepath")

	return params, ""
}

func (y *youTubeData) downloadWithYtDlp(videoID string, video bool) (string, error) {
	if videoID == "" {
		return "", errors.New("videoID is empty")
	}

	ytdlpParams, cookieFile := y.buildYtdlpParams(videoID, video)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ytdlpParams[0], ytdlpParams[1:]...)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr := string(exitErr.Stderr)
			if cookieFile != "" && strings.Contains(stderr, "Sign in to confirm you're not a bot") {
				_ = os.Remove(cookieFile)
			}
			return "", fmt.Errorf("yt-dlp failed with exit code %d: %s", exitErr.ExitCode(), stderr)
		}

		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("yt-dlp timed out for video ID: %s", videoID)
		}

		return "", fmt.Errorf("an unexpected error occurred while downloading %s: %w", videoID, err)
	}

	downloadedPathStr := strings.TrimSpace(string(output))
	if downloadedPathStr == "" {
		if cached := findCachedYouTubeFile(videoID); cached != "" {
			return cached, nil
		}
		return "", fmt.Errorf("no output path was returned for %s", videoID)
	}

	if _, err := os.Stat(downloadedPathStr); os.IsNotExist(err) {
		if cached := findCachedYouTubeFile(videoID); cached != "" {
			return cached, nil
		}
		return "", fmt.Errorf("the file was not found at the reported path: %s", downloadedPathStr)
	}

	return downloadedPathStr, nil
}

func (y *youTubeData) getCookieFile() string {
	cookiesPath := config.CookiesPath
	if len(cookiesPath) == 0 {
		return ""
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(cookiesPath))))
	if err != nil {
		return cookiesPath[0]
	}

	return cookiesPath[n.Int64()]
}

func (y *youTubeData) downloadWithApi(videoID string, _ bool) (string, error) {
	videoUrl := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
	api := newApiData(videoUrl)
	track, err := api.getTrack()
	if err != nil {
		return "", err
	}

	return processDownload(track)
}

func (y *youTubeData) getRecommendations(ctx context.Context, rawURL string) (*utils.PlatformTracks, error) {
	urlToUse := rawURL
	if urlToUse == "" {
		urlToUse = y.Query
	}

	videoID := extractVideoID(urlToUse)
	if videoID == "" && urlToUse != "" && !strings.HasPrefix(urlToUse, "http") {
		if len(urlToUse) == 11 {
			videoID = urlToUse
		}
	}

	if videoID == "" {
		return nil, errors.New("no valid YouTube video ID found for recommendations")
	}

	return GetYouTubeMixPlaylist(ctx, "RD"+videoID)
}
