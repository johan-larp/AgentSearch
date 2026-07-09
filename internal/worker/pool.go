package worker

import (
	"context"
	"log/slog"
	"sync"

	"github.com/johan-larp/agentsearch/internal/models"
)

// Job — единица работы для воркера.
type Job struct {
	Site   models.SiteConfig
	Target string
}

// Processor определяет контракт обработки одной задачи.
type Processor interface {
	Process(ctx context.Context, job Job) models.Result
}

// Pool реализует паттерн Worker Pool с фиксированным числом горутин.
// Канал jobs буферизуется неявно через submitters; канал results
// передается наружу для потребления результатов.
type Pool struct {
	workers   int
	jobs      chan Job
	results   chan models.Result
	processor Processor
}

// NewPool создает пул с заданным числом воркеров.
func NewPool(workers int, processor Processor) *Pool {
	return &Pool{
		workers:   workers,
		jobs:      make(chan Job),
		results:   make(chan models.Result),
		processor: processor,
	}
}

// Start запускает воркеров. Блокирует вызывающую горутину до отмены контекста
// или закрытия канала jobs, после чего закрывает канал results.
func (p *Pool) Start(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < p.workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			slog.Debug("worker started", "id", id)
			for {
				select {
				case job, ok := <-p.jobs:
					if !ok {
						slog.Debug("worker stopped", "id", id, "reason", "jobs_closed")
						return
					}
					res := p.processor.Process(ctx, job)
					select {
					case p.results <- res:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					slog.Debug("worker stopped", "id", id, "reason", "context_cancelled")
					return
				}
			}
		}(i)
	}

	// Когда все воркеры завершились — закрываем results
	go func() {
		wg.Wait()
		close(p.results)
	}()
}

// Submit отправляет задачу в пул. Блокируется, если внутренний канал переполнен
// (в данной реализации канал небуферизован, поэтому backpressure естественный).
func (p *Pool) Submit(job Job) {
	p.jobs <- job
}

// Close закрывает канал jobs, сигнализируя воркерам о завершении.
func (p *Pool) Close() {
	close(p.jobs)
}

// Results возвращает канал для чтения результатов.
func (p *Pool) Results() <-chan models.Result {
	return p.results
}
