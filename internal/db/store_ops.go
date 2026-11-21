package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// --- Mail Logs ---

func (s *Store) InsertMailLog(ctx context.Context, m *MailLog) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mail_logs(
			id, client_id, client_name, source, recipient, subject,
			body, status, error_message, duration_ms, client_ip, request_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ClientID, m.ClientName, m.Source, m.Recipient, m.Subject,
		m.Body, m.Status, m.ErrorMessage, m.DurationMs, m.ClientIP, m.RequestID, m.CreatedAt,
	)
	return err
}

func (s *Store) GetMailLog(ctx context.Context, id string) (*MailLog, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, client_id, client_name, source, recipient, subject,
		        body, status, error_message, duration_ms, client_ip, request_id, created_at
		 FROM mail_logs WHERE id = ?`, id)
	var m MailLog
	if err := row.Scan(
		&m.ID, &m.ClientID, &m.ClientName, &m.Source, &m.Recipient, &m.Subject,
		&m.Body, &m.Status, &m.ErrorMessage, &m.DurationMs, &m.ClientIP, &m.RequestID, &m.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

func buildMailLogWhere(f MailLogFilter) (string, []any) {
	var clauses []string
	var args []any
	if f.ClientID != "" {
		clauses = append(clauses, "client_id = ?")
		args = append(args, f.ClientID)
	}
	if f.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, f.Status)
	}
	if f.Source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, f.Source)
	}
	if f.From != nil {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		clauses = append(clauses, "created_at <= ?")
		args = append(args, *f.To)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := "%" + q + "%"
		clauses = append(clauses, "(recipient LIKE ? OR subject LIKE ? OR client_name LIKE ? OR error_message LIKE ?)")
		args = append(args, like, like, like, like)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (s *Store) ListMailLogs(ctx context.Context, f MailLogFilter) ([]MailLog, int64, error) {
	where, args := buildMailLogWhere(f)
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mail_logs`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	query := `SELECT id, client_id, client_name, source, recipient, subject,
	                 body, status, error_message, duration_ms, client_ip, request_id, created_at
	          FROM mail_logs` + where + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	qArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.QueryContext(ctx, query, qArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := make([]MailLog, 0)
	for rows.Next() {
		var m MailLog
		if err := rows.Scan(
			&m.ID, &m.ClientID, &m.ClientName, &m.Source, &m.Recipient, &m.Subject,
			&m.Body, &m.Status, &m.ErrorMessage, &m.DurationMs, &m.ClientIP, &m.RequestID, &m.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		list = append(list, m)
	}
	return list, total, rows.Err()
}

func (s *Store) ExportAllMailLogs(ctx context.Context, f MailLogFilter) ([]MailLog, error) {
	where, args := buildMailLogWhere(f)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, client_id, client_name, source, recipient, subject,
		        body, status, error_message, duration_ms, client_ip, request_id, created_at
		 FROM mail_logs`+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]MailLog, 0)
	for rows.Next() {
		var m MailLog
		if err := rows.Scan(
			&m.ID, &m.ClientID, &m.ClientName, &m.Source, &m.Recipient, &m.Subject,
			&m.Body, &m.Status, &m.ErrorMessage, &m.DurationMs, &m.ClientIP, &m.RequestID, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (s *Store) DeleteMailLogsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res1, err := s.db.ExecContext(ctx, `DELETE FROM mail_logs WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n1, _ := res1.RowsAffected()

	res2, err := s.db.ExecContext(ctx, `DELETE FROM cron_job_runs WHERE started_at < ? AND status != 'running'`, cutoff)
	if err != nil {
		return n1, err
	}
	n2, _ := res2.RowsAffected()
	return n1 + n2, nil
}

// --- Audit Logs ---

func (s *Store) InsertAuditLog(ctx context.Context, a *AuditLog) error {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_logs(
			actor_type, actor_id, actor_name, action, resource_type, resource_id,
			status, ip_address, user_agent, request_id, detail_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ActorType, a.ActorID, a.ActorName, a.Action, a.ResourceType, a.ResourceID,
		a.Status, a.IPAddress, a.UserAgent, a.RequestID, a.DetailJSON, a.CreatedAt,
	)
	return err
}

func buildAuditWhere(f AuditFilter) (string, []any) {
	var clauses []string
	var args []any
	if f.ActorType != "" {
		clauses = append(clauses, "actor_type = ?")
		args = append(args, f.ActorType)
	}
	if f.Action != "" {
		clauses = append(clauses, "action = ?")
		args = append(args, f.Action)
	}
	if f.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, f.Status)
	}
	if f.From != nil {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		clauses = append(clauses, "created_at <= ?")
		args = append(args, *f.To)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := "%" + q + "%"
		clauses = append(clauses, "(actor_name LIKE ? OR action LIKE ? OR resource_id LIKE ? OR detail_json LIKE ?)")
		args = append(args, like, like, like, like)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (s *Store) ListAuditLogs(ctx context.Context, f AuditFilter) ([]AuditLog, int64, error) {
	where, args := buildAuditWhere(f)
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	query := `SELECT id, actor_type, actor_id, actor_name, action, resource_type, resource_id,
	                 status, ip_address, user_agent, request_id, detail_json, created_at
	          FROM audit_logs` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	qArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.QueryContext(ctx, query, qArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := make([]AuditLog, 0)
	for rows.Next() {
		var a AuditLog
		if err := rows.Scan(
			&a.ID, &a.ActorType, &a.ActorID, &a.ActorName, &a.Action, &a.ResourceType, &a.ResourceID,
			&a.Status, &a.IPAddress, &a.UserAgent, &a.RequestID, &a.DetailJSON, &a.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		list = append(list, a)
	}
	return list, total, rows.Err()
}

func (s *Store) ExportAllAuditLogs(ctx context.Context, f AuditFilter) ([]AuditLog, error) {
	where, args := buildAuditWhere(f)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, actor_type, actor_id, actor_name, action, resource_type, resource_id,
		        status, ip_address, user_agent, request_id, detail_json, created_at
		 FROM audit_logs`+where+` ORDER BY id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]AuditLog, 0)
	for rows.Next() {
		var a AuditLog
		if err := rows.Scan(
			&a.ID, &a.ActorType, &a.ActorID, &a.ActorName, &a.Action, &a.ResourceType, &a.ResourceID,
			&a.Status, &a.IPAddress, &a.UserAgent, &a.RequestID, &a.DetailJSON, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *Store) DeleteAuditLogsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM audit_logs WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- Cron Jobs ---

func (s *Store) ListCronJobs(ctx context.Context) ([]CronJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, job_key, name, description, cron_expr, enabled, timeout_sec,
		        config_json, last_run_at, last_status, last_duration_ms, last_summary, next_run_at, updated_at
		 FROM cron_jobs ORDER BY job_key ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]CronJob, 0)
	for rows.Next() {
		j, err := scanCronJob(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *j)
	}
	return list, rows.Err()
}

func (s *Store) GetCronJobByID(ctx context.Context, id string) (*CronJob, error) {
	return scanCronJob(s.db.QueryRowContext(ctx,
		`SELECT id, job_key, name, description, cron_expr, enabled, timeout_sec,
		        config_json, last_run_at, last_status, last_duration_ms, last_summary, next_run_at, updated_at
		 FROM cron_jobs WHERE id = ?`, id))
}

func (s *Store) UpdateCronJob(ctx context.Context, id, cronExpr string, enabled bool, timeoutSec int, configJSON string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE cron_jobs SET cron_expr = ?, enabled = ?, timeout_sec = ?, config_json = ?, updated_at = ? WHERE id = ?`,
		cronExpr, boolToInt(enabled), timeoutSec, configJSON, time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateCronJobNextRun(ctx context.Context, id string, nextRun *time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE cron_jobs SET next_run_at = ? WHERE id = ?`, nextRun, id)
	return err
}

func (s *Store) UpdateCronJobExecution(ctx context.Context, id, status, summary string, durationMs int64, lastRun time.Time, nextRun *time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE cron_jobs SET last_run_at = ?, last_status = ?, last_duration_ms = ?, last_summary = ?, next_run_at = ?, updated_at = ? WHERE id = ?`,
		lastRun, status, durationMs, summary, nextRun, time.Now().UTC(), id,
	)
	return err
}

func (s *Store) InsertCronJobRun(ctx context.Context, run *CronJobRun) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cron_job_runs(id, job_id, trigger_type, triggered_by, status, affected_rows, output_log, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.JobID, run.TriggerType, run.TriggeredBy, run.Status, run.AffectedRows, run.OutputLog, run.StartedAt, run.FinishedAt,
	)
	return err
}

func (s *Store) FinishCronJobRun(ctx context.Context, runID, status string, affected int64, outputLog string, finishedAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE cron_job_runs SET status = ?, affected_rows = ?, output_log = ?, finished_at = ? WHERE id = ?`,
		status, affected, outputLog, finishedAt, runID,
	)
	return err
}

func (s *Store) ListCronJobRuns(ctx context.Context, jobID string, limit int) ([]CronJobRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, job_id, trigger_type, triggered_by, status, affected_rows, output_log, started_at, finished_at
		 FROM cron_job_runs WHERE job_id = ? ORDER BY started_at DESC LIMIT ?`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]CronJobRun, 0)
	for rows.Next() {
		var r CronJobRun
		var fin sql.NullTime
		if err := rows.Scan(
			&r.ID, &r.JobID, &r.TriggerType, &r.TriggeredBy, &r.Status,
			&r.AffectedRows, &r.OutputLog, &r.StartedAt, &fin,
		); err != nil {
			return nil, err
		}
		if fin.Valid {
			t := fin.Time
			r.FinishedAt = &t
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// --- Dashboard Summary ---

func (s *Store) GetDashboardSummary(ctx context.Context) (*DashboardSummary, error) {
	sum := &DashboardSummary{}

	_ = s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status = 'succeeded' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0)
		FROM mail_logs`,
	).Scan(&sum.TotalSent, &sum.SucceededCount, &sum.FailedCount)

	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	_ = s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status = 'succeeded' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0)
		FROM mail_logs WHERE created_at >= ?`, todayStart,
	).Scan(&sum.TodaySent, &sum.TodaySucceeded, &sum.TodayFailed)

	_ = s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END), 0)
		FROM clients`,
	).Scan(&sum.TotalClients, &sum.EnabledClients)

	smtpCfg, _ := s.GetSMTPConfig(ctx, nil)
	sum.SMTPConfigured = strings.TrimSpace(smtpCfg.Host) != "" && strings.TrimSpace(smtpCfg.User) != ""
	sum.SMTPHost = smtpCfg.Host
	sum.SMTPPort = smtpCfg.Port

	smtpSt, _ := s.GetSMTPStatus(ctx)
	sum.SMTPTestStatus = smtpSt.Status
	sum.SMTPTestError = smtpSt.Error
	sum.SMTPTestedAt = smtpSt.TestedAt

	// Last 7 days daily series (UTC)
	sevenDaysAgo := todayStart.AddDate(0, 0, -6)
	dayMap := make(map[string]*DailyStat, 7)
	series := make([]DailyStat, 0, 7)
	for i := 0; i < 7; i++ {
		d := sevenDaysAgo.AddDate(0, 0, i).Format("2006-01-02")
		series = append(series, DailyStat{Date: d})
	}
	for i := range series {
		dayMap[series[i].Date] = &series[i]
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT substr(created_at, 1, 10) AS d,
		       COALESCE(SUM(CASE WHEN status = 'succeeded' THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0)
		FROM mail_logs
		WHERE created_at >= ?
		GROUP BY d`, sevenDaysAgo)
	if err == nil {
		for rows.Next() {
			var d string
			var okCnt, failCnt int64
			if rows.Scan(&d, &okCnt, &failCnt) == nil {
				if st, exists := dayMap[d]; exists {
					st.Succeeded = okCnt
					st.Failed = failCnt
				}
			}
		}
		rows.Close()
	}
	sum.DailySeries = series

	recent, _, _ := s.ListMailLogs(ctx, MailLogFilter{Limit: 10})
	sum.RecentMails = recent

	jobs, _ := s.ListCronJobs(ctx)
	sum.CronJobs = jobs

	return sum, nil
}

func scanCronJob(row rowScanner) (*CronJob, error) {
	var j CronJob
	var enabled int
	var lastRun, nextRun sql.NullTime
	err := row.Scan(
		&j.ID, &j.JobKey, &j.Name, &j.Description, &j.CronExpr, &enabled, &j.TimeoutSec,
		&j.ConfigJSON, &lastRun, &j.LastStatus, &j.LastDurationMs, &j.LastSummary, &nextRun, &j.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	j.Enabled = enabled == 1
	if lastRun.Valid {
		t := lastRun.Time
		j.LastRunAt = &t
	}
	if nextRun.Valid {
		t := nextRun.Time
		j.NextRunAt = &t
	}
	return &j, nil
}
