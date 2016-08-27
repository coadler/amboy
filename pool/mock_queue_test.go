package pool

import (
	"errors"
	"sync"

	"github.com/mongodb/amboy"
	"github.com/tychoish/grip"
	"golang.org/x/net/context"
)

type QueueTester struct {
	started     bool
	isComplete  bool
	pool        amboy.Runner
	numComplete int
	toProcess   chan amboy.Job
	storage     map[string]amboy.Job

	mutex sync.RWMutex
}

func NewQueueTester(p amboy.Runner) *QueueTester {
	q := NewQueueTesterInstance()

	_ = p.SetQueue(q)
	q.pool = p

	return q
}

// Separate constructor for the object so we can avoid the side
// effects of the extra SetQueue for tests where that doesn't make
// sense.
func NewQueueTesterInstance() *QueueTester {
	return &QueueTester{
		toProcess: make(chan amboy.Job, 10),
		storage:   make(map[string]amboy.Job),
	}
}

func (q *QueueTester) Put(j amboy.Job) error {
	q.toProcess <- j
	q.storage[j.ID()] = j
	return nil
}

func (q *QueueTester) Get(name string) (amboy.Job, bool) {
	job, ok := q.storage[name]
	return job, ok
}

func (q *QueueTester) Started() bool {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.started
}

func (q *QueueTester) Complete(ctx context.Context, j amboy.Job) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.numComplete++

	return
}

func (q *QueueTester) Stats() amboy.QueueStats {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return amboy.QueueStats{
		Running:   len(q.storage) - len(q.toProcess),
		Completed: q.numComplete,
		Pending:   len(q.toProcess),
		Total:     len(q.storage),
	}
}

func (q *QueueTester) Runner() amboy.Runner {
	return q.pool
}

func (q *QueueTester) SetRunner(r amboy.Runner) error {
	if q.Started() {
		return errors.New("cannot set runner in a started pool")
	}
	q.pool = r
	return nil
}

func (q *QueueTester) Next(ctx context.Context) amboy.Job {
	select {
	case <-ctx.Done():
		return nil
	case job := <-q.toProcess:
		return job
	}
}

func (q *QueueTester) Start(ctx context.Context) error {
	if q.Started() {
		return nil
	}

	q.pool.Start(ctx)

	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.started = true
	return nil
}

func (q *QueueTester) Results() <-chan amboy.Job {
	output := make(chan amboy.Job)

	go func(m map[string]amboy.Job) {
		for _, job := range m {
			if job.Completed() {
				output <- job
			}
		}
		close(output)
	}(q.storage)

	return output
}

func (q *QueueTester) Wait() {
	if len(q.toProcess) == 0 {
		return
	}

	for {
		grip.Debugf("%+v\n", q.Stats())
		if len(q.storage) == q.numComplete {
			break
		}
	}
}

func (q *QueueTester) Close() {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	close(q.toProcess)
	q.isComplete = true
}
