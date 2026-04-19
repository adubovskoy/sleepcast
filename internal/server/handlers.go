package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sleepcast/internal/cleanup"
	"sleepcast/internal/storage"
	ytint "sleepcast/internal/youtube"
)

type Server struct {
	DB      *storage.DB
	Media   *storage.Media
	Jobs    *ytint.JobTracker
	Cleaner *cleanup.Cleaner
	WebDir  string
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.serveIndex)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.WebDir))))

	mux.HandleFunc("POST /api/play", s.handlePlay)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /media/", s.handleMedia)
	mux.HandleFunc("POST /api/finished", s.handleFinished)

	return mux
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(s.WebDir, "index.html"))
}

type playReq struct {
	URL string `json:"url"`
}

type playResp struct {
	VideoID      string `json:"videoId"`
	State        string `json:"state"` // pending | ready | error
	Title        string `json:"title,omitempty"`
	Error        string `json:"error,omitempty"`
	StartSeconds int    `json:"startSeconds,omitempty"`
}

func (s *Server) handlePlay(w http.ResponseWriter, r *http.Request) {
	var body playReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	ref, err := ytint.ParseURL(body.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	videoID := ref.ID

	if d, err := s.DB.GetDownload(videoID); err == nil {
		writeJSON(w, playResp{VideoID: videoID, State: string(ytint.StateReady), Title: d.Title, StartSeconds: ref.StartSeconds})
		return
	}

	if st, ok := s.Jobs.Get(videoID); ok && st.State == ytint.StatePending {
		writeJSON(w, playResp{VideoID: videoID, State: string(ytint.StatePending), StartSeconds: ref.StartSeconds})
		return
	}

	if !s.Jobs.Start(videoID) {
		st, _ := s.Jobs.Get(videoID)
		writeJSON(w, playResp{VideoID: videoID, State: string(st.State), Error: st.Error, StartSeconds: ref.StartSeconds})
		return
	}

	dest := s.Media.FilePath(videoID)
	jobCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	s.Jobs.Run(jobCtx, videoID, func(ctx context.Context) error {
		defer cancel()
		title, err := ytint.DownloadAudio(ctx, videoID, dest)
		if err != nil {
			return err
		}
		return s.DB.UpsertDownload(storage.Download{
			VideoID:      videoID,
			Title:        title,
			Filepath:     dest,
			DownloadedAt: time.Now().Unix(),
		})
	})

	writeJSON(w, playResp{VideoID: videoID, State: string(ytint.StatePending), StartSeconds: ref.StartSeconds})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	vid := r.URL.Query().Get("videoId")
	if !validVideoID(vid) {
		writeError(w, http.StatusBadRequest, "invalid videoId")
		return
	}
	if d, err := s.DB.GetDownload(vid); err == nil {
		writeJSON(w, playResp{VideoID: vid, State: string(ytint.StateReady), Title: d.Title})
		return
	}
	if st, ok := s.Jobs.Get(vid); ok {
		writeJSON(w, playResp{VideoID: vid, State: string(st.State), Error: st.Error})
		return
	}
	writeJSON(w, playResp{VideoID: vid, State: string(ytint.StateError), Error: "not started"})
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	vid := strings.TrimPrefix(r.URL.Path, "/media/")
	vid = strings.TrimSuffix(vid, ".m4a")
	if !validVideoID(vid) {
		http.NotFound(w, r)
		return
	}
	d, err := s.DB.GetDownload(vid)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(d.Filepath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "stat failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "audio/mp4")
	http.ServeContent(w, r, vid+".m4a", stat.ModTime(), f)
}

type finishedReq struct {
	VideoID string `json:"videoId"`
}

func (s *Server) handleFinished(w http.ResponseWriter, r *http.Request) {
	var body finishedReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !validVideoID(body.VideoID) {
		writeError(w, http.StatusBadRequest, "invalid videoId")
		return
	}
	if err := s.Cleaner.PurgeOne(body.VideoID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("finished: purge: %v", err)
		writeError(w, http.StatusInternalServerError, "purge")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func validVideoID(s string) bool {
	if len(s) != 11 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
