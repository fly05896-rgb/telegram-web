package webapp

import (
	"ashokshau/tg-web/internal/downloader"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

func lyricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isAuthorizedOrGuestRequest(r) {
		http.Error(w, "valid Telegram authentication required", http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()
	trackURL := strings.TrimSpace(q.Get("url"))
	title := strings.TrimSpace(q.Get("title"))
	artist := strings.TrimSpace(q.Get("artist"))
	trackID := strings.TrimSpace(q.Get("id"))
	durInt, _ := strconv.Atoi(strings.TrimSpace(q.Get("duration")))

	if trackURL == "" && title == "" && trackID == "" {
		http.Error(w, "track URL or title required", http.StatusBadRequest)
		return
	}
	lyrics, err := downloader.FetchLyricsWithMeta(r.Context(), downloader.LyricsQuery{
		URL:      trackURL,
		Title:    title,
		Artist:   artist,
		TrackID:  trackID,
		Duration: int32(durInt),
	})
	if errors.Is(err, downloader.ErrLyricsUnsupported) {
		http.Error(w, "lyrics are not available for this track URL", http.StatusUnprocessableEntity)
		return
	}
	if err != nil {
		http.Error(w, "lyrics could not load; try again", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(lyrics)
}
