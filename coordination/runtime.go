package coordination

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Runtime continuously drains policy decisions and their effect outbox. It is
// deliberately small: applications own mode bindings and effect adapters.
type Runtime struct {
	Engine           *Engine
	Effects          *EffectWorker
	Executions       *ExecutionQueue
	ExecutionWorkers []*ExecutionWorker
	Poll             time.Duration

	wake      chan struct{}
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc
}

type RuntimeConfig struct {
	Engine           *Engine
	Effects          *EffectWorker
	Executions       *ExecutionQueue
	ExecutionWorkers []*ExecutionWorker
	Poll             time.Duration
}

func NewRuntime(config RuntimeConfig) (*Runtime, error) {
	if config.Engine == nil || config.Effects == nil {
		return nil, errors.New("coordination: runtime engine and effect worker are required")
	}
	if (config.Executions == nil) != (len(config.ExecutionWorkers) == 0) {
		return nil, errors.New("coordination: execution queue and workers must be configured together")
	}
	runtime := &Runtime{
		Engine: config.Engine, Effects: config.Effects, Executions: config.Executions,
		ExecutionWorkers: append([]*ExecutionWorker(nil), config.ExecutionWorkers...),
		Poll:             config.Poll, wake: make(chan struct{}, 1), done: make(chan struct{}),
	}
	if runtime.Poll <= 0 {
		runtime.Poll = 500 * time.Millisecond
	}
	if runtime.Executions != nil {
		runtime.Executions.Wake = runtime.signal
	}
	return runtime, nil
}

func (r *Runtime) Submit(ctx context.Context, event Event) (bool, error) {
	inserted, err := r.Engine.Submit(ctx, event)
	if err == nil && inserted {
		r.signal()
	}
	return inserted, err
}

func (r *Runtime) Start(parent context.Context) {
	if r == nil {
		return
	}
	r.startOnce.Do(func() {
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithCancel(parent)
		r.cancel = cancel
		go r.run(ctx)
	})
}

func (r *Runtime) run(ctx context.Context) {
	var executionWorkers sync.WaitGroup
	for _, worker := range r.ExecutionWorkers {
		if worker == nil {
			continue
		}
		executionWorkers.Add(1)
		go func(worker *ExecutionWorker) {
			defer executionWorkers.Done()
			runExecutionWorker(ctx, worker, r.Poll, r.wake)
		}(worker)
	}
	defer func() {
		executionWorkers.Wait()
		close(r.done)
	}()
	poll := r.Poll
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		for {
			decision, decisionErr := r.Engine.ProcessNext(ctx)
			effect, effectErr := r.Effects.ProcessNext(ctx)
			if ctx.Err() != nil {
				return
			}
			if decisionErr != nil || effectErr != nil || (!decision && !effect) {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-r.wake:
		}
	}
}

func runExecutionWorker(ctx context.Context, worker *ExecutionWorker, poll time.Duration, wake <-chan struct{}) {
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		for {
			_, claimed, err := worker.RunNext(ctx)
			if ctx.Err() != nil {
				return
			}
			if err != nil || !claimed {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-wake:
		}
	}
}

func (r *Runtime) Close() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		if r.cancel != nil {
			r.cancel()
			<-r.done
		}
	})
}

func (r *Runtime) signal() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}
