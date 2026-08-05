package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
)

// ErrModuleNotFound is returned when a module ID doesn't exist for the given competition.
var ErrModuleNotFound = errors.New("module not found")

// GetCompetition fetches the single competition row. Returns nil, nil if no row exists.
func (s *Store) GetCompetition() (*model.Competition, error) {
	// start_date/end_date are CAST so the driver hands back the stored "2006-01-02" text instead
	// of converting the DATE-declared column into a time.Time (which scans as RFC3339).
	row := s.Reader.QueryRow(`
		SELECT id, name, level, allowed_ips, current_module_id,
		       CAST(start_date AS TEXT), CAST(end_date AS TEXT),
		       status, remaining_seconds, paused_at,
		       start_time, end_time, created_at, updated_at
		FROM competitions LIMIT 1`)

	var c model.Competition
	var currentModuleID sql.NullInt64
	var remainingSeconds sql.NullInt64
	var pausedAt sql.NullTime
	var startTime, endTime sql.NullString

	err := row.Scan(
		&c.ID, &c.Name, &c.Level, &c.AllowedIPs, &currentModuleID,
		&c.StartDate, &c.EndDate, &c.Status, &remainingSeconds, &pausedAt,
		&startTime, &endTime, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get competition: %w", err)
	}

	if currentModuleID.Valid {
		c.CurrentModuleID = &currentModuleID.Int64
	}
	if remainingSeconds.Valid {
		v := int(remainingSeconds.Int64)
		c.RemainingSeconds = &v
	}
	if pausedAt.Valid {
		c.PausedAt = &pausedAt.Time
	}
	if startTime.Valid {
		c.StartTime = &startTime.String
	}
	if endTime.Valid {
		c.EndTime = &endTime.String
	}

	return &c, nil
}

// UpsertCompetition inserts (if no row) or updates (if row exists), then refreshes the cache.
// It writes settings only. status/remaining_seconds/paused_at belong to the countdown and are
// never touched here, so saving the setup form mid-run cannot clobber a live timer.
func (s *Store) UpsertCompetition(c *model.Competition) error {
	existing, err := s.GetCompetition()
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	if existing == nil {
		_, err = s.Writer.Exec(`
			INSERT INTO competitions(name, level, allowed_ips, start_date, end_date,
			    status, start_time, end_time, updated_at)
			VALUES (?, ?, ?, ?, ?, 'waiting', ?, ?, ?)`,
			c.Name, c.Level, c.AllowedIPs, c.StartDate, c.EndDate,
			c.StartTime, c.EndTime, now,
		)
	} else {
		c.ID = existing.ID
		_, err = s.Writer.Exec(`
			UPDATE competitions SET
			    name=?, level=?, allowed_ips=?, start_date=?, end_date=?,
			    start_time=?, end_time=?, updated_at=?
			WHERE id=?`,
			c.Name, c.Level, c.AllowedIPs, c.StartDate, c.EndDate,
			c.StartTime, c.EndTime, now,
			c.ID,
		)
	}
	if err != nil {
		return fmt.Errorf("upsert competition: %w", err)
	}

	return s.LoadCompetitionCache()
}

// LoadCompetitionCache primes CompetitionCache from DB. Call once at startup after Open().
func (s *Store) LoadCompetitionCache() error {
	c, err := s.GetCompetition()
	if err != nil {
		return err
	}
	s.CompetitionCache.Store(c) // nil if no row — intentional
	s.reloadAllowedNets(c)
	return nil
}

// reloadAllowedNets parses the competition allowlist into net.IPNet once, so
// request-time jury checks skip JSON + IP parsing. A malformed list stores an
// empty set: callers treat empty as "loopback only".
func (s *Store) reloadAllowedNets(c *model.Competition) {
	nets := []net.IPNet{}
	if c != nil && c.AllowedIPs != "" && c.AllowedIPs != "[]" {
		var ips []string
		if err := json.Unmarshal([]byte(c.AllowedIPs), &ips); err != nil {
			log.Printf("allowlist: malformed allowed_ips, treating as empty: %v", err)
		} else {
			for _, e := range ips {
				if _, cidr, err := net.ParseCIDR(e); err == nil {
					nets = append(nets, *cidr)
				} else if ip := net.ParseIP(e); ip != nil {
					nets = append(nets, net.IPNet{IP: ip, Mask: fullMask(ip)})
				}
			}
		}
	}
	s.allowedNets.Store(&nets)
}

// fullMask returns a /32 (v4) or /128 (v6) mask so a single-IP entry becomes an
// IPNet that Contains only itself.
func fullMask(ip net.IP) net.IPMask {
	if ip.To4() != nil {
		return net.CIDRMask(32, 32)
	}
	return net.CIDRMask(128, 128)
}

// AllowedNets returns the parsed jury allowlist. Empty means loopback only.
func (s *Store) AllowedNets() []net.IPNet {
	if p := s.allowedNets.Load(); p != nil {
		return *p
	}
	return nil
}

// ListModules returns all modules for a competition ordered by "order" ASC.
func (s *Store) ListModules(competitionID int64) ([]*model.Module, error) {
	rows, err := s.Reader.Query(
		`SELECT id, competition_id, name, "order", created_at, updated_at
		 FROM modules WHERE competition_id = ? ORDER BY "order" ASC`, competitionID)
	if err != nil {
		return nil, fmt.Errorf("list modules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*model.Module
	for rows.Next() {
		var m model.Module
		if err := rows.Scan(&m.ID, &m.CompetitionID, &m.Name, &m.Order, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan module: %w", err)
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// GetModuleByID returns one module. Returns ErrModuleNotFound when it does not exist.
func (s *Store) GetModuleByID(id int64) (*model.Module, error) {
	var m model.Module
	err := s.Reader.QueryRow(
		`SELECT id, competition_id, name, "order", created_at, updated_at
		 FROM modules WHERE id = ?`, id,
	).Scan(&m.ID, &m.CompetitionID, &m.Name, &m.Order, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrModuleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get module: %w", err)
	}
	return &m, nil
}

// AutoSetCurrentIfFirst points competitions.current_module_id at moduleID only when it is unset.
func (s *Store) AutoSetCurrentIfFirst(competitionID, moduleID int64) error {
	_, err := s.Writer.Exec(
		`UPDATE competitions SET current_module_id = ?, updated_at = ?
		 WHERE id = ? AND current_module_id IS NULL`,
		moduleID, time.Now().UTC(), competitionID,
	)
	if err != nil {
		return fmt.Errorf("auto set current module: %w", err)
	}
	return s.LoadCompetitionCache()
}

// SetCurrentModule updates competitions.current_module_id and refreshes the cache.
// Returns ErrModuleNotFound if moduleID doesn't belong to the competition.
func (s *Store) SetCurrentModule(competitionID, moduleID int64) error {
	var exists int
	err := s.Reader.QueryRow(
		`SELECT 1 FROM modules WHERE id = ? AND competition_id = ?`, moduleID, competitionID,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return ErrModuleNotFound
	}
	if err != nil {
		return fmt.Errorf("lookup module: %w", err)
	}

	if _, err := s.Writer.Exec(
		`UPDATE competitions SET current_module_id = ?, updated_at = ? WHERE id = ?`,
		moduleID, time.Now().UTC(), competitionID,
	); err != nil {
		return fmt.Errorf("set current module: %w", err)
	}
	return s.LoadCompetitionCache()
}

// RenameModule changes a module's name.
func (s *Store) RenameModule(id int64, name string) error {
	_, err := s.Writer.Exec(
		`UPDATE modules SET name = ?, updated_at = ? WHERE id = ?`,
		name, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("rename module: %w", err)
	}
	return nil
}

// DeleteModule removes a module and clears current_module_id if it pointed here.
func (s *Store) DeleteModule(id int64) error {
	if _, err := s.Writer.Exec(`DELETE FROM modules WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete module: %w", err)
	}
	// ON DELETE SET NULL only fires for the FK; do it explicitly so the cache stays truthful.
	if _, err := s.Writer.Exec(
		`UPDATE competitions SET current_module_id = NULL, updated_at = ? WHERE current_module_id = ?`,
		time.Now().UTC(), id,
	); err != nil {
		return fmt.Errorf("clear current module: %w", err)
	}
	return s.LoadCompetitionCache()
}

var moduleSuffixes = [7]string{"A", "B", "C", "D", "E", "F", "G"}

// GenerateModules bulk-creates total modules named MA, MB, ... appended after existing ones.
// Suffixes already taken by existing modules are skipped, so repeated calls don't create duplicates.
func (s *Store) GenerateModules(competitionID int64, total int) ([]*model.Module, error) {
	if total < 1 || total > len(moduleSuffixes) {
		return nil, fmt.Errorf("total must be 1-%d, got %d", len(moduleSuffixes), total)
	}

	existing, err := s.ListModules(competitionID)
	if err != nil {
		return nil, err
	}
	taken := make(map[string]bool, len(existing))
	for _, m := range existing {
		taken[m.Name] = true
	}

	names := make([]string, 0, total)
	for _, suf := range moduleSuffixes {
		if len(names) == total {
			break
		}
		if name := "M" + suf; !taken[name] {
			names = append(names, name)
		}
	}
	if len(names) < total {
		return nil, fmt.Errorf("only %d of %d module names available (MA-M%s all used)",
			len(names), total, moduleSuffixes[len(moduleSuffixes)-1])
	}

	now := time.Now().UTC()
	out := make([]*model.Module, 0, total)
	for _, name := range names {
		res, err := s.Writer.Exec(
			`INSERT INTO modules(competition_id, name, "order", created_at, updated_at)
			 SELECT ?, ?, COALESCE(MAX("order"),0)+1, ?, ? FROM modules WHERE competition_id = ?`,
			competitionID, name, now, now, competitionID,
		)
		if err != nil {
			return nil, fmt.Errorf("insert module %s: %w", name, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("module id %s: %w", name, err)
		}
		var order int
		if err := s.Reader.QueryRow(`SELECT "order" FROM modules WHERE id = ?`, id).Scan(&order); err != nil {
			return nil, fmt.Errorf("module order %s: %w", name, err)
		}
		out = append(out, &model.Module{
			ID: id, CompetitionID: competitionID, Name: name, Order: order,
			CreatedAt: now, UpdatedAt: now,
		})
	}

	if err := s.AutoSetCurrentIfFirst(competitionID, out[0].ID); err != nil {
		return nil, err
	}
	return out, nil
}

// UpsertModuleByName inserts a module if no module with that name exists for the competition.
// Returns the module ID.
func (s *Store) UpsertModuleByName(competitionID int64, name string) (int64, error) {
	var id int64
	err := s.Reader.QueryRow(
		`SELECT id FROM modules WHERE competition_id = ? AND name = ?`, competitionID, name,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("lookup module: %w", err)
	}

	// Order computed inside the INSERT so concurrent creates can't read the same MAX.
	now := time.Now().UTC()
	res, err := s.Writer.Exec(
		`INSERT INTO modules(competition_id, name, "order", created_at, updated_at)
		 SELECT ?, ?, COALESCE(MAX("order"),0)+1, ?, ? FROM modules WHERE competition_id = ?`,
		competitionID, name, now, now, competitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("insert module: %w", err)
	}
	return res.LastInsertId()
}
