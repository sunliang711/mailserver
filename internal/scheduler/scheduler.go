package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"github.com/sunliang711/mailserver/internal/audit"
	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
	"github.com/sunliang711/mailserver/internal/email"
	"github.com/sunliang711/mailserver/internal/migration"
)

var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

type jobMeta struct {
	DefaultSchedule string
	DefaultTimeout  int
}

var builtinJobMeta = map[string]jobMeta{
	"cleanup_audit_logs":   {DefaultSchedule: "0 2 * * *", DefaultTimeout: 300},
	"cleanup_mail_records": {DefaultSchedule: "0 3 * * *", DefaultTimeout: 300},
	"smtp_health_check":    {DefaultSchedule: "*/30 * * * *", DefaultTimeout: 60},
	"migration_backup":     {DefaultSchedule: "0 4 * * *", DefaultTimeout: 300},
}

func DefaultScheduleFor(jobKey string) string {
	if m, ok := builtinJobMeta[jobKey]; ok {
		return m.DefaultSchedule
	}
	return "0 4 * * *"
}

func DefaultTimeoutFor(jobKey string) int {
	if m, ok := builtinJobMeta[jobKey]; ok && m.DefaultTimeout > 0 {
		return m.DefaultTimeout
	}
	return 300
}

// PreviewCron validates a cron expression and returns the next count execution times in local time.
func PreviewCron(expr string, count int) ([]time.Time, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, errors.New("cron expression is required")
	}
	sched, err := cronParser.Parse(expr)
	if err != nil {
		return nil, err
	}
	if count <= 0 {
		count = 5
	}
	out := make([]time.Time, 0, count)
	at := time.Now()
	for i := 0; i < count; i++ {
		at = sched.Next(at)
		if at.IsZero() {
			break
		}
		out = append(out, at)
	}
	return out, nil
}

func ValidateCron(expr string) error {
	_, err := PreviewCron(expr, 1)
	return err
}

func NormalizeJobConfig(jobKey string, incoming json.RawMessage) (string, error) {
	var raw map[string]any
	if len(incoming) > 0 {
		if err := json.Unmarshal(incoming, &raw); err != nil {
			return "", fmt.Errorf("invalid config JSON: %w", err)
		}
	}
	switch jobKey {
	case "cleanup_audit_logs", "cleanup_mail_records":
		days := intFromMap(raw, "retention_days", 90)
		if days < 1 || days > 36500 {
			return "", errors.New("retention_days must be between 1 and 36500")
		}
		b, _ := json.Marshal(map[string]int{"retention_days": days})
		return string(b), nil

	case "smtp_health_check":
		probeTimeout := intFromMap(raw, "probe_timeout_sec", 15)
		if probeTimeout < 1 || probeTimeout > 120 {
			return "", errors.New("probe_timeout_sec must be between 1 and 120")
		}
		b, _ := json.Marshal(map[string]int{"probe_timeout_sec": probeTimeout})
		return string(b), nil

	default:
		if len(incoming) == 0 {
			return "{}", nil
		}
		return string(incoming), nil
	}
}

type Engine struct {
	store      *db.Store
	factory    *email.Factory
	auditLog   *audit.Logger
	logger     zerolog.Logger
	cipher     *crypto.Cipher
	migSvc     *migration.Service
	httpClient *http.Client

	mu       sync.Mutex
	c        *cron.Cron
	entryIDs map[string]cron.EntryID
	running  map[string]bool
	appCtx   context.Context
	cancel   context.CancelFunc
}

func NewEngine(
	store *db.Store,
	factory *email.Factory,
	auditLog *audit.Logger,
	logger zerolog.Logger,
	cipher *crypto.Cipher,
	migSvc *migration.Service,
) *Engine {
	appCtx, cancel := context.WithCancel(context.Background())
	return &Engine{
		store:      store,
		factory:    factory,
		auditLog:   auditLog,
		logger:     logger.With().Str("component", "scheduler").Logger(),
		cipher:     cipher,
		migSvc:     migSvc,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		entryIDs:   make(map[string]cron.EntryID),
		running:    make(map[string]bool),
		appCtx:     appCtx,
		cancel:     cancel,
	}
}

func (e *Engine) Start() error {
	e.mu.Lock()
	e.c = cron.New(cron.WithParser(cronParser))
	e.c.Start()
	e.mu.Unlock()

	return e.ReloadAll(e.appCtx)
}

func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.mu.Lock()
	c := e.c
	e.mu.Unlock()
	if c != nil {
		ctx := c.Stop()
		<-ctx.Done()
	}
}

func (e *Engine) ReloadAll(ctx context.Context) error {
	jobs, err := e.store.ListCronJobs(ctx)
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.c == nil {
		return nil
	}
	for id, entryID := range e.entryIDs {
		e.c.Remove(entryID)
		delete(e.entryIDs, id)
	}

	for _, job := range jobs {
		if !job.Enabled {
			_ = e.store.UpdateCronJobNextRun(ctx, job.ID, nil)
			continue
		}
		j := job
		entryID, err := e.c.AddFunc(j.CronExpr, func() {
			e.executeJob(e.appCtx, j.ID, "schedule", "scheduler")
		})
		if err != nil {
			e.logger.Error().Err(err).Str("job_key", j.JobKey).Str("expr", j.CronExpr).Msg("register cron job failed")
			continue
		}
		e.entryIDs[j.ID] = entryID
		entry := e.c.Entry(entryID)
		if !entry.Next.IsZero() {
			nxt := entry.Next.UTC()
			_ = e.store.UpdateCronJobNextRun(ctx, j.ID, &nxt)
		}
	}
	return nil
}

func (e *Engine) RunNow(_ context.Context, jobID, actorName string) (*db.CronJobRun, error) {
	if _, err := e.store.GetCronJobByID(e.appCtx, jobID); err != nil {
		return nil, err
	}
	run := e.executeJob(e.appCtx, jobID, "manual", actorName)
	return run, nil
}

func (e *Engine) executeJob(ctx context.Context, jobID, triggerType, triggeredBy string) *db.CronJobRun {
	e.mu.Lock()
	if e.running[jobID] {
		e.mu.Unlock()
		now := time.Now().UTC()
		run := &db.CronJobRun{
			ID:          crypto.NewULID(),
			JobID:       jobID,
			TriggerType: triggerType,
			TriggeredBy: triggeredBy,
			Status:      "skipped",
			OutputLog:   "上一次执行尚未结束，跳过本次",
			StartedAt:   now,
			FinishedAt:  &now,
		}
		_ = e.store.InsertCronJobRun(ctx, run)
		return run
	}
	e.running[jobID] = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.running, jobID)
		e.mu.Unlock()
	}()

	job, err := e.store.GetCronJobByID(ctx, jobID)
	if err != nil {
		return &db.CronJobRun{JobID: jobID, Status: "failed", OutputLog: err.Error(), StartedAt: time.Now().UTC()}
	}

	timeoutSec := job.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = DefaultTimeoutFor(job.JobKey)
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	start := time.Now().UTC()
	run := &db.CronJobRun{
		ID:          crypto.NewULID(),
		JobID:       job.ID,
		TriggerType: triggerType,
		TriggeredBy: triggeredBy,
		Status:      "running",
		StartedAt:   start,
	}
	_ = e.store.InsertCronJobRun(ctx, run)

	affected, summary, execErr := e.dispatchJob(runCtx, job)
	fin := time.Now().UTC()
	durationMs := fin.Sub(start).Milliseconds()

	status := "succeeded"
	if execErr != nil {
		status = "failed"
		if errors.Is(execErr, context.DeadlineExceeded) || runCtx.Err() == context.DeadlineExceeded {
			status = "timeout"
		}
		if summary == "" {
			summary = execErr.Error()
		} else {
			summary = summary + "; err: " + execErr.Error()
		}
	}

	run.Status = status
	run.AffectedRows = affected
	run.OutputLog = summary
	run.FinishedAt = &fin
	finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelFinish()
	_ = e.store.FinishCronJobRun(finishCtx, run.ID, status, affected, summary, fin)

	var nextRun *time.Time
	e.mu.Lock()
	if e.c != nil {
		if entryID, ok := e.entryIDs[job.ID]; ok {
			nxt := e.c.Entry(entryID).Next
			if !nxt.IsZero() {
				u := nxt.UTC()
				nextRun = &u
			}
		}
	}
	e.mu.Unlock()

	_ = e.store.UpdateCronJobExecution(finishCtx, job.ID, status, summary, durationMs, start, nextRun)

	if e.auditLog != nil {
		actorType := "system"
		if triggerType == "manual" {
			actorType = "admin"
		}
		e.auditLog.Record(finishCtx, audit.Event{
			ActorType:    actorType,
			ActorID:      triggeredBy,
			ActorName:    triggeredBy,
			Action:       "cron.execute",
			ResourceType: "cron_job",
			ResourceID:   job.ID,
			Status:       map[bool]string{true: "success", false: "failure"}[execErr == nil],
			Detail: map[string]any{
				"job_key":       job.JobKey,
				"trigger_type":  triggerType,
				"affected_rows": affected,
				"summary":       summary,
				"duration_ms":   durationMs,
			},
		})
	}

	return run
}

func (e *Engine) dispatchJob(ctx context.Context, job *db.CronJob) (int64, string, error) {
	var cfg map[string]any
	_ = json.Unmarshal([]byte(job.ConfigJSON), &cfg)

	switch job.JobKey {
	case "cleanup_audit_logs":
		days := intFromMap(cfg, "retention_days", 90)
		if days < 1 {
			days = 1
		}
		cutoff := time.Now().UTC().AddDate(0, 0, -days)
		n, err := e.store.DeleteAuditLogsBefore(ctx, cutoff)
		if err != nil {
			return 0, "", err
		}
		return n, fmt.Sprintf("deleted %d audit logs older than %d days", n, days), nil

	case "cleanup_mail_records":
		days := intFromMap(cfg, "retention_days", 90)
		if days < 1 {
			days = 1
		}
		cutoff := time.Now().UTC().AddDate(0, 0, -days)
		n, err := e.store.DeleteMailLogsBefore(ctx, cutoff)
		if err != nil {
			return 0, "", err
		}
		return n, fmt.Sprintf("deleted %d mail/cron records older than %d days", n, days), nil

	case "smtp_health_check":
		probeTimeoutSec := intFromMap(cfg, "probe_timeout_sec", 15)
		if probeTimeoutSec < 1 || probeTimeoutSec > 120 {
			probeTimeoutSec = 15
		}
		smtpCfg, err := e.store.GetSMTPConfig(ctx, e.cipher)
		if err != nil {
			_ = e.store.UpdateSMTPStatus(ctx, "failed", err.Error())
			return 0, "", err
		}
		if strings.TrimSpace(smtpCfg.Host) == "" {
			_ = e.store.UpdateSMTPStatus(ctx, "untested", "")
			return 0, "SMTP is not configured yet, skipped probe", nil
		}
		tCtx, cancel := context.WithTimeout(ctx, time.Duration(probeTimeoutSec)*time.Second)
		defer cancel()
		if err := e.factory.TestSMTP(tCtx, smtpCfg, time.Duration(probeTimeoutSec)*time.Second); err != nil {
			_ = e.store.UpdateSMTPStatus(ctx, "failed", err.Error())
			return 0, "", err
		}
		_ = e.store.UpdateSMTPStatus(ctx, "ok", "")
		return 1, fmt.Sprintf("SMTP %s:%d connection OK", smtpCfg.Host, smtpCfg.Port), nil

	case "migration_backup":
		return e.runMigrationBackup(ctx, job.ConfigJSON)

	default:
		return 0, "", fmt.Errorf("unknown job_key %q", job.JobKey)
	}
}

func intFromMap(m map[string]any, key string, fallback int) int {
	if m == nil {
		return fallback
	}
	v, ok := m[key]
	if !ok {
		return fallback
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return fallback
	}
}
