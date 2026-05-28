package async_queue

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/huangyangke/go-aikit/log"
)

// sendToDeadLetter 将毒消息转移到死信 stream，确保 PEL 中不留残留.
// 优先级：ackAndDel 删除原始消息 > sendToDeadLetter 写入死信记录.
// 两步互不依赖：即使 XADD 失败，原始消息已被 ackAndDel，不会再无限重试.
// 参数：ctx - 上下文（仅提取 namespace，实际 XADD 使用 WithoutCancel 派生 ctx），msg - 毒消息.
func (c *Consumer) sendToDeadLetter(ctx context.Context, msg redis.XMessage) {
	// recovery ctx 可能已超时/取消，死信写入必须脱离原 ctx 否则 XADD 随 ctx 取消而丢失记录
	dlCtx := context.WithoutCancel(ctx)
	streamKey := buildDeadLetterStreamKey(c.namespace)
	// msg.Values 是 Redis 内部 map，直接写入会污染原始数据，后续打印或比对可能看到死信元数据
	values := make(map[string]any, len(msg.Values)+2)
	for k, v := range msg.Values {
		values[k] = v
	}
	values["dead_at"] = time.Now().Unix()
	values["original_msg_id"] = msg.ID

	id, err := c.rdb.XAdd(dlCtx, &redis.XAddArgs{
		Stream: streamKey,
		Values: values,
		MaxLen: 10000,
		Approx: true,
	}).Result()
	if err != nil {
		log.Error("[Consumer][dead_letter][xadd_error][msg_id=%s]: %v", msg.ID, err)
	} else {
		log.Info("[Consumer][dead_letter][transferred][msg_id=%s→%s]", msg.ID, id)
	}
}
