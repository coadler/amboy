package queue

import (
	"context"
	"math/rand"

	"github.com/deciduosity/amboy"
	"github.com/deciduosity/amboy/dependency"
)

type adaptiveOrderItems struct {
	jobs      map[string]amboy.Job
	ready     []string
	waiting   []string
	stalled   []string
	completed []string
	passed    []string
}

func (items *adaptiveOrderItems) add(j amboy.Job) error {
	id := j.ID()
	if _, ok := items.jobs[id]; ok {
		return amboy.NewDuplicateJobErrorf("cannot add duplicate job with id '%s'", id)
	}

	items.jobs[id] = j

	if j.Status().Completed {
		items.completed = append(items.completed, id)
		return nil
	}
	ti := j.TimeInfo()
	if !ti.IsDispatchable() {
		items.waiting = append(items.waiting, id)
		return nil
	}

	if ti.IsStale() {
		items.stalled = append(items.stalled, id)
		return nil
	}

	switch j.Dependency().State() {
	case dependency.Ready:
		items.ready = append(items.ready, id)
	case dependency.Blocked:
		items.waiting = append(items.waiting, id)
	case dependency.Unresolved:
		items.stalled = append(items.stalled, id)
	case dependency.Passed:
		items.passed = append(items.passed, id)
	}

	return nil
}

func (items *adaptiveOrderItems) remove(id string) {
	new := make([]string, 0, cap(items.completed))
	for _, jid := range items.completed {
		if id != jid {
			new = append(new, jid)
			continue
		}

		delete(items.jobs, id)
	}

	items.completed = new
}

func (items *adaptiveOrderItems) updateCompleted(ctx context.Context) {
	new := make([]string, 0, len(items.completed))

	for _, id := range items.completed {
		if ctx.Err() != nil {
			return
		}

		if _, ok := items.jobs[id]; !ok {
			continue
		}

		new = append(new, id)
	}
	items.completed = new
}

func (items *adaptiveOrderItems) updatePassed(ctx context.Context) {
	new := make([]string, 0, len(items.passed))

	for _, id := range items.passed {
		if ctx.Err() != nil {
			return
		}

		if _, ok := items.jobs[id]; !ok {
			continue
		}

		new = append(new, id)
	}
	items.passed = new
}

func (items *adaptiveOrderItems) updateWaiting(ctx context.Context) {
	new := []string{}

	for _, id := range items.waiting {
		if ctx.Err() != nil {
			return
		}

		job, ok := items.jobs[id]
		if !ok {
			continue
		}
		ti := job.TimeInfo()

		// check DispatchBy
		if ti.IsStale() {
			items.stalled = append(items.stalled, id)
			continue
		}

		// check WaitUntil
		if !ti.IsDispatchable() {
			new = append(new, id)
			continue
		}

		status := job.Status()
		if status.Completed || status.InProgress {
			items.completed = append(items.completed, id)
			continue
		}

		state := job.Dependency().State()
		if state == dependency.Ready {
			items.ready = append(items.ready, id)
			continue
		}

		if state == dependency.Blocked {
			new = append(new, id)
			continue
		}
		if state == dependency.Unresolved {
			items.stalled = append(items.stalled, id)
			continue
		}
	}
	items.waiting = new
}

func (items *adaptiveOrderItems) updateStalled(ctx context.Context) {
	new := []string{}
	for _, id := range items.stalled {
		if ctx.Err() != nil {
			return
		}

		job, ok := items.jobs[id]
		if !ok {
			continue
		}

		if job.TimeInfo().IsStale() {
			new = append(new, id)
			continue
		}

		status := job.Status()
		if status.Completed || status.InProgress {
			items.completed = append(items.completed, id)
			continue
		}

		state := job.Dependency().State()
		if state == dependency.Ready {
			items.ready = append(items.ready, id)
			continue
		}

		if state == dependency.Blocked {
			items.waiting = append(items.waiting, id)
			continue
		}

		if state == dependency.Unresolved {
			new = append(new, id)
			continue
		}
	}

	items.stalled = new
}

func (items *adaptiveOrderItems) updateReady(ctx context.Context) {
	// shuffle the order of the ready queue.
	//   in the future this might be good to sort based on the
	//   number of edges, and randomized otherwise.
	if len(items.ready) > 1 {
		new := make([]string, len(items.ready))
		for i, r := range rand.Perm(len(items.ready)) {
			if ctx.Err() != nil {
				return
			}

			if _, ok := items.jobs[items.ready[r]]; !ok {
				continue
			}

			new[i] = items.ready[r]
		}
		items.ready = new
	}

}

func (items *adaptiveOrderItems) refilter(ctx context.Context) {
	items.updateWaiting(ctx)
	items.updateStalled(ctx)
	items.updateReady(ctx)
}

func (items *adaptiveOrderItems) delete(ctx context.Context, id string) int {
	if _, ok := items.jobs[id]; !ok {
		return 0
	}

	delete(items.jobs, id)

	items.updateCompleted(ctx)
	items.updatePassed(ctx)
	items.updateWaiting(ctx)
	items.updateStalled(ctx)
	items.updateReady(ctx)

	return 1
}
