package web

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/fzrilsh/lks-judge/internal/realtime"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
)

// HandleFilesGET renders the jury file manager.
func HandleFilesGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/?setup=1", http.StatusSeeOther)
			return
		}
		files, err := st.ListFiles(comp.ID)
		if err != nil {
			log.Printf("list files: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		saved := r.URL.Query().Get("saved") == "1"
		errMsg := r.URL.Query().Get("error")
		if err := templates.FilesPage(comp, files, saved, errMsg).Render(r.Context(), w); err != nil {
			log.Printf("render files: %v", err)
		}
	}
}

// HandleFileTogglePOST flips is_public and broadcasts the updated row.
func HandleFileTogglePOST(st *store.Store, hub *realtime.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := st.ToggleFilePublic(r.PathValue("id"))
		if errors.Is(err, store.ErrFileNotFound) {
			http.Redirect(w, r, "/jury/files?error=file+not+found", http.StatusSeeOther)
			return
		}
		if err != nil {
			log.Printf("toggle file: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		hub.Broadcast(realtime.EvFileListUpdated, map[string]any{
			"id": f.ID, "name": f.Name, "path": f.Path, "is_public": f.IsPublic,
		})
		http.Redirect(w, r, "/jury/files?saved=1", http.StatusSeeOther)
	}
}

// HandleFileDeletePOST removes the DB row then the on-disk file. Disk errors are
// logged only: the row is already gone, so the file is unreachable regardless.
func HandleFileDeletePOST(st *store.Store, _ string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path, err := st.DeleteFile(r.PathValue("id"))
		if errors.Is(err, store.ErrFileNotFound) {
			http.Redirect(w, r, "/jury/files?error=file+not+found", http.StatusSeeOther)
			return
		}
		if err != nil {
			log.Printf("delete file: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("delete file %s from disk: %v", path, err)
		}
		http.Redirect(w, r, "/jury/files?saved=1", http.StatusSeeOther)
	}
}

// HandleFileDownloadGET serves a file with Range support via http.ServeContent.
// Auth is inline: a valid participant session or an allowlisted jury IP. A
// participant requesting a non-public file gets 404, not 403, so the file's
// existence is never confirmed to someone not entitled to it.
func HandleFileDownloadGET(st *store.Store, _ string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := st.GetFileByID(r.PathValue("id"))
		if errors.Is(err, store.ErrFileNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("get file: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		_, jury := juryAllowed(st, r)
		if !jury {
			// Not jury: require a valid participant session, and hide private files.
			participant := false
			if cookie, cerr := r.Cookie("participant_session"); cerr == nil {
				if _, verr := st.ValidateSession(cookie.Value); verr == nil {
					participant = true
				}
			}
			if !participant || !f.IsPublic {
				http.NotFound(w, r)
				return
			}
		}

		file, err := os.Open(f.Path)
		if err != nil {
			log.Printf("open file %s: %v", f.Path, err)
			http.NotFound(w, r)
			return
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil {
			log.Printf("stat file %s: %v", f.Path, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(f.Name)+`"`)
		http.ServeContent(w, r, f.Name, info.ModTime(), file)
	}
}
