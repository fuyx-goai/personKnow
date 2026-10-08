# Go 语言学习笔记

# Goroutine 的基本概念

Goroutine 是 Go 语言中的轻 Go 运行时量级线程，由（runtime）调度，而不是操作系统。
创建一个 goroutine 只需要在函数调用前加上 go 关键字，例如 `go doWork()`。

与操作系统线程相比，goroutine 的初始栈只有 2KB，非常便宜，
一个进程轻松创建几十万个 goroutine，这就是 Go 天生适合高并发的原因。

# Channel 与协程通信

Go 的哲学是"不要通过共享内存来通信，而要通过通信来共享内存"。
Channel（通道）就是 goroutine 之间传递数据的管道。

无缓冲通道 `ch := make(chan int)` 发送会阻塞，直到有人接收；
带缓冲通道 `make(chan int, 10)` 在缓冲区满之前发送不阻塞。
关闭通道用 `close(ch)`，接收方可以用 `v, ok := <-ch` 判断通道是否已关闭。

# Context 超时控制

context.Context 用于在多个 goroutine 之间传递取消信号、超时和截止时间。
使用 `context.WithTimeout(parent, 3*time.Second)` 可以让下游操作在超时后自动终止，
标准做法是把 ctx 作为函数的第一个参数层层传递下去，并在耗时循环里检查 `ctx.Done()`。

# 今天星期几

今天是2026年10月8日，星期四
