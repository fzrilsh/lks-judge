package store

import (
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
)

// ShuffleResult is one assigned seat from a shuffle run.
type ShuffleResult struct {
	ID     int64
	Seat   int
	Name   string
	School string
}

func scanParticipant(row interface {
	Scan(...any) error
}) (*model.Participant, error) {
	var p model.Participant
	var pcNumber sql.NullInt64
	var ipAddress sql.NullString
	var plainPwd sql.NullString
	err := row.Scan(
		&p.ID, &p.CompetitionID, &p.Name, &p.School,
		&pcNumber, &p.Password, &plainPwd, &ipAddress,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if pcNumber.Valid {
		v := int(pcNumber.Int64)
		p.PCNumber = &v
	}
	if plainPwd.Valid {
		p.PlainPassword = &plainPwd.String
	}
	if ipAddress.Valid {
		p.IPAddress = &ipAddress.String
	}
	return &p, nil
}

const participantCols = `id, competition_id, name, school, pc_number, password, plain_password, ip_address, created_at, updated_at`

// GetParticipantByPCNumber queries a participant by pc_number within a competition.
func (s *Store) GetParticipantByPCNumber(competitionID int64, pcNumber int) (*model.Participant, error) {
	row := s.Reader.QueryRow(
		`SELECT `+participantCols+`
		 FROM participants WHERE competition_id = ? AND pc_number = ?`, competitionID, pcNumber)
	p, err := scanParticipant(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("participant not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query participant: %w", err)
	}
	return p, nil
}

// GetParticipantByID queries a participant by id.
func (s *Store) GetParticipantByID(id int64) (*model.Participant, error) {
	row := s.Reader.QueryRow(
		`SELECT `+participantCols+`
		 FROM participants WHERE id = ?`, id)
	p, err := scanParticipant(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("participant not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get participant: %w", err)
	}
	return p, nil
}

// ListParticipants returns all participants for a competition, seated first then unseated by name.
func (s *Store) ListParticipants(competitionID int64) ([]*model.Participant, error) {
	rows, err := s.Reader.Query(
		`SELECT `+participantCols+`
		 FROM participants WHERE competition_id = ?
		 ORDER BY pc_number ASC NULLS LAST, name ASC`, competitionID)
	if err != nil {
		return nil, fmt.Errorf("list participants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*model.Participant
	for rows.Next() {
		p, err := scanParticipant(rows)
		if err != nil {
			return nil, fmt.Errorf("scan participant: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CreateParticipant inserts a new participant and returns the new ID.
func (s *Store) CreateParticipant(competitionID int64, name, school string, pcNumber *int, passwordHash, plainPwd string) (int64, error) {
	now := time.Now().UTC()
	res, err := s.Writer.Exec(
		`INSERT INTO participants(competition_id, name, school, pc_number, password, plain_password, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		competitionID, name, school, pcNumber, passwordHash, plainPwd, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create participant: %w", err)
	}
	return res.LastInsertId()
}

// UpsertParticipant inserts or updates a participant. When pcNumber is set the
// match key is (competition_id, pc_number): re-importing the same seat updates
// the existing row (including its name), so renaming a participant while keeping
// their PC number replaces the old row rather than colliding with the unique PC
// index. Rows without a PC number fall back to matching by (competition_id,
// name), since the unique PC index only applies when pc_number IS NOT NULL.
// Returns the participant ID and the plain password (non-empty only on INSERT).
func (s *Store) UpsertParticipant(competitionID int64, name, school string, pcNumber *int, ipAddress *string, passwordHash, plainPwd string) (int64, string, error) {
	now := time.Now().UTC()

	var id int64
	var err error
	if pcNumber != nil {
		err = s.Reader.QueryRow(
			`SELECT id FROM participants WHERE competition_id = ? AND pc_number = ?`,
			competitionID, *pcNumber,
		).Scan(&id)
	} else {
		err = s.Reader.QueryRow(
			`SELECT id FROM participants WHERE competition_id = ? AND name = ?`,
			competitionID, name,
		).Scan(&id)
	}

	if err == sql.ErrNoRows {
		res, err := s.Writer.Exec(
			`INSERT INTO participants(competition_id, name, school, pc_number, ip_address, password, plain_password, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			competitionID, name, school, pcNumber, ipAddress, passwordHash, plainPwd, now, now,
		)
		if err != nil {
			return 0, "", fmt.Errorf("insert participant: %w", err)
		}
		id, _ = res.LastInsertId()
		return id, plainPwd, nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("lookup participant: %w", err)
	}

	_, err = s.Writer.Exec(
		`UPDATE participants SET name=?, school=?, pc_number=?, ip_address=?, updated_at=? WHERE id=?`,
		name, school, pcNumber, ipAddress, now, id,
	)
	if err != nil {
		return 0, "", fmt.Errorf("update participant: %w", err)
	}
	invalidateParticipant(id)
	return id, "", nil // password unchanged on update
}

// UpdateParticipantIP records the client IP on login (spec §5).
func (s *Store) UpdateParticipantIP(id int64, ip string) error {
	_, err := s.Writer.Exec(
		`UPDATE participants SET ip_address=?, updated_at=? WHERE id=?`,
		ip, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update participant ip: %w", err)
	}
	invalidateParticipant(id)
	return nil
}

// DeleteParticipant removes a participant by ID.
func (s *Store) DeleteParticipant(id int64) error {
	_, err := s.Writer.Exec(`DELETE FROM participants WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete participant: %w", err)
	}
	invalidateParticipant(id)
	return nil
}

// UpdateParticipantSeats bulk-assigns pc_number values from shuffle results.
func (s *Store) UpdateParticipantSeats(assignments []ShuffleResult) error {
	tx, err := s.Writer.Begin()
	if err != nil {
		return fmt.Errorf("shuffle seats: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	// Clear seats first: a re-shuffle reassigns numbers already held by other
	// rows, so a straight per-row UPDATE collides on UNIQUE(competition_id,
	// pc_number) mid-transaction. NULL them all (multiple NULLs are allowed),
	// then assign the fresh, collision-free set.
	for _, a := range assignments {
		if _, err = tx.Exec(
			`UPDATE participants SET pc_number = NULL, updated_at = ? WHERE id = ?`,
			now, a.ID,
		); err != nil {
			return fmt.Errorf("shuffle seats: clear id=%d: %w", a.ID, err)
		}
	}
	for _, a := range assignments {
		_, err = tx.Exec(
			`UPDATE participants SET pc_number = ?, updated_at = ? WHERE id = ?`,
			a.Seat, now, a.ID,
		)
		if err != nil {
			return fmt.Errorf("shuffle seats: update id=%d: %w", a.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	ids := make([]int64, len(assignments))
	for i, a := range assignments {
		ids[i] = a.ID
	}
	invalidateParticipant(ids...)
	return nil
}

// ShuffleSeats assigns seats 1..N to all participants in random order.
// Returns results WITHOUT writing to DB — call UpdateParticipantSeats to persist.
func ShuffleSeats(participants []*model.Participant) []ShuffleResult {
	seats := make([]int, len(participants))
	for i := range seats {
		seats[i] = i + 1
	}
	rand.Shuffle(len(seats), func(i, j int) { seats[i], seats[j] = seats[j], seats[i] })
	results := make([]ShuffleResult, len(participants))
	for i, p := range participants {
		results[i] = ShuffleResult{ID: p.ID, Seat: seats[i], Name: p.Name, School: p.School}
	}
	return results
}
