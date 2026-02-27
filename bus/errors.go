// 消息总线的错误定义。

package bus

import "errors"

// ErrQueueFull 在消息无法入队时返回（缓冲区已满）。
var ErrQueueFull = errors.New("bus: queue full")
