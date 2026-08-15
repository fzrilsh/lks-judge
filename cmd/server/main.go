package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fzrilsh/lks-judge/internal/backup"
	"github.com/fzrilsh/lks-judge/internal/logfile"
	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/realtime"
	"github.com/fzrilsh/lks-judge/internal/scoring"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/upload"
	"github.com/fzrilsh/lks-judge/internal/web"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
)

func main() {
	dataDir := flag.String("data", "./data", "data directory for SQLite + uploads")
	listen := flag.String("listen", "0.0.0.0:8080", "HTTP listen address")
	dev := flag.Bool("dev", false, "enable dev mode (seed default data)")
	var juryIPs multiFlag
	flag.Var(&juryIPs, "jury-ip", "extra jury IP or CIDR granted /jury/* access, in memory only (repeatable or comma-separated)")
	flag.Parse()

	// Tee logs to a per-day file under {data}/logs so a run leaves a record,
	// not just the terminal. Set up before anything else logs.
	logDir := filepath.Join(*dataDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		log.Fatalf("mkdir logs: %v", err)
	}
	rotator := logfile.New(logDir)
	defer func() { _ = rotator.Close() }()
	log.SetOutput(io.MultiWriter(os.Stderr, rotator))

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

	// --jury-ip: in-memory allowlist, never persisted, survives Reset.
	if bad := st.SetExtraNets(juryIPs); len(bad) > 0 {
		log.Fatalf("--jury-ip: not an IP or CIDR: %v", bad)
	}
	if len(juryIPs) > 0 {
		log.Printf("extra jury allowlist (flag, not persisted): %v", []string(juryIPs))
	}

	// header title accessor: templ shell reads the live competition name
	templates.SetCompetitionName(func() string {
		if c := st.CompetitionCache.Load(); c != nil && c.Name != "" {
			return c.Name
		}
		return "LKS Judge Platform"
	})

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

	// leaderboard cache: pre-render once at startup so the first public load has data
	scoreCache := scoring.NewCache()
	if c := st.CompetitionCache.Load(); c != nil {
		if err := scoreCache.Refresh(st, c.ID); err != nil {
			log.Printf("prime leaderboard cache: %v", err)
		}
	}

	// graceful shutdown context (needed before goroutines reference ctx.Done)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// start backup goroutine
	go backup.Start(*dataDir, st.Writer, ctx.Done())

	// sweep expired upload sessions and their tmp chunk dirs
	go upload.StartCleanup(st, *dataDir, ctx.Done())

	// sweep expired participant sessions (bounds memory across a multi-day run)
	go store.StartSessionSweep(st, ctx.Done())

	// WS hub: one goroutine owns the client set for the life of the process
	hub := realtime.NewHub()
	go hub.Run(ctx)

	// countdown ticker: drives waiting -> running -> finished and the 1200s form-open threshold
	tickCount := 0
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
		FormOpened: func(open bool) {
			hub.Broadcast(realtime.EvFormOpened, map[string]bool{"status": open})
		},
		// Tick fires every second; the wire only needs it every 5 (spec §8).
		Tick: func(seconds int) {
			tickCount++
			if tickCount%5 == 0 {
				status := "waiting"
				if c := st.CompetitionCache.Load(); c != nil {
					status = c.Status
				}
				hub.Broadcast(realtime.EvCountdownTick, map[string]any{"seconds": seconds, "status": status})
			}
		},
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
	mux.Handle("GET /", web.RequireParticipant(st)(web.HandleDashboard(st)))

	// public countdown display (projector) and the shared polling endpoint
	mux.Handle("GET /countdown", web.HandleCountdownPublicGET(st))
	mux.Handle("GET /countdown/time", web.HandleCountdownTimeGET(st))

	// websocket: auth optional by design (spec §8), anonymous clients get a reduced event set
	mux.Handle("GET /ws", web.HandleWS(st, hub))

	// jury routes
	juryMw := web.RequireJury(st)
	mux.Handle("GET /jury/", juryMw(web.HandleDashboardJuryGET(st)))
	mux.Handle("GET /jury/competition", juryMw(web.HandleJuryGET(st)))
	mux.Handle("POST /jury/competition", juryMw(web.HandleJuryPOST(st)))
	mux.Handle("POST /jury/reset", juryMw(web.HandleResetPOST(st, scoreCache, hub, *dataDir,
		func() error { return backup.RunOnce(*dataDir, st.Writer) })))

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
	mux.Handle("POST /jury/modules/set-current", juryMw(web.HandleModulesSetCurrentPOST(st, hub)))
	mux.Handle("POST /jury/modules/{id}/rename", juryMw(web.HandleModuleRenamePOST(st)))
	mux.Handle("POST /jury/modules/{id}/delete", juryMw(web.HandleModuleDeletePOST(st, hub)))

	// jury automark routes
	mux.Handle("GET /jury/automark", juryMw(web.HandleAutomarkGET(st, *dataDir)))
	mux.Handle("POST /jury/automark", juryMw(web.HandleAutomarkSavePOST(st, *dataDir)))
	mux.Handle("POST /jury/automark/run", juryMw(web.HandleAutomarkRunPOST(st, hub, *dataDir)))
	mux.Handle("POST /jury/automark/apply", juryMw(web.HandleAutomarkApplyPOST(st, scoreCache, hub)))

	// jury countdown routes
	mux.Handle("GET /jury/countdown", juryMw(web.HandleCountdownJuryGET(st)))
	mux.Handle("POST /jury/countdown", juryMw(web.HandleCountdownJuryPOST(st)))
	mux.Handle("POST /jury/countdown/pause", juryMw(web.HandleCountdownPause(st)))
	mux.Handle("POST /jury/countdown/resume", juryMw(web.HandleCountdownResume(st)))
	mux.Handle("POST /jury/countdown/stop", juryMw(web.HandleCountdownStop(st)))

	// chunked upload routes: participant session or allowlisted jury IP
	upMw := web.RequireUploader(st)
	// submissionOpen re-derives the submission window from the competition cache.
	// Injected so upload need not import realtime (spec §11 package graph).
	submissionOpen := func() bool {
		c := st.CompetitionCache.Load()
		seconds, _ := realtime.TimeLeft(c, time.Now())
		return realtime.FormOpen(c, seconds)
	}
	mux.Handle("POST /upload/init", upMw(upload.HandleInitPOST(st, submissionOpen)))
	mux.Handle("PUT /upload/{id}/chunk/{n}", upMw(upload.HandleChunkPUT(st, *dataDir)))
	mux.Handle("GET /upload/{id}/status", upMw(upload.HandleStatusGET(st, *dataDir)))
	mux.Handle("POST /upload/{id}/complete", upMw(upload.HandleCompletePOST(st, *dataDir, submissionOpen, func(f *model.File) {
		hub.Broadcast(realtime.EvFileListUpdated, map[string]any{
			"id": f.ID, "name": f.Name, "path": f.Path, "is_public": f.IsPublic,
		})
	})))

	// file download: inline auth (participant session or jury IP), private files hidden from participants
	mux.Handle("GET /files/{id}/download", web.HandleFileDownloadGET(st))

	// jury file management
	mux.Handle("GET /jury/files", juryMw(web.HandleFilesGET(st)))
	mux.Handle("POST /jury/files/{id}/toggle", juryMw(web.HandleFileTogglePOST(st, hub)))
	mux.Handle("POST /jury/files/{id}/delete", juryMw(web.HandleFileDeletePOST(st, hub)))

	// jury submissions matrix: per-cell download plus bulk ZIP export
	mux.Handle("GET /jury/submissions", juryMw(web.HandleSubmissionsGET(st)))
	mux.Handle("GET /jury/submissions/export.zip", juryMw(web.HandleSubmissionsExportZipGET(st)))
	mux.Handle("GET /jury/submissions/module/{moduleID}/export.zip", juryMw(web.HandleModuleSubmissionsExportZipGET(st)))
	mux.Handle("GET /jury/submissions/{id}/download", juryMw(web.HandleSubmissionDownloadGET(st)))

	// jury scoring: raw-score matrix + PDF export
	mux.Handle("GET /jury/scoring", juryMw(web.Gzip(web.HandleScoringGET(st))))
	mux.Handle("POST /jury/scoring", juryMw(web.HandleScoringPOST(st, scoreCache, hub)))
	mux.Handle("GET /jury/scoring/export-pdf", juryMw(web.HandleScoringExportPDF(st)))

	// public leaderboard (HTML shell + JSON snapshot), unauthenticated by design, gzip-scoped
	mux.Handle("GET /leaderboard", web.Gzip(web.HandleLeaderboardGET(st, scoreCache)))
	mux.Handle("GET /leaderboard.json", web.Gzip(web.HandleLeaderboardGET(st, scoreCache)))

	// healthz
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
	})

	srv := &http.Server{
		Addr:              *listen,
		Handler:           web.CSRFProtect(mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", *listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	// Drain HTTP first: no in-flight request should still be writing when the
	// backup snapshots the DB.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown: %v", err)
	}

	if err := backup.RunOnce(*dataDir, st.Writer); err != nil {
		log.Printf("shutdown backup: %v", err)
	}

	log.Println("server stopped")
}

// multiFlag collects a repeatable string flag. A single occurrence may also be
// comma-separated, so --jury-ip="a,b" and --jury-ip a --jury-ip b both work.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(v string) error {
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			*m = append(*m, p)
		}
	}
	return nil
}
