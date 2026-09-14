// Package concurrency holds the only allowed bare-goroutine owners: pool, semaphore, supervisor, batcher, singleflight (§7.1). Semaphore and Supervisor exist; pool/batcher/singleflight land with the tasks that need them.
package concurrency
