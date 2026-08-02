package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/fzrilsh/lks-judge/internal/excel"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
	"golang.org/x/crypto/bcrypt"
)

func HandleParticipantsGET(st *store.Store) http.HandlerFunc {
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
		saved := r.URL.Query().Get("saved") == "1"
		errMsg := r.URL.Query().Get("error")
		if err := templates.ParticipantsPage(participants, saved, errMsg).Render(r.Context(), w); err != nil {
			log.Printf("render participants: %v", err)
		}
	}
}

func HandleParticipantsPOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/", http.StatusSeeOther)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		name := r.FormValue("name")
		school := r.FormValue("school")
		if name == "" || school == "" {
			http.Redirect(w, r, "/jury/participants?error=name+and+school+required", http.StatusSeeOther)
			return
		}

		var pcNumber *int
		if v := r.FormValue("pc_number"); v != "" {
			n, err := strconv.Atoi(v)
			if err == nil && n > 0 {
				pcNumber = &n
			}
		}

		plain, err := excel.RandomPassword()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(plain), 8)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if _, err := st.CreateParticipant(comp.ID, name, school, pcNumber, string(hash)); err != nil {
			log.Printf("create participant: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/jury/participants?saved=1", http.StatusSeeOther)
	}
}

func HandleParticipantDeletePOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if err := st.DeleteParticipant(id); err != nil {
			log.Printf("delete participant: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/jury/participants?saved=1", http.StatusSeeOther)
	}
}

func HandleParticipantsImportPOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Redirect(w, r, "/jury/participants?error=file+required", http.StatusSeeOther)
			return
		}
		defer func() { _ = file.Close() }()

		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}

		imported, err := excel.ImportParticipants(st, data)
		if err != nil {
			log.Printf("import participants: %v", err)
			http.Redirect(w, r, "/jury/participants?error=import+failed:+"+err.Error(), http.StatusSeeOther)
			return
		}
		log.Printf("imported %d participants", len(imported))
		http.Redirect(w, r, "/jury/participants?saved=1", http.StatusSeeOther)
	}
}

func HandleParticipantsExportGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := excel.ExportParticipants(st)
		if err != nil {
			log.Printf("export participants: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="participants.xlsx"`)
		_, _ = w.Write(data)
	}
}

func HandleParticipantsShuffleGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := templates.ShufflePage(nil).Render(r.Context(), w); err != nil {
			log.Printf("render shuffle: %v", err)
		}
	}
}

func HandleParticipantsShufflePOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/", http.StatusSeeOther)
			return
		}

		all, err := st.ListParticipants(comp.ID)
		if err != nil {
			log.Printf("list participants: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		results := store.ShuffleSeats(all)

		if err := st.UpdateParticipantSeats(results); err != nil {
			log.Printf("update seats: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// JSON response for animation + HTML fallback
		if r.Header.Get("Accept") == "application/json" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(results)
			return
		}

		if err := templates.ShufflePage(results).Render(r.Context(), w); err != nil {
			log.Printf("render shuffle result: %v", err)
		}
	}
}
