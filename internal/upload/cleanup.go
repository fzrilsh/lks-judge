package upload

import (
	"log"
	"os"
	"time"

	"github.com/fzrilsh/lks-judge/internal/store"
)

// StartCleanup sweeps expired upload sessions every 10 minutes until done is
// closed. Each swept ID's tmp chunk directory is removed so abandoned uploads
// don't accumulate on disk. Call as a goroutine from main.
func StartCleanup(st *store.Store, dataDir string, done <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ids, err := st.DeleteExpiredUploadSessions(time.Now().UTC())
			if err != nil {
				log.Printf("upload cleanup: %v", err)
				continue
			}
			for _, id := range ids {
				if err := os.RemoveAll(TmpDir(dataDir, id)); err != nil {
					log.Printf("upload cleanup: remove tmp %s: %v", id, err)
				}
			}
			if len(ids) > 0 {
				log.Printf("upload cleanup: swept %d expired session(s)", len(ids))
			}
		case <-done:
			return
		}
	}
}
