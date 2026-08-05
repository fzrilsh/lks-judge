package model

import "time"

// Competition maps to the competitions table.
// status: waiting | running | paused | finished
type Competition struct {
	ID               int64
	Name             string
	Level            string
	AllowedIPs       string // JSON array e.g. ["192.168.1.1"]
	CurrentModuleID  *int64
	StartDate        string // DATE
	EndDate          string // DATE
	Status           string
	RemainingSeconds *int
	PausedAt         *time.Time
	StartTime        *string // TIME
	EndTime          *string // TIME
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Module maps to the modules table.
type Module struct {
	ID            int64
	CompetitionID int64
	Name          string
	Order         int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Participant maps to the participants table.
type Participant struct {
	ID            int64
	CompetitionID int64
	Name          string
	School        string // "member" column in Excel
	PCNumber      *int   // seat number; NULL = not yet seated
	Password      string // bcrypt hash
	PlainPassword string // plaintext copy for jury re-export (internal LAN tradeoff, spec §5)
	IPAddress     *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// File maps to the files table.
type File struct {
	ID            string // UUID
	CompetitionID int64
	Name          string
	Path          string
	IsPublic      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Submission maps to the submissions table.
// UNIQUE(participant_id, module_id)
type Submission struct {
	ID            string // UUID
	ParticipantID int64
	ModuleID      int64
	Name          string
	FilePath      string
	SubmittedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Score maps to the scores table.
// wsi_score is pre-computed on every upsert — never derived at query time.
// UNIQUE(participant_id, module_id)
type Score struct {
	ID            int64
	ParticipantID int64
	ModuleID      int64
	Score         *int
	WSIScore      *int // 700 + (raw-median)*2.8, clamped [0,1000]
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Session maps to the sessions table.
// Jury is stateless (IP-only); only participants have rows here.
// expires_at = 9999-12-31 sentinel (lifetime session).
type Session struct {
	Token     string
	OwnerID   int64 // participants.id
	ExpiresAt time.Time
}

// UploadSession maps to the upload_sessions table.
// upload_type: "submission" | "file"
// uploader_role: "participant" | "jury"
// uploader_id = 0 sentinel when uploader_role = "jury"
type UploadSession struct {
	ID            string // UUID
	UploaderID    int64
	UploaderRole  string
	CompetitionID int64
	ModuleID      *int64
	Filename      string
	TotalChunks   int
	TotalSize     int64
	UploadType    string
	ExpiresAt     time.Time
	CreatedAt     time.Time
}
