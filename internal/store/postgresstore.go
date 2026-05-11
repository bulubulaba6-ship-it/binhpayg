package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/misc"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	log "github.com/sirupsen/logrus"
)

const (
	defaultConfigTable  = "config_store"
	defaultAuthTable    = "auth_store"
	defaultBillingTable = "billing_sessions"
	defaultUsageTable   = "usage_store"
	defaultConfigKey    = "config"
	defaultUsageKey     = "usage"
)

// PostgresStoreConfig captures configuration required to initialize a Postgres-backed store.
type PostgresStoreConfig struct {
	DSN          string
	Schema       string
	ConfigTable  string
	AuthTable    string
	BillingTable string
	UsageTable   string
	SpoolDir     string
}

// PostgresStore persists configuration and authentication metadata using PostgreSQL as backend
// while mirroring data to a local workspace so existing file-based workflows continue to operate.
type PostgresStore struct {
	db         *sql.DB
	cfg        PostgresStoreConfig
	spoolRoot  string
	configPath string
	authDir    string
	mu         sync.Mutex

	activeTokensMu sync.Mutex
	activeTokens   map[string]*cliproxyusage.Detail
}

// NewPostgresStore establishes a connection to PostgreSQL and prepares the local workspace.
func NewPostgresStore(ctx context.Context, cfg PostgresStoreConfig) (*PostgresStore, error) {
	trimmedDSN := strings.TrimSpace(cfg.DSN)
	if trimmedDSN == "" {
		return nil, fmt.Errorf("postgres store: DSN is required")
	}
	cfg.DSN = trimmedDSN
	if cfg.ConfigTable == "" {
		cfg.ConfigTable = defaultConfigTable
	}
	if cfg.AuthTable == "" {
		cfg.AuthTable = defaultAuthTable
	}
	if cfg.BillingTable == "" {
		cfg.BillingTable = defaultBillingTable
	}
	if cfg.UsageTable == "" {
		cfg.UsageTable = defaultUsageTable
	}

	spoolRoot := strings.TrimSpace(cfg.SpoolDir)
	if spoolRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			spoolRoot = filepath.Join(cwd, "pgstore")
		} else {
			spoolRoot = filepath.Join(os.TempDir(), "pgstore")
		}
	}
	absSpool, err := filepath.Abs(spoolRoot)
	if err != nil {
		return nil, fmt.Errorf("postgres store: resolve spool directory: %w", err)
	}
	configDir := filepath.Join(absSpool, "config")
	authDir := filepath.Join(absSpool, "auths")
	if err = os.MkdirAll(configDir, 0o700); err != nil {
		return nil, fmt.Errorf("postgres store: create config directory: %w", err)
	}
	if err = os.MkdirAll(authDir, 0o700); err != nil {
		return nil, fmt.Errorf("postgres store: create auth directory: %w", err)
	}

	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres store: open database connection: %w", err)
	}
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres store: ping database: %w", err)
	}

	store := &PostgresStore{
		db:           db,
		cfg:          cfg,
		spoolRoot:    absSpool,
		configPath:   filepath.Join(configDir, "config.yaml"),
		authDir:      authDir,
		activeTokens: make(map[string]*cliproxyusage.Detail),
	}
	return store, nil
}

// Close releases the underlying database connection.
func (s *PostgresStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// EnsureSchema creates the required tables (and schema when provided).
func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres store: not initialized")
	}
	if schema := strings.TrimSpace(s.cfg.Schema); schema != "" {
		query := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quoteIdentifier(schema))
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("postgres store: create schema: %w", err)
		}
	}
	configTable := s.fullTableName(s.cfg.ConfigTable)
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`, configTable)); err != nil {
		return fmt.Errorf("postgres store: create config table: %w", err)
	}
	authTable := s.fullTableName(s.cfg.AuthTable)
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			content JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`, authTable)); err != nil {
		return fmt.Errorf("postgres store: create auth table: %w", err)
	}
	usageTable := s.fullTableName(s.cfg.UsageTable)
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			content JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`, usageTable)); err != nil {
		return fmt.Errorf("postgres store: create usage table: %w", err)
	}
	billingTable := s.fullTableName(s.cfg.BillingTable)
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			session_id TEXT PRIMARY KEY,
			principal TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			auth_id TEXT NOT NULL DEFAULT '',
			auth_index TEXT NOT NULL DEFAULT '',
			started_at TIMESTAMPTZ NOT NULL,
			last_seen_at TIMESTAMPTZ NOT NULL,
			finished_at TIMESTAMPTZ,
			duration_seconds BIGINT NOT NULL DEFAULT 0,
			credits DOUBLE PRECISION NOT NULL DEFAULT 0,
			finalized BOOLEAN NOT NULL DEFAULT FALSE,
			input_tokens BIGINT NOT NULL DEFAULT 0,
			output_tokens BIGINT NOT NULL DEFAULT 0,
			reasoning_tokens BIGINT NOT NULL DEFAULT 0,
			cached_tokens BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`, billingTable)); err != nil {
		return fmt.Errorf("postgres store: create billing table: %w", err)
	}
	
	// Harden schema migration: run each update independently to ensure success even if columns already exist.
	alterQueries := []string{
		fmt.Sprintf("ALTER TABLE %s ALTER COLUMN credits TYPE DOUBLE PRECISION", billingTable),
		fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS input_tokens BIGINT NOT NULL DEFAULT 0", billingTable),
		fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS output_tokens BIGINT NOT NULL DEFAULT 0", billingTable),
		fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS reasoning_tokens BIGINT NOT NULL DEFAULT 0", billingTable),
		fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS cached_tokens BIGINT NOT NULL DEFAULT 0", billingTable),
		fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS principal TEXT NOT NULL DEFAULT ''", billingTable),
	}

	for _, q := range alterQueries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			log.WithError(err).Debugf("postgres store: migration step skipped or failed (this is usually fine if column exists)")
		}
	}
	
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS %s ON %s (principal)
	`, quoteIdentifier(s.cfg.BillingTable+"_principal_idx"), billingTable)); err != nil {
		return fmt.Errorf("postgres store: create billing index: %w", err)
	}
	return nil
}

// BeginExecutionSession persists or refreshes the active state for a session.
func (s *PostgresStore) BeginExecutionSession(ctx context.Context, record cliproxyauth.ExecutionSessionRecord) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres store: not initialized")
	}
	sessionID := strings.TrimSpace(record.SessionID)
	if sessionID == "" {
		return fmt.Errorf("postgres store: execution session id is empty")
	}
	startedAt := record.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = startedAt
	}
	query := fmt.Sprintf(`
		INSERT INTO %s AS target (
			session_id, principal, provider, model, auth_id, auth_index,
			started_at, last_seen_at, finalized, created_at, updated_at,
			input_tokens, output_tokens, reasoning_tokens, cached_tokens, credits
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, FALSE, NOW(), $8, 0, 0, 0, 0, 0)
		ON CONFLICT (session_id) DO UPDATE
		SET principal = EXCLUDED.principal,
			provider = EXCLUDED.provider,
			model = EXCLUDED.model,
			auth_id = EXCLUDED.auth_id,
			auth_index = EXCLUDED.auth_index,
			last_seen_at = EXCLUDED.last_seen_at,
			updated_at = EXCLUDED.updated_at,
			started_at = LEAST(target.started_at, EXCLUDED.started_at)
		WHERE NOT target.finalized
	`, s.fullTableName(s.cfg.BillingTable))
	if _, err := s.db.ExecContext(ctx, query,
		sessionID,
		strings.TrimSpace(record.Principal),
		strings.TrimSpace(record.Provider),
		strings.TrimSpace(record.Model),
		strings.TrimSpace(record.AuthID),
		strings.TrimSpace(record.AuthIndex),
		startedAt,
		updatedAt,
	); err != nil {
		return fmt.Errorf("postgres store: begin execution session: %w", err)
	}
	return nil
}

// HandleUsage intercepts usage records and accumulates token counts for the active session.
func (s *PostgresStore) HandleUsage(ctx context.Context, record cliproxyusage.Record) {
	if s == nil {
		return
	}
	sessionID := strings.TrimSpace(record.SessionID)
	if sessionID == "" {
		return
	}
	s.activeTokensMu.Lock()
	
	if s.activeTokens == nil {
		s.activeTokens = make(map[string]*cliproxyusage.Detail)
	}
	
	detail, ok := s.activeTokens[sessionID]
	if !ok {
		detail = &cliproxyusage.Detail{}
		s.activeTokens[sessionID] = detail
	}
	// Calculate the delta (incremental difference) to support cumulative usage updates
	inDelta := record.Detail.InputTokens - detail.InputTokens
	outDelta := record.Detail.OutputTokens - detail.OutputTokens
	reasonDelta := record.Detail.ReasoningTokens - detail.ReasoningTokens
	cacheDelta := record.Detail.CachedTokens - detail.CachedTokens

	// Update memory state with the new absolute values
	detail.InputTokens = record.Detail.InputTokens
	detail.OutputTokens = record.Detail.OutputTokens
	detail.ReasoningTokens = record.Detail.ReasoningTokens
	detail.CachedTokens = record.Detail.CachedTokens
	s.activeTokensMu.Unlock()

	// 2. Proactively update database (Additive with Delta)
	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Calculate credits for the incremental difference only
		creditDelta := cliproxyauth.CalculateTokenCost(record.Model, inDelta, outDelta, reasonDelta, cacheDelta)

		query := fmt.Sprintf(`
			INSERT INTO %s (
				session_id, model, input_tokens, output_tokens, reasoning_tokens, cached_tokens, credits, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
			ON CONFLICT (session_id) DO UPDATE
			SET input_tokens = COALESCE(target.input_tokens, 0) + EXCLUDED.input_tokens,
				output_tokens = COALESCE(target.output_tokens, 0) + EXCLUDED.output_tokens,
				reasoning_tokens = COALESCE(target.reasoning_tokens, 0) + EXCLUDED.reasoning_tokens,
				cached_tokens = COALESCE(target.cached_tokens, 0) + EXCLUDED.cached_tokens,
				credits = COALESCE(target.credits, 0) + EXCLUDED.credits,
				updated_at = NOW()
		`, s.fullTableName(s.cfg.BillingTable))

		_, _ = s.db.ExecContext(updateCtx, query,
			sessionID,
			record.Model,
			inDelta,
			outDelta,
			reasonDelta,
			cacheDelta,
			creditDelta,
		)
	}()
}

// FinalizeExecutionSession marks a session as finished and records its credit bucket.
func (s *PostgresStore) FinalizeExecutionSession(ctx context.Context, sessionID string, finishedAt time.Time) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("postgres store: not initialized")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("postgres store: execution session id is empty")
	}
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres store: begin finalize transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var (
		startedAt time.Time
		finalized bool
		modelStr  string
	)
	selectQuery := fmt.Sprintf(`
		SELECT started_at, finalized, model
		FROM %s
		WHERE session_id = $1
		FOR UPDATE
	`, s.fullTableName(s.cfg.BillingTable))
	err = tx.QueryRowContext(ctx, selectQuery, sessionID).Scan(&startedAt, &finalized, &modelStr)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		startedAt = finishedAt
	case err != nil:
		return fmt.Errorf("postgres store: load execution session: %w", err)
	case finalized:
		if commitErr := tx.Commit(); commitErr != nil {
			return fmt.Errorf("postgres store: commit finalized execution session: %w", commitErr)
		}
		return nil
	}
	if finishedAt.Before(startedAt) {
		finishedAt = startedAt
	}
	duration := finishedAt.Sub(startedAt)
	
	s.activeTokensMu.Lock()
	tokens := s.activeTokens[sessionID]
	delete(s.activeTokens, sessionID)
	s.activeTokensMu.Unlock()
	
	var inTok, outTok, reasonTok, cacheTok int64
	if tokens != nil {
		inTok = tokens.InputTokens
		outTok = tokens.OutputTokens
		reasonTok = tokens.ReasoningTokens
		cacheTok = tokens.CachedTokens
	}
	
	credits := cliproxyauth.CalculateTokenCost(modelStr, inTok, outTok, reasonTok, cacheTok)
	upsertQuery := fmt.Sprintf(`
		INSERT INTO %s AS target (
			session_id, principal, provider, model, auth_id, auth_index,
			started_at, last_seen_at, finished_at, duration_seconds, credits,
			finalized, input_tokens, output_tokens, reasoning_tokens, cached_tokens, created_at, updated_at
		)
		VALUES ($1, '', '', '', '', '', $2, $2, $2, $3, $4, TRUE, $5, $6, $7, $8, NOW(), NOW())
		ON CONFLICT (session_id) DO UPDATE
		SET started_at = LEAST(target.started_at, EXCLUDED.started_at),
			last_seen_at = GREATEST(target.last_seen_at, EXCLUDED.last_seen_at),
			finished_at = EXCLUDED.finished_at,
			duration_seconds = EXCLUDED.duration_seconds,
			credits = EXCLUDED.credits,
			input_tokens = EXCLUDED.input_tokens,
			output_tokens = EXCLUDED.output_tokens,
			reasoning_tokens = EXCLUDED.reasoning_tokens,
			cached_tokens = EXCLUDED.cached_tokens,
			finalized = TRUE,
			updated_at = NOW(),
			-- Preserve the principal written by BeginExecutionSession; never overwrite with empty
			principal = CASE WHEN target.principal <> '' THEN target.principal ELSE EXCLUDED.principal END
		WHERE NOT target.finalized
	`, s.fullTableName(s.cfg.BillingTable))
	if _, err = tx.ExecContext(ctx, upsertQuery, sessionID, finishedAt, int64(duration/time.Second), credits, inTok, outTok, reasonTok, cacheTok); err != nil {
		return fmt.Errorf("postgres store: finalize execution session: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("postgres store: commit execution session finalization: %w", err)
	}
	return nil
}

// GetExecutionQuotaSummary returns quota usage aggregated by caller principal.
func (s *PostgresStore) GetExecutionQuotaSummary(ctx context.Context, principal string) (cliproxyauth.ExecutionQuotaSnapshot, error) {
	if s == nil || s.db == nil {
		return cliproxyauth.ExecutionQuotaSnapshot{}, fmt.Errorf("postgres store: not initialized")
	}
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return cliproxyauth.ExecutionQuotaSnapshot{}, fmt.Errorf("postgres store: principal is empty")
	}
	query := fmt.Sprintf(`
		WITH principal_stats AS (
			SELECT
				COUNT(session_id) AS sessions,
				SUM(CASE WHEN finalized = FALSE THEN 1 ELSE 0 END) AS active_sessions,
				MAX(last_seen_at) AS last_session_at,
				-- 5h Window
				COALESCE(SUM(CASE WHEN started_at >= NOW() - INTERVAL '5 hours' THEN credits ELSE 0 END), 0) AS interval_credits,
				COALESCE(SUM(CASE WHEN started_at >= NOW() - INTERVAL '5 hours' THEN 1 ELSE 0 END), 0) AS interval_requests,
				COALESCE(SUM(CASE WHEN started_at >= NOW() - INTERVAL '5 hours' THEN (input_tokens + output_tokens + reasoning_tokens + cached_tokens) ELSE 0 END), 0) AS interval_tokens,
				COALESCE(SUM(CASE WHEN started_at >= NOW() - INTERVAL '5 hours' THEN duration_seconds ELSE 0 END), 0) AS interval_duration,
				-- Daily Window
				COALESCE(SUM(CASE WHEN started_at >= NOW() - INTERVAL '24 hours' THEN credits ELSE 0 END), 0) AS daily_credits,
				COALESCE(SUM(CASE WHEN started_at >= NOW() - INTERVAL '24 hours' THEN 1 ELSE 0 END), 0) AS daily_requests,
				COALESCE(SUM(CASE WHEN started_at >= NOW() - INTERVAL '24 hours' THEN (input_tokens + output_tokens + reasoning_tokens + cached_tokens) ELSE 0 END), 0) AS daily_tokens,
				COALESCE(SUM(CASE WHEN started_at >= NOW() - INTERVAL '24 hours' THEN duration_seconds ELSE 0 END), 0) AS daily_duration,
				-- Monthly Window
				COALESCE(SUM(CASE WHEN started_at >= NOW() - INTERVAL '30 days' THEN credits ELSE 0 END), 0) AS monthly_credits,
				COALESCE(SUM(CASE WHEN started_at >= NOW() - INTERVAL '30 days' THEN 1 ELSE 0 END), 0) AS monthly_requests,
				COALESCE(SUM(CASE WHEN started_at >= NOW() - INTERVAL '30 days' THEN (input_tokens + output_tokens + reasoning_tokens + cached_tokens) ELSE 0 END), 0) AS monthly_tokens,
				COALESCE(SUM(CASE WHEN started_at >= NOW() - INTERVAL '30 days' THEN duration_seconds ELSE 0 END), 0) AS monthly_duration,
				-- Lifetime Window
				COALESCE(SUM(credits), 0) AS lifetime_credits,
				COALESCE(SUM(input_tokens + output_tokens + reasoning_tokens + cached_tokens), 0) AS lifetime_tokens,
				COALESCE(SUM(duration_seconds), 0) AS lifetime_duration
			FROM %s
			WHERE principal = $1
		)
		SELECT * FROM principal_stats
	`, s.fullTableName(s.cfg.BillingTable))
	var summary cliproxyauth.ExecutionQuotaSnapshot
	var lastSessionAt sql.NullTime
	if err := s.db.QueryRowContext(ctx, query, principal).Scan(
		&summary.Sessions,
		&summary.ActiveSessions,
		&lastSessionAt,
		&summary.Interval.Credits, &summary.Interval.Requests, &summary.Interval.Tokens, &summary.Interval.Duration,
		&summary.Daily.Credits, &summary.Daily.Requests, &summary.Daily.Tokens, &summary.Daily.Duration,
		&summary.Monthly.Credits, &summary.Monthly.Requests, &summary.Monthly.Tokens, &summary.Monthly.Duration,
		&summary.Lifetime.Credits, &summary.Lifetime.Tokens, &summary.Lifetime.Duration,
	); err != nil {
		return cliproxyauth.ExecutionQuotaSnapshot{}, fmt.Errorf("postgres store: load execution quota summary: %w", err)
	}
	if lastSessionAt.Valid {
		summary.LastSessionAt = lastSessionAt.Time
	}

	// Calculate Throughput (TPM)
	summary.Interval.Throughput = float64(summary.Interval.Tokens) / 300.0
	summary.Daily.Throughput = float64(summary.Daily.Tokens) / 1440.0
	summary.Monthly.Throughput = float64(summary.Monthly.Tokens) / 43200.0
	summary.Lifetime.Throughput = float64(summary.Lifetime.Tokens) / 43200.0

	// Backward compatibility
	summary.CreditsUsed = summary.Interval.Credits
	summary.TotalCreditsUsed = summary.Lifetime.Credits
	summary.CreditLimit = 100

	// Incorporate active in-memory tokens into the summary
	now := time.Now().UTC()
	rolling5h := now.Add(-5 * time.Hour)
	rolling1d := now.Add(-24 * time.Hour)
	rolling30d := now.Add(-30 * 24 * time.Hour)

	s.activeTokensMu.Lock()
	if len(s.activeTokens) > 0 {
		// Identify which active sessions belong to this principal
		activeSessionsQuery := fmt.Sprintf(`
			SELECT session_id, started_at, (input_tokens + output_tokens + reasoning_tokens + cached_tokens) as db_tokens
			FROM %s
			WHERE principal = $1 AND finalized = FALSE
		`, s.fullTableName(s.cfg.BillingTable))
		rows, err := s.db.QueryContext(ctx, activeSessionsQuery, principal)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var sid string
				var startedAt time.Time
				var dbToks int64
				if err := rows.Scan(&sid, &startedAt, &dbToks); err == nil {
					if memToks, ok := s.activeTokens[sid]; ok && memToks != nil {
						// memToks are the TOTAL tokens seen so far in memory.
						// The DB record for an active session might already have some tokens if it was partially flushed.
						// But currently BeginExecutionSession doesn't flush tokens.
						delta := (memToks.InputTokens + memToks.OutputTokens + memToks.ReasoningTokens + memToks.CachedTokens) - dbToks
						if delta > 0 {
							summary.Lifetime.Tokens += delta
							if !startedAt.Before(rolling30d) {
								summary.Monthly.Tokens += delta
							}
							if !startedAt.Before(rolling1d) {
								summary.Daily.Tokens += delta
							}
							if !startedAt.Before(rolling5h) {
								summary.Interval.Tokens += delta
							}
						}
					}
				}
			}
		}
	}
	s.activeTokensMu.Unlock()

	// Re-calculate Throughput (TPM) after adding active tokens
	summary.Interval.Throughput = float64(summary.Interval.Tokens) / 300.0
	summary.Daily.Throughput = float64(summary.Daily.Tokens) / 1440.0
	summary.Monthly.Throughput = float64(summary.Monthly.Tokens) / 43200.0
	summary.Lifetime.Throughput = float64(summary.Lifetime.Tokens) / 43200.0

	recentQuery := fmt.Sprintf(`
		SELECT
			session_id, provider, model, principal, started_at, updated_at,
			duration_seconds, credits, input_tokens, output_tokens, reasoning_tokens, cached_tokens
		FROM %s
		WHERE principal = $1
		ORDER BY started_at DESC
		LIMIT 20
	`, s.fullTableName(s.cfg.BillingTable))
	
	recentRows, err := s.db.QueryContext(ctx, recentQuery, principal)
	if err == nil {
		defer recentRows.Close()
		for recentRows.Next() {
			var (
				recSessionID, recProvider, recModel, recPrincipal string
				recStartedAt, recUpdatedAt                        time.Time
				recDuration                                       int64
				recCredits                                        float64
				recInput, recOutput, recReasoning, recCached       int64
			)
			if err := recentRows.Scan(
				&recSessionID, &recProvider, &recModel, &recPrincipal,
				&recStartedAt, &recUpdatedAt, &recDuration, &recCredits,
				&recInput, &recOutput, &recReasoning, &recCached,
			); err == nil {
				summary.RecentSessions = append(summary.RecentSessions, cliproxyauth.ExecutionSessionSummary{
					ExecutionSessionRecord: cliproxyauth.ExecutionSessionRecord{
						SessionID:       recSessionID,
						Principal:       recPrincipal,
						Provider:        recProvider,
						Model:           recModel,
						StartedAt:       recStartedAt,
						UpdatedAt:       recUpdatedAt,
						InputTokens:     recInput,
						OutputTokens:    recOutput,
						ReasoningTokens: recReasoning,
						CachedTokens:    recCached,
					},
					DurationSeconds: recDuration,
					Credits:         recCredits,
				})
			}
		}
	}

	return summary, nil
}

// Bootstrap synchronizes configuration and auth records between PostgreSQL and the local workspace.
func (s *PostgresStore) Bootstrap(ctx context.Context, exampleConfigPath string) error {
	if err := s.EnsureSchema(ctx); err != nil {
		return err
	}
	if err := s.syncConfigFromDatabase(ctx, exampleConfigPath); err != nil {
		return err
	}
	if err := s.syncAuthFromDatabase(ctx); err != nil {
		return err
	}
	if err := s.syncUsageFromDatabase(ctx); err != nil {
		return err
	}
	return nil
}

// ConfigPath returns the managed configuration file path inside the spool directory.
func (s *PostgresStore) ConfigPath() string {
	if s == nil {
		return ""
	}
	return s.configPath
}

// AuthDir returns the local directory containing mirrored auth files.
func (s *PostgresStore) AuthDir() string {
	if s == nil {
		return ""
	}
	return s.authDir
}

// WorkDir exposes the root spool directory used for mirroring.
func (s *PostgresStore) WorkDir() string {
	if s == nil {
		return ""
	}
	return s.spoolRoot
}

// SetBaseDir implements the optional interface used by authenticators; it is a no-op because
// the Postgres-backed store controls its own workspace.
func (s *PostgresStore) SetBaseDir(string) {}

// Save persists authentication metadata to disk and PostgreSQL.
func (s *PostgresStore) Save(ctx context.Context, auth *cliproxyauth.Auth) (string, error) {
	if auth == nil {
		return "", fmt.Errorf("postgres store: auth is nil")
	}

	path, err := s.resolveAuthPath(auth)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("postgres store: missing file path attribute for %s", auth.ID)
	}

	if auth.Disabled {
		if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
			return "", nil
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("postgres store: create auth directory: %w", err)
	}

	switch {
	case auth.Storage != nil:
		if err = auth.Storage.SaveTokenToFile(path); err != nil {
			return "", err
		}
	case auth.Metadata != nil:
		raw, errMarshal := json.Marshal(auth.Metadata)
		if errMarshal != nil {
			return "", fmt.Errorf("postgres store: marshal metadata: %w", errMarshal)
		}
		if existing, errRead := os.ReadFile(path); errRead == nil {
			if jsonEqual(existing, raw) {
				return path, nil
			}
		} else if errRead != nil && !errors.Is(errRead, fs.ErrNotExist) {
			return "", fmt.Errorf("postgres store: read existing metadata: %w", errRead)
		}
		tmp := path + ".tmp"
		if errWrite := os.WriteFile(tmp, raw, 0o600); errWrite != nil {
			return "", fmt.Errorf("postgres store: write temp auth file: %w", errWrite)
		}
		if errRename := os.Rename(tmp, path); errRename != nil {
			return "", fmt.Errorf("postgres store: rename auth file: %w", errRename)
		}
	default:
		return "", fmt.Errorf("postgres store: nothing to persist for %s", auth.ID)
	}

	if auth.Attributes == nil {
		auth.Attributes = make(map[string]string)
	}
	auth.Attributes["path"] = path

	if strings.TrimSpace(auth.FileName) == "" {
		auth.FileName = auth.ID
	}

	relID, err := s.relativeAuthID(path)
	if err != nil {
		return "", err
	}
	if err = s.upsertAuthRecord(ctx, relID, path); err != nil {
		return "", err
	}
	return path, nil
}

// List enumerates all auth records stored in PostgreSQL.
func (s *PostgresStore) List(ctx context.Context) ([]*cliproxyauth.Auth, error) {
	query := fmt.Sprintf("SELECT id, content, created_at, updated_at FROM %s ORDER BY id", s.fullTableName(s.cfg.AuthTable))
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("postgres store: list auth: %w", err)
	}
	defer rows.Close()

	auths := make([]*cliproxyauth.Auth, 0, 32)
	for rows.Next() {
		var (
			id        string
			payload   string
			createdAt time.Time
			updatedAt time.Time
		)
		if err = rows.Scan(&id, &payload, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("postgres store: scan auth row: %w", err)
		}
		path, errPath := s.absoluteAuthPath(id)
		if errPath != nil {
			log.WithError(errPath).Warnf("postgres store: skipping auth %s outside spool", id)
			continue
		}
		metadata := make(map[string]any)
		if err = json.Unmarshal([]byte(payload), &metadata); err != nil {
			log.WithError(err).Warnf("postgres store: skipping auth %s with invalid json", id)
			continue
		}
		provider := strings.TrimSpace(valueAsString(metadata["type"]))
		if provider == "" {
			provider = "unknown"
		}
		attr := map[string]string{"path": path}
		if email := strings.TrimSpace(valueAsString(metadata["email"])); email != "" {
			attr["email"] = email
		}
		auth := &cliproxyauth.Auth{
			ID:               normalizeAuthID(id),
			Provider:         provider,
			FileName:         normalizeAuthID(id),
			Label:            labelFor(metadata),
			Status:           cliproxyauth.StatusActive,
			Attributes:       attr,
			Metadata:         metadata,
			CreatedAt:        createdAt,
			UpdatedAt:        updatedAt,
			LastRefreshedAt:  time.Time{},
			NextRefreshAfter: time.Time{},
		}
		cliproxyauth.ApplyCustomHeadersFromMetadata(auth)
		auths = append(auths, auth)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres store: iterate auth rows: %w", err)
	}
	return auths, nil
}

// Delete removes an auth file and the corresponding database record.
func (s *PostgresStore) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("postgres store: id is empty")
	}
	path, err := s.resolveDeletePath(id)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err = os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("postgres store: delete auth file: %w", err)
	}
	relID, err := s.relativeAuthID(path)
	if err != nil {
		return err
	}
	return s.deleteAuthRecord(ctx, relID)
}

// PersistAuthFiles stores the provided auth file changes in PostgreSQL.
func (s *PostgresStore) PersistAuthFiles(ctx context.Context, _ string, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range paths {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		relID, err := s.relativeAuthID(trimmed)
		if err != nil {
			// Attempt to resolve absolute path under authDir.
			abs := trimmed
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(s.authDir, trimmed)
			}
			relID, err = s.relativeAuthID(abs)
			if err != nil {
				log.WithError(err).Warnf("postgres store: ignoring auth path %s", trimmed)
				continue
			}
			trimmed = abs
		}
		if err = s.syncAuthFile(ctx, relID, trimmed); err != nil {
			return err
		}
	}
	return nil
}

// PersistConfig mirrors the local configuration file to PostgreSQL.
func (s *PostgresStore) PersistConfig(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s.deleteConfigRecord(ctx)
		}
		return fmt.Errorf("postgres store: read config file: %w", err)
	}
	return s.persistConfig(ctx, data)
}

// syncConfigFromDatabase writes the database-stored config to disk or seeds the database from template.
func (s *PostgresStore) syncConfigFromDatabase(ctx context.Context, exampleConfigPath string) error {
	query := fmt.Sprintf("SELECT content FROM %s WHERE id = $1", s.fullTableName(s.cfg.ConfigTable))
	var content string
	err := s.db.QueryRowContext(ctx, query, defaultConfigKey).Scan(&content)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, errStat := os.Stat(s.configPath); errors.Is(errStat, fs.ErrNotExist) {
			if exampleConfigPath != "" {
				if errCopy := misc.CopyConfigTemplate(exampleConfigPath, s.configPath); errCopy != nil {
					return fmt.Errorf("postgres store: copy example config: %w", errCopy)
				}
			} else {
				if errCreate := os.MkdirAll(filepath.Dir(s.configPath), 0o700); errCreate != nil {
					return fmt.Errorf("postgres store: prepare config directory: %w", errCreate)
				}
				if errWrite := os.WriteFile(s.configPath, []byte{}, 0o600); errWrite != nil {
					return fmt.Errorf("postgres store: create empty config: %w", errWrite)
				}
			}
		}
		data, errRead := os.ReadFile(s.configPath)
		if errRead != nil {
			return fmt.Errorf("postgres store: read local config: %w", errRead)
		}
		if errPersist := s.persistConfig(ctx, data); errPersist != nil {
			return errPersist
		}
	case err != nil:
		return fmt.Errorf("postgres store: load config from database: %w", err)
	default:
		if err = os.MkdirAll(filepath.Dir(s.configPath), 0o700); err != nil {
			return fmt.Errorf("postgres store: prepare config directory: %w", err)
		}
		normalized := normalizeLineEndings(content)
		if err = os.WriteFile(s.configPath, []byte(normalized), 0o600); err != nil {
			return fmt.Errorf("postgres store: write config to spool: %w", err)
		}
	}
	return nil
}

// syncAuthFromDatabase populates the local auth directory from PostgreSQL data.
func (s *PostgresStore) syncAuthFromDatabase(ctx context.Context) error {
	query := fmt.Sprintf("SELECT id, content FROM %s", s.fullTableName(s.cfg.AuthTable))
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("postgres store: load auth from database: %w", err)
	}
	defer rows.Close()

	if err = os.RemoveAll(s.authDir); err != nil {
		return fmt.Errorf("postgres store: reset auth directory: %w", err)
	}
	if err = os.MkdirAll(s.authDir, 0o700); err != nil {
		return fmt.Errorf("postgres store: recreate auth directory: %w", err)
	}

	for rows.Next() {
		var (
			id      string
			payload string
		)
		if err = rows.Scan(&id, &payload); err != nil {
			return fmt.Errorf("postgres store: scan auth row: %w", err)
		}
		path, errPath := s.absoluteAuthPath(id)
		if errPath != nil {
			log.WithError(errPath).Warnf("postgres store: skipping auth %s outside spool", id)
			continue
		}
		if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("postgres store: create auth subdir: %w", err)
		}
		if err = os.WriteFile(path, []byte(payload), 0o600); err != nil {
			return fmt.Errorf("postgres store: write auth file: %w", err)
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("postgres store: iterate auth rows: %w", err)
	}
	return nil
}

func (s *PostgresStore) syncAuthFile(ctx context.Context, relID, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s.deleteAuthRecord(ctx, relID)
		}
		return fmt.Errorf("postgres store: read auth file: %w", err)
	}
	if len(data) == 0 {
		return s.deleteAuthRecord(ctx, relID)
	}
	return s.persistAuth(ctx, relID, data)
}

func (s *PostgresStore) upsertAuthRecord(ctx context.Context, relID, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("postgres store: read auth file: %w", err)
	}
	if len(data) == 0 {
		return s.deleteAuthRecord(ctx, relID)
	}
	return s.persistAuth(ctx, relID, data)
}

func (s *PostgresStore) persistAuth(ctx context.Context, relID string, data []byte) error {
	jsonPayload := json.RawMessage(data)
	query := fmt.Sprintf(`
		INSERT INTO %s (id, content, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (id)
		DO UPDATE SET content = EXCLUDED.content, updated_at = NOW()
	`, s.fullTableName(s.cfg.AuthTable))
	if _, err := s.db.ExecContext(ctx, query, relID, jsonPayload); err != nil {
		return fmt.Errorf("postgres store: upsert auth record: %w", err)
	}
	return nil
}

func (s *PostgresStore) deleteAuthRecord(ctx context.Context, relID string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", s.fullTableName(s.cfg.AuthTable))
	if _, err := s.db.ExecContext(ctx, query, relID); err != nil {
		return fmt.Errorf("postgres store: delete auth record: %w", err)
	}
	return nil
}

func (s *PostgresStore) persistConfig(ctx context.Context, data []byte) error {
	query := fmt.Sprintf(`
		INSERT INTO %s (id, content, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (id)
		DO UPDATE SET content = EXCLUDED.content, updated_at = NOW()
	`, s.fullTableName(s.cfg.ConfigTable))
	normalized := normalizeLineEndings(string(data))
	if _, err := s.db.ExecContext(ctx, query, defaultConfigKey, normalized); err != nil {
		return fmt.Errorf("postgres store: upsert config: %w", err)
	}
	return nil
}

func (s *PostgresStore) deleteConfigRecord(ctx context.Context) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", s.fullTableName(s.cfg.ConfigTable))
	if _, err := s.db.ExecContext(ctx, query, defaultConfigKey); err != nil {
		return fmt.Errorf("postgres store: delete config: %w", err)
	}
	return nil
}

func (s *PostgresStore) resolveAuthPath(auth *cliproxyauth.Auth) (string, error) {
	if auth == nil {
		return "", fmt.Errorf("postgres store: auth is nil")
	}
	if auth.Attributes != nil {
		if p := strings.TrimSpace(auth.Attributes["path"]); p != "" {
			return p, nil
		}
	}
	if fileName := strings.TrimSpace(auth.FileName); fileName != "" {
		if filepath.IsAbs(fileName) {
			return fileName, nil
		}
		return filepath.Join(s.authDir, fileName), nil
	}
	if auth.ID == "" {
		return "", fmt.Errorf("postgres store: missing id")
	}
	if filepath.IsAbs(auth.ID) {
		return auth.ID, nil
	}
	return filepath.Join(s.authDir, filepath.FromSlash(auth.ID)), nil
}

func (s *PostgresStore) resolveDeletePath(id string) (string, error) {
	if strings.ContainsRune(id, os.PathSeparator) || filepath.IsAbs(id) {
		return id, nil
	}
	return filepath.Join(s.authDir, filepath.FromSlash(id)), nil
}

func (s *PostgresStore) relativeAuthID(path string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("postgres store: store not initialized")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.authDir, path)
	}
	clean := filepath.Clean(path)
	rel, err := filepath.Rel(s.authDir, clean)
	if err != nil {
		return "", fmt.Errorf("postgres store: compute relative path: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("postgres store: path %s outside managed directory", path)
	}
	return filepath.ToSlash(rel), nil
}

func (s *PostgresStore) absoluteAuthPath(id string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("postgres store: store not initialized")
	}
	clean := filepath.Clean(filepath.FromSlash(id))
	if strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("postgres store: invalid auth identifier %s", id)
	}
	path := filepath.Join(s.authDir, clean)
	rel, err := filepath.Rel(s.authDir, path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("postgres store: resolved auth path escapes auth directory")
	}
	return path, nil
}

func (s *PostgresStore) fullTableName(name string) string {
	if strings.TrimSpace(s.cfg.Schema) == "" {
		return quoteIdentifier(name)
	}
	return quoteIdentifier(s.cfg.Schema) + "." + quoteIdentifier(name)
}

func quoteIdentifier(identifier string) string {
	replaced := strings.ReplaceAll(identifier, "\"", "\"\"")
	return "\"" + replaced + "\""
}

// syncUsageFromDatabase loads the usage statistics snapshot from the database and merges it.
func (s *PostgresStore) syncUsageFromDatabase(ctx context.Context) error {
	query := fmt.Sprintf("SELECT content FROM %s WHERE id = $1", s.fullTableName(s.cfg.UsageTable))
	var payload []byte
	err := s.db.QueryRowContext(ctx, query, defaultUsageKey).Scan(&payload)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("postgres store: load usage from database: %w", err)
	}
	var snapshot usage.StatisticsSnapshot
	if err = json.Unmarshal(payload, &snapshot); err != nil {
		log.WithError(err).Warn("postgres store: failed to unmarshal usage snapshot from database")
		return nil
	}
	stats := usage.GetRequestStatistics()
	if stats != nil {
		stats.MergeSnapshot(snapshot)
	}
	return nil
}

// PersistUsage saves the current memory usage snapshot into PostgreSQL.
func (s *PostgresStore) PersistUsage(ctx context.Context) error {
	stats := usage.GetRequestStatistics()
	if stats == nil {
		return nil
	}
	snapshot := stats.Snapshot()
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("postgres store: marshal usage snapshot: %w", err)
	}
	query := fmt.Sprintf(`
		INSERT INTO %s (id, content, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (id)
		DO UPDATE SET content = EXCLUDED.content, updated_at = NOW()
	`, s.fullTableName(s.cfg.UsageTable))
	if _, err := s.db.ExecContext(ctx, query, defaultUsageKey, json.RawMessage(payload)); err != nil {
		return fmt.Errorf("postgres store: upsert usage snapshot: %w", err)
	}
	return nil
}

// StartPeriodicUsageSync starts a background goroutine that periodically flushes
// usage statistics to the PostgresStore. It runs until the context is canceled.
func (s *PostgresStore) StartPeriodicUsageSync(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// One final flush before exit
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.PersistUsage(flushCtx)
			cancel()
			return
		case <-ticker.C:
			flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := s.PersistUsage(flushCtx); err != nil {
				log.WithError(err).Warn("postgres store: periodic usage sync failed")
			}
			cancel()
		}
	}
}

func valueAsString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return ""
	}
}

func labelFor(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if v := strings.TrimSpace(valueAsString(metadata["label"])); v != "" {
		return v
	}
	if v := strings.TrimSpace(valueAsString(metadata["email"])); v != "" {
		return v
	}
	if v := strings.TrimSpace(valueAsString(metadata["project_id"])); v != "" {
		return v
	}
	return ""
}

func normalizeAuthID(id string) string {
	return filepath.ToSlash(filepath.Clean(id))
}

func normalizeLineEndings(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}
