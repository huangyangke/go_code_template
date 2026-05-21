package async_queue

import "container/heap"

// item 是优先级队列中的元素
// priority 越大越优先（对应 Python 侧 0-9，9 最高）
type item struct {
	priority int
	value    [2]any // [msg_id, data]
	index    int
}

type priorityHeap []*item

func (h priorityHeap) Len() int           { return len(h) }
func (h priorityHeap) Less(i, j int) bool { return h[i].priority > h[j].priority } // 大顶堆
func (h priorityHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *priorityHeap) Push(x any) {
	it := x.(*item)
	it.index = len(*h)
	*h = append(*h, it)
}
func (h *priorityHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return it
}

// EndpointPendingQueue 单个 endpoint 的本地优先级缓冲队列
type EndpointPendingQueue struct {
	h priorityHeap
}

func (q *EndpointPendingQueue) Push(priority int, msgID string, data map[string]any) {
	heap.Push(&q.h, &item{priority: priority, value: [2]any{msgID, data}})
}

// Peek 查看队头，不弹出
func (q *EndpointPendingQueue) Peek() (msgID string, data map[string]any, ok bool) {
	if len(q.h) == 0 {
		return "", nil, false
	}
	it := q.h[0]
	return it.value[0].(string), it.value[1].(map[string]any), true
}

// Pop 弹出队头
func (q *EndpointPendingQueue) Pop() (msgID string, data map[string]any, ok bool) {
	if len(q.h) == 0 {
		return "", nil, false
	}
	it := heap.Pop(&q.h).(*item)
	return it.value[0].(string), it.value[1].(map[string]any), true
}

func (q *EndpointPendingQueue) Len() int    { return len(q.h) }
func (q *EndpointPendingQueue) IsEmpty() bool { return len(q.h) == 0 }
