package web

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
)

// HandleSubmissionsGET renders the jury matrix of participants x modules.
func HandleSubmissionsGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/?setup=1", http.StatusSeeOther)
			return
		}
		participants, err := st.ListParticipants(comp.ID)
		if err != nil {
			log.Printf("list participants: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		modules, err := st.ListModules(comp.ID)
		if err != nil {
			log.Printf("list modules: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		subs, err := st.ListSubmissions(comp.ID)
		if err != nil {
			log.Printf("list submissions: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// cell[participantID][moduleID] = submission
		cell := make(map[int64]map[int64]*model.Submission, len(participants))
		for _, s := range subs {
			m := cell[s.ParticipantID]
			if m == nil {
				m = make(map[int64]*model.Submission)
				cell[s.ParticipantID] = m
			}
			m[s.ModuleID] = s
		}
		if err := templates.Submissions(comp, participants, modules, cell).Render(r.Context(), w); err != nil {
			log.Printf("render submissions: %v", err)
		}
	}
}

// HandleSubmissionDownloadGET serves one submission file (jury only) with Range support.
func HandleSubmissionDownloadGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, jury := juryAllowed(st, r); !jury {
			http.NotFound(w, r)
			return
		}
		sub, err := st.GetSubmissionByID(r.PathValue("id"))
		if errors.Is(err, store.ErrSubmissionNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("get submission: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		file, err := os.Open(sub.FilePath)
		if err != nil {
			log.Printf("open submission %s: %v", sub.FilePath, err)
			http.NotFound(w, r)
			return
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil {
			log.Printf("stat submission %s: %v", sub.FilePath, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+cdFilename(sub.Name)+`"`)
		log.Printf("submission download: id=%s name=%q ip=%s", sub.ID, sub.Name, clientIP(r))
		http.ServeContent(w, r, sub.Name, info.ModTime(), file)
	}
}

// HandleSubmissionsExportZipGET streams every submission as a ZIP (jury only),
// laid out {pc}-{participant}/{module}/{file}. Missing files on disk are skipped.
func HandleSubmissionsExportZipGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, jury := juryAllowed(st, r); !jury {
			http.NotFound(w, r)
			return
		}
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/?setup=1", http.StatusSeeOther)
			return
		}
		subs, err := st.ListSubmissions(comp.ID)
		if err != nil {
			log.Printf("list submissions: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Name lookups: participants and modules by ID.
		participants, err := st.ListParticipants(comp.ID)
		if err != nil {
			log.Printf("list participants: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		modules, err := st.ListModules(comp.ID)
		if err != nil {
			log.Printf("list modules: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		pName := make(map[int64]string, len(participants))
		for _, p := range participants {
			pName[p.ID] = participantFolder(p)
		}
		mName := make(map[int64]string, len(modules))
		for _, m := range modules {
			mName[m.ID] = m.Name
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="submissions.zip"`)
		zw := zip.NewWriter(w)
		defer func() { _ = zw.Close() }()

		for _, s := range subs {
			f, err := os.Open(s.FilePath)
			if err != nil {
				continue // skip files gone from disk; best effort
			}
			entry := filepath.ToSlash(filepath.Join(pName[s.ParticipantID], mName[s.ModuleID], s.Name))
			dst, err := zw.Create(entry)
			if err != nil {
				_ = f.Close()
				log.Printf("zip create %s: %v", entry, err)
				return
			}
			if _, err := io.Copy(dst, f); err != nil {
				_ = f.Close()
				log.Printf("zip copy %s: %v", entry, err)
				return
			}
			_ = f.Close()
		}
	}
}

// participantFolder builds the ZIP top-level folder name for a participant.
func participantFolder(p *model.Participant) string {
	if p.PCNumber != nil {
		return fmt.Sprintf("%02d-%s", *p.PCNumber, p.Name)
	}
	return p.Name
}
