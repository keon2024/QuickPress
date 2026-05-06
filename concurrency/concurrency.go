package concurrency

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"quickpress/config"
	"quickpress/reader"
	"quickpress/requests"
)

type Manager struct {
	mu     sync.Mutex
	runner *Runner
}

type Status struct {
	Running       bool            `json:"running"`
	StartedAt     time.Time       `json:"started_at,omitempty"`
	StoppedAt     time.Time       `json:"stopped_at,omitempty"`
	Elapsed       string          `json:"elapsed"`
	ElapsedMS     int64           `json:"elapsed_ms"`
	Unit          string          `json:"unit"`
	LoopLimit     int             `json:"loop_limit"`
	CurrentTarget int             `json:"current_target"`
	ActiveWorkers int             `json:"active_workers"`
	CurrentStage  int             `json:"current_stage"`
	CurrentLoop   int             `json:"current_loop"`
	DatasetSize   int             `json:"dataset_size"`
	Stages        []config.Stage  `json:"stages"`
	Metrics       MetricsSnapshot `json:"metrics"`
	Message       string          `json:"message,omitempty"`
}

type MetricsSnapshot struct {
	TotalRequests uint64         `json:"total_requests"`
	Success       uint64         `json:"success"`
	Failure       uint64         `json:"failure"`
	AvgLatencyMS  float64        `json:"avg_latency_ms"`
	MaxLatencyMS  float64        `json:"max_latency_ms"`
	AvgRPS        float64        `json:"avg_rps"`
	LastError     string         `json:"last_error,omitempty"`
	StatusCodes   map[int]uint64 `json:"status_codes,omitempty"`
}

type RequestLog struct {
	ID              uint64            `json:"id"`
	OccurredAt      time.Time         `json:"occurred_at"`
	WorkerID        int               `json:"worker_id"`
	Loop            int               `json:"loop"`
	Stage           int               `json:"stage"`
	StepIndex       int               `json:"step_index"`
	RequestName     string            `json:"request_name"`
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	Query           map[string]any    `json:"query,omitempty"`
	RequestHeaders  map[string]string `json:"request_headers,omitempty"`
	RequestBodyType string            `json:"request_body_type,omitempty"`
	RequestBody     string            `json:"request_body,omitempty"`
	ExpectedStatus  int               `json:"expected_status,omitempty"`
	Contains        []string          `json:"contains,omitempty"`
	Extractors      map[string]string `json:"extractors,omitempty"`
	StatusCode      int               `json:"status_code"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	ResponseBody    string            `json:"response_body,omitempty"`
	DurationMS      int64             `json:"duration_ms"`
	Success         bool              `json:"success"`
	Error           string            `json:"error,omitempty"`
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Start(cfg config.Config) error {
	cfg.Normalize()
	if err := cfg.ValidateForRun(); err != nil {
		return err
	}

	dataset, err := reader.Load(reader.Config{Type: cfg.Reader.Type, FilePath: cfg.Reader.File})
	if err != nil {
		return err
	}
	executor := requests.NewExecutor(cfg)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runner != nil && m.runner.IsRunning() {
		return fmt.Errorf("压测已经在运行中")
	}

	runner := newRunner(cfg, dataset, executor)
	m.runner = runner
	runner.Start()
	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	runner := m.runner
	m.mu.Unlock()
	if runner == nil {
		return nil
	}
	runner.Stop()
	return nil
}

func (m *Manager) AdjustTarget(target int) error {
	m.mu.Lock()
	runner := m.runner
	m.mu.Unlock()
	if runner == nil || !runner.IsRunning() {
		return fmt.Errorf("当前没有运行中的压测任务")
	}
	return runner.AdjustTarget(target)
}

func (m *Manager) ReplaceStages(stages []config.Stage) error {
	m.mu.Lock()
	runner := m.runner
	m.mu.Unlock()
	if runner == nil || !runner.IsRunning() {
		return fmt.Errorf("当前没有运行中的压测任务")
	}
	return runner.ReplaceStages(stages)
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	runner := m.runner
	m.mu.Unlock()
	if runner == nil {
		return Status{Message: "尚未启动压测"}
	}
	return runner.Status()
}

func (m *Manager) Results(limit int, failuresOnly bool) []RequestLog {
	m.mu.Lock()
	runner := m.runner
	m.mu.Unlock()
	if runner == nil {
		return []RequestLog{}
	}
	return runner.Results(limit, failuresOnly)
}

type Runner struct {
	cfgMu sync.RWMutex
	cfg   config.Config

	dataset  *reader.Dataset
	executor *requests.Executor
	metrics  *metricsCollector
	logs     *requestLogBuffer

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	workerMu      sync.Mutex
	workers       map[int]context.CancelFunc
	nextWorkerID  int
	activeWorkers atomic.Int64
	currentTarget atomic.Int64
	currentStage  atomic.Int64
	currentLoop   atomic.Int64
	running       atomic.Bool

	stateMu   sync.RWMutex
	startedAt time.Time
	stoppedAt time.Time
}

func newRunner(cfg config.Config, dataset *reader.Dataset, executor *requests.Executor) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Runner{
		cfg:      cfg,
		dataset:  dataset,
		executor: executor,
		metrics:  newMetricsCollector(),
		logs:     newRequestLogBuffer(400),
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
		workers:  map[int]context.CancelFunc{},
	}
}

func (r *Runner) Start() {
	r.stateMu.Lock()
	r.startedAt = time.Now()
	r.stoppedAt = time.Time{}
	r.stateMu.Unlock()
	r.running.Store(true)
	go r.controlLoop()
}

func (r *Runner) Stop() {
	if !r.IsRunning() {
		return
	}
	r.cancel()
	<-r.done
}

func (r *Runner) IsRunning() bool {
	return r.running.Load()
}

func (r *Runner) AdjustTarget(target int) error {
	if target < 0 {
		return fmt.Errorf("target 不能小于 0")
	}
	if !r.IsRunning() {
		return fmt.Errorf("当前没有运行中的压测任务")
	}

	r.cfgMu.Lock()
	defer r.cfgMu.Unlock()

	oldConcurrency := r.cfg.Concurrency
	existing := normalizedStages(oldConcurrency.Stages)
	if len(existing) == 0 {
		existing = []config.Stage{{Label: "运行中调整", Duration: 1, Target: target}}
	}

	now := time.Now()
	elapsed := r.elapsedAt(now)
	cutoff := scheduleCutoffDuration(oldConcurrency, elapsed)
	prefix := stagesUpToDuration(existing, cutoff)
	suffix := stagesAfterDuration(existing, cutoff)
	if len(suffix) == 0 {
		suffix = []config.Stage{{Label: "运行中调整", Duration: 1, Target: target}}
	} else {
		suffix[0].Target = target
	}

	merged := normalizedStages(append(prefix, suffix...))
	newConcurrency := oldConcurrency
	newConcurrency.Stages = merged
	r.cfg.Concurrency.Stages = merged

	remappedElapsed := remapElapsedAfterStageChange(oldConcurrency, newConcurrency, elapsed)
	r.stateMu.Lock()
	if !r.startedAt.IsZero() {
		r.startedAt = now.Add(-remappedElapsed)
	}
	r.stateMu.Unlock()
	r.updateRuntimeState(newConcurrency, remappedElapsed)
	return nil
}

func (r *Runner) ReplaceStages(stages []config.Stage) error {
	stages = normalizedStages(stages)
	if len(stages) == 0 {
		return fmt.Errorf("至少保留一个阶段")
	}
	if !r.IsRunning() {
		return fmt.Errorf("当前没有运行中的压测任务")
	}

	r.cfgMu.Lock()
	defer r.cfgMu.Unlock()

	now := time.Now()
	oldConcurrency := r.cfg.Concurrency
	elapsed := r.elapsedAt(now)
	existing := normalizedStages(oldConcurrency.Stages)
	if len(existing) == 0 {
		existing = []config.Stage{{Label: "运行中阶段", Duration: 1, Target: int(r.currentTarget.Load())}}
	}

	cutoff := completedStageCutoffDuration(oldConcurrency, elapsed)
	merged := mergeStagesAtDuration(existing, stages, cutoff)
	if len(merged) == 0 {
		return fmt.Errorf("至少保留一个阶段")
	}

	newConcurrency := oldConcurrency
	newConcurrency.Stages = merged
	r.cfg.Concurrency.Stages = merged

	remappedElapsed := remapElapsedAfterStageChange(oldConcurrency, newConcurrency, elapsed)
	r.stateMu.Lock()
	if !r.startedAt.IsZero() {
		r.startedAt = now.Add(-remappedElapsed)
	}
	r.stateMu.Unlock()
	r.updateRuntimeState(newConcurrency, remappedElapsed)
	return nil
}

func (r *Runner) Status() Status {
	r.cfgMu.RLock()
	concurrencyCfg := r.cfg.Concurrency
	stages := append([]config.Stage(nil), concurrencyCfg.Stages...)
	r.cfgMu.RUnlock()

	r.stateMu.RLock()
	startedAt := r.startedAt
	stoppedAt := r.stoppedAt
	r.stateMu.RUnlock()

	elapsed := elapsedBetween(startedAt, stoppedAt, time.Now())
	message := "压测运行中"
	if !r.IsRunning() {
		message = "压测已停止"
	}

	return Status{
		Running:       r.IsRunning(),
		StartedAt:     startedAt,
		StoppedAt:     stoppedAt,
		Elapsed:       elapsed.Truncate(time.Second).String(),
		ElapsedMS:     elapsed.Milliseconds(),
		Unit:          concurrencyCfg.Unit,
		LoopLimit:     concurrencyCfg.Loop,
		CurrentTarget: int(r.currentTarget.Load()),
		ActiveWorkers: int(r.activeWorkers.Load()),
		CurrentStage:  int(r.currentStage.Load()) + 1,
		CurrentLoop:   int(r.currentLoop.Load()),
		DatasetSize:   r.dataset.Size(),
		Stages:        stages,
		Metrics:       r.metrics.Snapshot(startedAt, stoppedAt),
		Message:       message,
	}
}

func (r *Runner) controlLoop() {
	defer func() {
		r.reconcileWorkers(0)
		r.stateMu.Lock()
		r.stoppedAt = time.Now()
		r.stateMu.Unlock()
		r.running.Store(false)
		close(r.done)
	}()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		if done := r.applySchedule(); done {
			return
		}
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) applySchedule() bool {
	r.cfgMu.RLock()
	conc := r.cfg.Concurrency
	r.cfgMu.RUnlock()

	r.stateMu.RLock()
	startedAt := r.startedAt
	r.stateMu.RUnlock()
	if startedAt.IsZero() {
		return false
	}

	target, stageIdx, loopIdx, finished := resolveSchedule(conc, time.Since(startedAt))
	if finished {
		return true
	}
	r.currentTarget.Store(int64(target))
	r.currentStage.Store(int64(stageIdx))
	r.currentLoop.Store(int64(loopIdx))
	r.reconcileWorkers(target)
	return false
}

func (r *Runner) reconcileWorkers(target int) {
	if target < 0 {
		target = 0
	}
	r.currentTarget.Store(int64(target))

	r.workerMu.Lock()
	defer r.workerMu.Unlock()
	current := len(r.workers)
	if current == target {
		return
	}

	if current < target {
		for i := 0; i < target-current; i++ {
			r.spawnWorkerLocked()
		}
		return
	}

	extra := current - target
	ids := make([]int, 0, extra)
	for id := range r.workers {
		ids = append(ids, id)
		if len(ids) == extra {
			break
		}
	}
	for _, id := range ids {
		cancel := r.workers[id]
		delete(r.workers, id)
		cancel()
	}
}

func (r *Runner) spawnWorkerLocked() {
	workerCtx, cancel := context.WithCancel(r.ctx)
	id := r.nextWorkerID
	r.nextWorkerID++
	r.workers[id] = cancel
	r.activeWorkers.Add(1)

	go func() {
		defer r.activeWorkers.Add(-1)
		defer func() {
			if recovered := recover(); recovered != nil {
				r.metrics.SetLastError(fmt.Sprintf("worker panic: %v", recovered))
			}
		}()
		for {
			select {
			case <-workerCtx.Done():
				return
			default:
			}
			result := r.executor.Run(r.dataset.Next())
			r.metrics.Observe(result)
			r.logs.Append(r.buildRequestLogs(id, result))
		}
	}()
}

func (r *Runner) Results(limit int, failuresOnly bool) []RequestLog {
	return r.logs.Results(limit, failuresOnly)
}

func (r *Runner) buildRequestLogs(workerID int, result requests.Result) []RequestLog {
	if len(result.Steps) == 0 {
		return nil
	}
	loop := int(r.currentLoop.Load())
	stage := int(r.currentStage.Load()) + 1
	logs := make([]RequestLog, 0, len(result.Steps))
	for index, step := range result.Steps {
		logs = append(logs, RequestLog{
			OccurredAt:      step.StartedAt,
			WorkerID:        workerID,
			Loop:            loop,
			Stage:           stage,
			StepIndex:       index + 1,
			RequestName:     step.RequestName,
			Method:          step.Method,
			URL:             step.URL,
			Query:           cloneAnyMap(step.Query),
			RequestHeaders:  cloneStringMap(step.RequestHeaders),
			RequestBodyType: step.RequestBodyType,
			RequestBody:     truncateDisplayText(step.RequestBody),
			ExpectedStatus:  step.ExpectedStatus,
			Contains:        append([]string(nil), step.Contains...),
			Extractors:      cloneStringMap(step.Extractors),
			StatusCode:      step.StatusCode,
			ResponseHeaders: cloneStringMap(step.ResponseHeaders),
			ResponseBody:    truncateDisplayText(step.ResponseBody),
			DurationMS:      step.Duration.Milliseconds(),
			Success:         step.Success,
			Error:           step.Error,
		})
	}
	return logs
}

func (r *Runner) elapsedAt(now time.Time) time.Duration {
	r.stateMu.RLock()
	startedAt := r.startedAt
	stoppedAt := r.stoppedAt
	r.stateMu.RUnlock()
	return elapsedBetween(startedAt, stoppedAt, now)
}

func (r *Runner) updateRuntimeState(conc config.Concurrency, elapsed time.Duration) {
	target, stageIdx, loopIdx, finished := resolveSchedule(conc, elapsed)
	stages := normalizedStages(conc.Stages)
	if len(stages) == 0 {
		stageIdx = 0
		loopIdx = 1
		target = 0
		finished = true
	} else {
		if stageIdx < 0 {
			stageIdx = 0
		}
		if stageIdx >= len(stages) {
			stageIdx = len(stages) - 1
		}
		if loopIdx <= 0 {
			loopIdx = 1
		}
		if finished {
			target = 0
		}
	}
	r.currentStage.Store(int64(stageIdx))
	r.currentLoop.Store(int64(loopIdx))
	r.currentTarget.Store(int64(target))
	r.reconcileWorkers(target)
}

func elapsedBetween(startedAt, stoppedAt, now time.Time) time.Duration {
	if startedAt.IsZero() {
		return 0
	}
	if stoppedAt.IsZero() {
		return now.Sub(startedAt)
	}
	return stoppedAt.Sub(startedAt)
}

func scheduleCutoffDuration(conc config.Concurrency, elapsed time.Duration) int {
	stages := normalizedStages(conc.Stages)
	if len(stages) == 0 {
		return 0
	}
	cycleDuration := scheduleCycleDuration(conc)
	if cycleDuration <= 0 {
		return 0
	}
	unit := stageUnitDuration(conc.Unit)
	if unit <= 0 {
		unit = time.Second
	}
	pos := elapsed % cycleDuration
	if pos < 0 {
		pos = 0
	}
	cutoff := int(pos / unit)
	if cutoff < 0 {
		return 0
	}
	maxDuration := totalStageDuration(stages)
	if cutoff > maxDuration {
		return maxDuration
	}
	return cutoff
}

func completedStageCutoffDuration(conc config.Concurrency, elapsed time.Duration) int {
	cutoff := scheduleCutoffDuration(conc, elapsed)
	completed := 0
	for _, stage := range normalizedStages(conc.Stages) {
		next := completed + stage.Duration
		if next > cutoff {
			break
		}
		completed = next
	}
	return completed
}

func stagesUpToDuration(stages []config.Stage, cutoff int) []config.Stage {
	normalized := normalizedStages(stages)
	if len(normalized) == 0 || cutoff <= 0 {
		return nil
	}
	result := make([]config.Stage, 0, len(normalized))
	elapsed := 0
	for _, stage := range normalized {
		stageEnd := elapsed + stage.Duration
		if stageEnd <= cutoff {
			result = append(result, stage)
			elapsed = stageEnd
			continue
		}
		if cutoff > elapsed {
			stage.Duration = cutoff - elapsed
			result = append(result, stage)
		}
		break
	}
	return normalizedStages(result)
}

func stagesAfterDuration(stages []config.Stage, cutoff int) []config.Stage {
	normalized := normalizedStages(stages)
	if len(normalized) == 0 {
		return nil
	}
	result := make([]config.Stage, 0, len(normalized))
	elapsed := 0
	for _, stage := range normalized {
		stageEnd := elapsed + stage.Duration
		if stageEnd <= cutoff {
			elapsed = stageEnd
			continue
		}
		if cutoff > elapsed {
			stage.Duration = stageEnd - cutoff
			result = append(result, stage)
		} else {
			result = append(result, stage)
		}
		elapsed = stageEnd
	}
	return normalizedStages(result)
}

func mergeStagesAtDuration(existing, desired []config.Stage, cutoff int) []config.Stage {
	prefix := stagesUpToDuration(existing, cutoff)
	future := stagesAfterDuration(desired, cutoff)
	merged := append([]config.Stage{}, prefix...)
	merged = append(merged, future...)
	return normalizedStages(merged)
}

func remapElapsedAfterStageChange(oldConc, newConc config.Concurrency, elapsed time.Duration) time.Duration {
	if elapsed <= 0 {
		return 0
	}
	oldCycleDuration := scheduleCycleDuration(oldConc)
	newCycleDuration := scheduleCycleDuration(newConc)
	if oldCycleDuration <= 0 || newCycleDuration <= 0 {
		return elapsed
	}
	completedLoops := elapsed / oldCycleDuration
	positionInLoop := elapsed % oldCycleDuration
	return completedLoops*newCycleDuration + positionInLoop
}

func scheduleCycleDuration(conc config.Concurrency) time.Duration {
	stages := normalizedStages(conc.Stages)
	if len(stages) == 0 {
		return 0
	}
	return time.Duration(totalStageDuration(stages)) * stageUnitDuration(conc.Unit)
}

func resolveSchedule(conc config.Concurrency, elapsed time.Duration) (target int, stageIdx int, loopIdx int, finished bool) {
	stages := normalizedStages(conc.Stages)
	if len(stages) == 0 {
		return 1, 0, 1, false
	}
	unit := stageUnitDuration(conc.Unit)
	cycleDuration := time.Duration(totalStageDuration(stages)) * unit
	if cycleDuration <= 0 {
		return stages[len(stages)-1].Target, len(stages) - 1, 1, false
	}

	if conc.Loop > 0 {
		totalDuration := cycleDuration * time.Duration(conc.Loop)
		if elapsed >= totalDuration {
			return 0, len(stages) - 1, conc.Loop, true
		}
	}

	loopIdx = int(elapsed/cycleDuration) + 1
	pos := elapsed % cycleDuration
	stageEnd := time.Duration(0)
	for i, stage := range stages {
		stageEnd += time.Duration(stage.Duration) * unit
		if pos < stageEnd {
			return stage.Target, i, loopIdx, false
		}
	}
	return stages[len(stages)-1].Target, len(stages) - 1, loopIdx, false
}

func normalizedStages(stages []config.Stage) []config.Stage {
	if len(stages) == 0 {
		return nil
	}
	out := append([]config.Stage(nil), stages...)
	for i := range out {
		if out[i].Duration <= 0 {
			out[i].Duration = 1
		}
		if out[i].Target < 0 {
			out[i].Target = 0
		}
	}
	return out
}

func totalStageDuration(stages []config.Stage) int {
	total := 0
	for _, stage := range normalizedStages(stages) {
		total += stage.Duration
	}
	return total
}

func stageUnitDuration(unit string) time.Duration {
	switch unit {
	case config.UnitMinute:
		return time.Minute
	case config.UnitHour:
		return time.Hour
	default:
		return time.Second
	}
}

const failureLogCapacity = 100

type requestLogBuffer struct {
	mu              sync.Mutex
	nextID          uint64
	capacity        int
	failureCapacity int
	entries         []RequestLog
	failures        []RequestLog
}

func newRequestLogBuffer(capacity int) *requestLogBuffer {
	if capacity <= 0 {
		capacity = 200
	}
	return &requestLogBuffer{
		capacity:        capacity,
		failureCapacity: failureLogCapacity,
		entries:         make([]RequestLog, 0, capacity),
		failures:        make([]RequestLog, 0, failureLogCapacity),
	}
}

func (b *requestLogBuffer) Append(items []RequestLog) {
	if len(items) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, item := range items {
		b.nextID++
		item.ID = b.nextID
		if !item.Success {
			failure := cloneRequestLog(item)
			if len(b.failures) == b.failureCapacity {
				copy(b.failures, b.failures[1:])
				b.failures[len(b.failures)-1] = failure
			} else {
				b.failures = append(b.failures, failure)
			}
		}
		if len(b.entries) == b.capacity {
			copy(b.entries, b.entries[1:])
			b.entries[len(b.entries)-1] = item
			continue
		}
		b.entries = append(b.entries, item)
	}
}

func (b *requestLogBuffer) Results(limit int, failuresOnly bool) []RequestLog {
	b.mu.Lock()
	defer b.mu.Unlock()
	source := b.entries
	if failuresOnly {
		source = b.failures
	}
	if len(source) == 0 {
		return []RequestLog{}
	}
	if limit <= 0 || limit > len(source) {
		limit = len(source)
	}
	result := make([]RequestLog, 0, limit)
	for i := len(source) - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, cloneRequestLog(source[i]))
	}
	return result
}

func cloneRequestLog(item RequestLog) RequestLog {
	item.Query = cloneAnyMap(item.Query)
	item.RequestHeaders = cloneStringMap(item.RequestHeaders)
	item.Extractors = cloneStringMap(item.Extractors)
	item.ResponseHeaders = cloneStringMap(item.ResponseHeaders)
	item.Contains = append([]string(nil), item.Contains...)
	return item
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneAnyValue(value)
	}
	return out
}

func cloneAnyValue(input any) any {
	switch value := input.(type) {
	case map[string]any:
		return cloneAnyMap(value)
	case []any:
		out := make([]any, len(value))
		for i := range value {
			out[i] = cloneAnyValue(value[i])
		}
		return out
	default:
		return value
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func truncateDisplayText(input string) string {
	const max = 8192
	if len(input) <= max {
		return input
	}
	return input[:max] + "\n...（已截断）"
}

type metricsCollector struct {
	total        atomic.Uint64
	success      atomic.Uint64
	failure      atomic.Uint64
	totalLatency atomic.Int64
	maxLatency   atomic.Int64

	mu          sync.Mutex
	statusCodes map[int]uint64
	lastError   string
}

func newMetricsCollector() *metricsCollector {
	return &metricsCollector{statusCodes: map[int]uint64{}}
}

func (m *metricsCollector) Observe(result requests.Result) {
	m.total.Add(1)
	if result.Success {
		m.success.Add(1)
	} else {
		m.failure.Add(1)
		if result.Error != "" {
			m.SetLastError(result.Error)
		}
	}
	if result.StatusCode > 0 {
		m.mu.Lock()
		m.statusCodes[result.StatusCode]++
		m.mu.Unlock()
	}
	latency := result.Latency.Nanoseconds()
	m.totalLatency.Add(latency)
	for {
		current := m.maxLatency.Load()
		if latency <= current {
			break
		}
		if m.maxLatency.CompareAndSwap(current, latency) {
			break
		}
	}
}

func (m *metricsCollector) SetLastError(err string) {
	m.mu.Lock()
	m.lastError = err
	m.mu.Unlock()
}

func (m *metricsCollector) Snapshot(startedAt, stoppedAt time.Time) MetricsSnapshot {
	total := m.total.Load()
	success := m.success.Load()
	failure := m.failure.Load()
	avgLatencyMS := 0.0
	if total > 0 {
		avgLatencyMS = float64(m.totalLatency.Load()) / float64(total) / float64(time.Millisecond)
	}
	elapsed := time.Duration(0)
	if !startedAt.IsZero() {
		if stoppedAt.IsZero() {
			elapsed = time.Since(startedAt)
		} else {
			elapsed = stoppedAt.Sub(startedAt)
		}
	}
	avgRPS := 0.0
	if elapsed > 0 {
		avgRPS = float64(total) / elapsed.Seconds()
	}

	m.mu.Lock()
	statusCodes := make(map[int]uint64, len(m.statusCodes))
	for code, count := range m.statusCodes {
		statusCodes[code] = count
	}
	lastError := m.lastError
	m.mu.Unlock()

	return MetricsSnapshot{
		TotalRequests: total,
		Success:       success,
		Failure:       failure,
		AvgLatencyMS:  avgLatencyMS,
		MaxLatencyMS:  float64(m.maxLatency.Load()) / float64(time.Millisecond),
		AvgRPS:        avgRPS,
		LastError:     lastError,
		StatusCodes:   statusCodes,
	}
}
