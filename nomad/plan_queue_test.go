package nomad

import (
	"testing"
	"time"

	"github.com/hashicorp/nomad/nomad/structs"
)

func testPlanQueue(t *testing.T) *PlanQueue {
	pq, err := NewPlanQueue()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	return pq
}

func mockPlan() *structs.Plan {
	return &structs.Plan{
		Priority:        50,
		EvalCreateIndex: 1000,
	}
}

func mockPlanResult() *structs.PlanResult {
	return &structs.PlanResult{}
}

func TestPlanQueue_Enqueue_Dequeue(t *testing.T) {
	pq := testPlanQueue(t)
	pq.SetEnabled(true)

	plan := mockPlan()
	future, err := pq.Enqueue(plan)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	stats := pq.Stats()
	if stats.Depth != 1 {
		t.Fatalf("bad: %#v", stats)
	}

	resCh := make(chan *structs.PlanResult, 1)
	go func() {
		res, err := future.Wait()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		resCh <- res
	}()

	pending, err := pq.Dequeue(time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	stats = pq.Stats()
	if stats.Depth != 0 {
		t.Fatalf("bad: %#v", stats)
	}

	if pending == nil || pending.plan != plan {
		t.Fatalf("bad: %#v", pending)
	}

	result := mockPlanResult()
	pending.respond(result, nil)

	select {
	case r := <-resCh:
		if r != result {
			t.Fatalf("Bad: %#v", r)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout")
	}
}

func TestPlanQueue_Enqueue_Disable(t *testing.T) {
	pq := testPlanQueue(t)

	// Enqueue
	plan := mockPlan()
	pq.SetEnabled(true)
	future, err := pq.Enqueue(plan)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Flush via SetEnabled
	pq.SetEnabled(false)

	// Check the stats
	stats := pq.Stats()
	if stats.Depth != 0 {
		t.Fatalf("bad: %#v", stats)
	}

	// Future should be canceled
	res, err := future.Wait()
	if err != planQueueFlushed {
		t.Fatalf("err: %v", err)
	}
	if res != nil {
		t.Fatalf("bad: %#v", res)
	}
}

func TestPlanQueue_Dequeue_Timeout(t *testing.T) {
	pq := testPlanQueue(t)
	pq.SetEnabled(true)

	start := time.Now()
	out, err := pq.Dequeue(5 * time.Millisecond)
	end := time.Now()

	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != nil {
		t.Fatalf("unexpected: %#v", out)
	}

	if diff := end.Sub(start); diff < 5*time.Millisecond {
		t.Fatalf("bad: %#v", diff)
	}
}

// Ensure higher priority dequeued first
func TestPlanQueue_Dequeue_Priority(t *testing.T) {
	pq := testPlanQueue(t)
	pq.SetEnabled(true)

	plan1 := mockPlan()
	plan1.Priority = 10
	pq.Enqueue(plan1)

	plan2 := mockPlan()
	plan2.Priority = 30
	pq.Enqueue(plan2)

	plan3 := mockPlan()
	plan3.Priority = 20
	pq.Enqueue(plan3)

	out1, _ := pq.Dequeue(time.Second)
	if out1.plan != plan2 {
		t.Fatalf("bad: %#v", out1)
	}

	out2, _ := pq.Dequeue(time.Second)
	if out2.plan != plan3 {
		t.Fatalf("bad: %#v", out2)
	}

	out3, _ := pq.Dequeue(time.Second)
	if out3.plan != plan1 {
		t.Fatalf("bad: %#v", out3)
	}
}

// Ensure FIFO at fixed priority
func TestPlanQueue_Dequeue_FIFO(t *testing.T) {
	pq := testPlanQueue(t)
	pq.SetEnabled(true)
	NUM := 100

	for i := 0; i < NUM; i++ {
		plan := mockPlan()
		plan.EvalCreateIndex = uint64(i)
		pq.Enqueue(plan)
	}

	for i := 0; i < NUM; i++ {
		out1, _ := pq.Dequeue(time.Second)
		if out1.plan.EvalCreateIndex != uint64(i) {
			t.Fatalf("bad: %d %#v", i, out1)
		}
	}
}