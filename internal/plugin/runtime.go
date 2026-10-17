package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type State string

const (
	Disabled    State = "disabled"
	Review      State = "review required"
	Starting    State = "starting"
	Ready       State = "ready"
	Busy        State = "running"
	Restarting  State = "restarting"
	Quarantined State = "quarantined"
	Invalid     State = "invalid"
)

type Status struct {
	Entry      Entry
	State      State
	Error, Log string
	Failures   int
	RetryAt    time.Time
}
type callResult struct {
	result Result
	err    error
}
type callJob struct {
	ctx        context.Context
	invocation Invocation
	result     chan callResult
}
type runtime struct {
	entry    Entry
	ctx      context.Context
	cancel   context.CancelFunc
	jobs     chan callJob
	done     chan struct{}
	mu       sync.Mutex
	status   Status
	changed  func()
	failures []time.Time
}

var retryDelays = []time.Duration{500 * time.Millisecond, 2 * time.Second, 5 * time.Second}

func newRuntime(parent context.Context, entry Entry, changed func()) *runtime {
	return newRuntimeWithHealth(parent, entry, changed, 30*time.Second)
}
func newRuntimeWithHealth(parent context.Context, entry Entry, changed func(), healthEvery time.Duration) *runtime {
	ctx, cancel := context.WithCancel(parent)
	r := &runtime{entry: entry, ctx: ctx, cancel: cancel, jobs: make(chan callJob, 16), done: make(chan struct{}), changed: changed, status: Status{Entry: entry, State: Starting}}
	go r.loop(healthEvery)
	return r
}
func (r *runtime) snapshot() Status { r.mu.Lock(); defer r.mu.Unlock(); return r.status }
func (r *runtime) set(state State, err error, p *process, retry time.Time) {
	r.mu.Lock()
	r.status.State = state
	r.status.RetryAt = retry
	r.status.Error = ""
	if err != nil {
		r.status.Error = err.Error()
	}
	if p != nil {
		r.status.Log = p.log.String()
	}
	r.status.Failures = len(r.failures)
	r.mu.Unlock()
	r.changed()
}
func (r *runtime) invoke(ctx context.Context, inv Invocation) (Result, error) {
	state := r.snapshot().State
	if state != Ready && state != Busy {
		return Result{}, fmt.Errorf("plugin is %s", state)
	}
	job := callJob{ctx: ctx, invocation: inv, result: make(chan callResult, 1)}
	select {
	case <-r.ctx.Done():
		return Result{}, errors.New("plugin stopped")
	default:
	}
	select {
	case r.jobs <- job:
	default:
		return Result{}, errors.New("plugin command queue is full")
	}
	select {
	case got := <-job.result:
		return got.result, got.err
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-r.ctx.Done():
		return Result{}, errors.New("plugin stopped")
	}
}
func (r *runtime) fault(err error, p *process) (time.Duration, bool) {
	now := time.Now()
	valid := r.failures[:0]
	for _, t := range r.failures {
		if now.Sub(t) < time.Minute {
			valid = append(valid, t)
		}
	}
	r.failures = append(valid, now)
	if p != nil {
		p.stop()
	}
	if len(r.failures) > len(retryDelays) {
		r.set(Quarantined, err, p, time.Time{})
		return 0, false
	}
	delay := retryDelays[len(r.failures)-1]
	r.set(Restarting, err, p, now.Add(delay))
	return delay, true
}
func (r *runtime) loop(healthEvery time.Duration) {
	defer close(r.done)
	var p *process
	defer func() {
		if p != nil {
			p.stop()
		}
	}()
	retry := time.NewTimer(0)
	defer retry.Stop()
	health := time.NewTicker(healthEvery)
	defer health.Stop()
	scheduleFault := func(err error) {
		delay, again := r.fault(err, p)
		p = nil
		if again {
			retry.Reset(delay)
		}
	}
	for {
		var exited <-chan struct{}
		var faults <-chan error
		if p != nil {
			exited = p.done
			faults = p.readErr
		}
		select {
		case <-r.ctx.Done():
			return
		case <-retry.C:
			r.set(Starting, nil, nil, time.Time{})
			var err error
			p, err = startProcess(r.entry)
			if err == nil {
				ctx, cancel := context.WithTimeout(r.ctx, 3*time.Second)
				var hello struct {
					ProtocolVersion int `json:"protocolVersion"`
				}
				err = p.call(ctx, "initialize", map[string]any{"protocolVersion": ProtocolVersion, "host": "SunBook", "capabilities": []string{"commands", "document.read", "document.edit", "preview.markdown"}}, &hello)
				cancel()
				if err == nil && hello.ProtocolVersion != ProtocolVersion {
					err = errors.New("plugin selected an incompatible protocol")
				}
			}
			if err != nil {
				scheduleFault(err)
				continue
			}
			r.set(Ready, nil, p, time.Time{})
		case err := <-faults:
			scheduleFault(err)
		case <-exited:
			scheduleFault(errors.New("plugin process exited"))
		case job := <-r.jobs:
			if job.ctx.Err() != nil {
				job.result <- callResult{err: job.ctx.Err()}
				continue
			}
			if p == nil {
				job.result <- callResult{err: fmt.Errorf("plugin is %s", r.snapshot().State)}
				continue
			}
			r.set(Busy, nil, p, time.Time{})
			ctx, cancel := context.WithTimeout(job.ctx, 5*time.Second)
			stop := context.AfterFunc(r.ctx, cancel)
			var result Result
			err := p.call(ctx, "execute", job.invocation, &result)
			stop()
			cancel()
			if err == nil && (len(result.Body) > 256*1024 || len(result.Message) > 4096 || len(result.Title) > 200 || len(result.Edits) > 10000) {
				err = errors.New("plugin result exceeds display or edit limits")
			}
			if err == nil && !r.entry.Manifest.Allows(EditDocument) && len(result.Edits) > 0 {
				err = &RPCError{Code: -32602, Message: "plugin lacks document.edit permission"}
			}
			if err == nil && result.Preview != nil {
				preview := result.Preview
				if preview.Kind != "markdown" || len(preview.Text) > 1024*1024 || !utf8.ValidString(preview.Text) || strings.ContainsRune(preview.Text, 0) {
					err = errors.New("invalid Markdown preview; expected UTF-8 text up to 1 MiB without NUL")
				}
			}
			job.result <- callResult{result: result, err: err}
			var applicationError *RPCError
			if err != nil && !errors.As(err, &applicationError) {
				scheduleFault(err)
			} else {
				r.set(Ready, nil, p, time.Time{})
			}
		case <-health.C:
			if p == nil {
				continue
			}
			ctx, cancel := context.WithTimeout(r.ctx, 2*time.Second)
			err := p.ping(ctx)
			cancel()
			if err != nil {
				scheduleFault(err)
			}
		}
	}
}
