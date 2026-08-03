package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fzrilsh/lks-judge/internal/backup"
	"github.com/fzrilsh/lks-judge/internal/realtime"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web"
)

func main() {
	dataDir := flag.String("data", "./data", "data directory for SQLite + uploads")
	listen := flag.String("listen", "0.0.0.0:8080", "HTTP listen address")
	dev := flag.Bool("dev", false, "enable dev mode (seed default data)")
	flag.Parse()

	log.Printf("starting LKS Judge server")
	log.Printf("data directory: %s", *dataDir)
	log.Printf("listen address: %s", *listen)

	// create subdirs
	for _, d := range []string{"backups", "files", "submissions", "uploads_tmp"} {
		if err := os.MkdirAll(filepath.Join(*dataDir, d), 0o755); err != nil {
			log.Fatalf("mkdir %s: %v", d, err)
		}
	}

	// open store
	st, err := store.Open(*dataDir)
	if err != nil {
		log.Fatalf("store.Open: %v", err)
	}
	defer func() { _ = st.Close() }()

	log.Printf("database ready at %s", filepath.Join(*dataDir, "lks.sqlite"))

	// dev seed
	if *dev {
		if err := st.SeedDevData(); err != nil {
			log.Fatalf("seed dev data: %v", err)
		}
	}

	// prime competition state cache
	if err := st.LoadCompetitionCache(); err != nil {
		log.Fatalf("load competition cache: %v", err)
	}

	// graceful shutdown context (needed before goroutines reference ctx.Done)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// start backup goroutine
	go backup.Start(*dataDir, st.Writer, ctx.Done())

	// countdown ticker: drives waiting -> running -> finished and the 1200s form-open threshold
	cd := &realtime.Countdown{
		Snapshot: st.CompetitionCache.Load,
		Transition: func(to string) {
			c := st.CompetitionCache.Load()
			if c == nil {
				return
			}
			if err := st.TransitionStatus(c.Status, to); err != nil {
				log.Printf("countdown transition: %v", err)
			}
		},
		// ponytail: log only until the WS hub lands (Phase 8), then broadcast FormOpened.
		FormOpened: func(open bool) { log.Printf("FormOpened{status:%v}", open) },
	}
	go cd.Run(ctx)

	// router
	mux := http.NewServeMux()

	// static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", web.StaticHandler()))

	// auth routes
	mux.HandleFunc("GET /login", web.HandleLoginGET)
	mux.HandleFunc("POST /login", web.HandleLoginPOST(st))
	mux.HandleFunc("POST /logout", web.HandleLogoutPOST(st))

	// protected participant routes
	mux.Handle("GET /", web.RequireParticipant(st)(http.HandlerFunc(web.HandleDashboard)))

	// public countdown display (projector) and the shared polling endpoint
	mux.Handle("GET /countdown", web.HandleCountdownPublicGET(st))
	mux.Handle("GET /countdown/time", web.HandleCountdownTimeGET(st))

	// jury routes
	juryMw := web.RequireJury(st)
	mux.Handle("GET /jury/", juryMw(web.HandleJuryGET(st)))
	mux.Handle("POST /jury/", juryMw(web.HandleJuryPOST(st)))
	mux.Handle("POST /jury/reset", juryMw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "501 Not Implemented", http.StatusNotImplemented)
	})))

	// jury participant routes
	mux.Handle("GET /jury/participants", juryMw(web.HandleParticipantsGET(st)))
	mux.Handle("POST /jury/participants", juryMw(web.HandleParticipantsPOST(st)))
	mux.Handle("POST /jury/participants/import", juryMw(web.HandleParticipantsImportPOST(st)))
	mux.Handle("GET /jury/participants/export", juryMw(web.HandleParticipantsExportGET(st)))
	mux.Handle("GET /jury/participants/shuffle", juryMw(web.HandleParticipantsShuffleGET(st)))
	mux.Handle("POST /jury/participants/shuffle", juryMw(web.HandleParticipantsShufflePOST(st)))
	mux.Handle("POST /jury/participants/{id}/delete", juryMw(web.HandleParticipantDeletePOST(st)))

	// jury module routes
	mux.Handle("GET /jury/modules", juryMw(web.HandleModulesGET(st)))
	mux.Handle("POST /jury/modules", juryMw(web.HandleModulesPOST(st)))
	mux.Handle("POST /jury/modules/generate", juryMw(web.HandleModulesGeneratePOST(st)))
	mux.Handle("POST /jury/modules/set-current", juryMw(web.HandleModulesSetCurrentPOST(st)))
	mux.Handle("POST /jury/modules/{id}/rename", juryMw(web.HandleModuleRenamePOST(st)))
	mux.Handle("POST /jury/modules/{id}/delete", juryMw(web.HandleModuleDeletePOST(st)))

	// jury countdown routes
	mux.Handle("GET /jury/countdown", juryMw(web.HandleCountdownJuryGET(st)))
	mux.Handle("POST /jury/countdown", juryMw(web.HandleCountdownJuryPOST(st)))
	mux.Handle("POST /jury/countdown/pause", juryMw(web.HandleCountdownPause(st)))
	mux.Handle("POST /jury/countdown/resume", juryMw(web.HandleCountdownResume(st)))
	mux.Handle("POST /jury/countdown/stop", juryMw(web.HandleCountdownStop(st)))

	// healthz
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
	})

	srv := &http.Server{
		Addr:    *listen,
		Handler: mux,
	}

	go func() {
		log.Printf("listening on %s", *listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	if err := backup.RunOnce(*dataDir, st.Writer); err != nil {
		log.Printf("shutdown backup: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown: %v", err)
	}

	log.Println("server stopped")
}
