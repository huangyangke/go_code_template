package async_queue

import "container/heap"

// item 是优先级队列中的元素
// priority 越大越优先（对应 Python 侧 0-9，9 最高）.
type item struct {
	priority int
	msgID    string
	data     map[string]any
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

// EndpointPendingQueue 单个 endpoint 的本地优先级缓冲队列.
type EndpointPendingQueue struct {
	h priorityHeap
}

// Push 添加元素到优先级队列.
// 参数：priority - 优先级（0-9，9最高），msgID - 消息 ID, data - 消息原始数据.
func (q *EndpointPendingQueue) Push(priority int, msgID string, data map[string]any) {
	heap.Push(&q.h, &item{priority: priority, msgID: msgID, data: data})
}

// Peek 查看队头元素但不弹出.
// 返回值：msgID - 消息 ID, data - 消息数据, ok - 队列是否非空.
func (q *EndpointPendingQueue) Peek() (msgID string, data map[string]any, ok bool) {
	if len(q.h) == 0 {
		return "", nil, false
	}
	it := q.h[0]
	return it.msgID, it.data, true
}

// Pop 弹出队头元素.
// 返回值：msgID - 消息 ID, data - 消息数据, ok - 队列是否非空.
func (q *EndpointPendingQueue) Pop() (msgID string, data map[string]any, ok bool) {
	if len(q.h) == 0 {
		return "", nil, false
	}
	it := heap.Pop(&q.h).(*item)
	return it.msgID, it.data, true
}

// Len 返回队列长度.
func (q *EndpointPendingQueue) Len() int { return len(q.h) }

// IsEmpty 判断队列是否为空.
func (q *EndpointPendingQueue) IsEmpty() bool { return len(q.h) == 0 }
