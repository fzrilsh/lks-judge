package web

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
)

func HandleModulesGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/?setup=1", http.StatusSeeOther)
			return
		}
		modules, err := st.ListModules(comp.ID)
		if err != nil {
			log.Printf("list modules: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		saved := r.URL.Query().Get("saved") == "1"
		errMsg := r.URL.Query().Get("error")
		if err := templates.ModulesPage(comp, modules, saved, errMsg).Render(r.Context(), w); err != nil {
			log.Printf("render modules: %v", err)
		}
	}
}

func HandleModulesPOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/", http.StatusSeeOther)
			return
		}
		name := r.FormValue("name")
		if name == "" {
			http.Redirect(w, r, "/jury/modules?error=name+required", http.StatusSeeOther)
			return
		}
		id, err := st.UpsertModuleByName(comp.ID, name)
		if err != nil {
			log.Printf("create module: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := st.AutoSetCurrentIfFirst(comp.ID, id); err != nil {
			log.Printf("auto set current module: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/jury/modules?saved=1", http.StatusSeeOther)
	}
}

func HandleModulesGeneratePOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/", http.StatusSeeOther)
			return
		}
		total, err := strconv.Atoi(r.FormValue("total"))
		if err != nil {
			http.Redirect(w, r, "/jury/modules?error=total+must+be+a+number", http.StatusSeeOther)
			return
		}
		if _, err := st.GenerateModules(comp.ID, total); err != nil {
			log.Printf("generate modules: %v", err)
			http.Redirect(w, r, "/jury/modules?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/jury/modules?saved=1", http.StatusSeeOther)
	}
}

func HandleModulesSetCurrentPOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/", http.StatusSeeOther)
			return
		}
		moduleID, err := strconv.ParseInt(r.FormValue("module_id"), 10, 64)
		if err != nil {
			http.Error(w, "bad module id", http.StatusBadRequest)
			return
		}
		if err := st.SetCurrentModule(comp.ID, moduleID); err != nil {
			if errors.Is(err, store.ErrModuleNotFound) {
				http.Redirect(w, r, "/jury/modules?error=module+not+found", http.StatusSeeOther)
				return
			}
			log.Printf("set current module: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/jury/modules?saved=1", http.StatusSeeOther)
	}
}

func HandleModuleRenamePOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		name := r.FormValue("name")
		if name == "" {
			http.Redirect(w, r, "/jury/modules?error=name+required", http.StatusSeeOther)
			return
		}
		if err := st.RenameModule(id, name); err != nil {
			log.Printf("rename module: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/jury/modules?saved=1", http.StatusSeeOther)
	}
}

func HandleModuleDeletePOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if err := st.DeleteModule(id); err != nil {
			log.Printf("delete module: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/jury/modules?saved=1", http.StatusSeeOther)
	}
}
