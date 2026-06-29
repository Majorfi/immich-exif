package process

import (
	"sync"
	"sync/atomic"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

type WorkerPool struct {
	client    *api.ImmichClient
	uploader  Uploader
	cfg       *model.Config
	emitter   model.EventEmitter
	workers   int
	cancelled atomic.Bool
}

func NewWorkerPool(client *api.ImmichClient, uploader Uploader, cfg *model.Config, emitter model.EventEmitter) *WorkerPool {
	return &WorkerPool{
		client:   client,
		uploader: uploader,
		cfg:      cfg,
		emitter:  emitter,
		workers:  cfg.Workers,
	}
}

func (wp *WorkerPool) Process(assetIDs []string) []model.ProcessResult {
	total := len(assetIDs)
	jobs := make(chan int)
	results := make([]model.ProcessResult, total)
	var wg sync.WaitGroup
	wp.cancelled.Store(false)

	emitResult := func(index int, result model.ProcessResult) {
		results[index] = result
	}

	for w := 0; w < wp.workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if wp.cancelled.Load() {
					emitResult(idx, model.ProcessResult{AssetID: assetIDs[idx], Status: model.StatusSkipped, Message: "user cancelled"})
					continue
				}
				result := ProcessAsset(wp.client, wp.uploader, wp.cfg, assetIDs[idx], idx+1, total, wp.emitter)
				emitResult(idx, result)
				if result.Cancelled {
					wp.cancelled.Store(true)
				}
			}
		}()
	}

	for i := range assetIDs {
		if wp.cancelled.Load() {
			emitResult(i, model.ProcessResult{AssetID: assetIDs[i], Status: model.StatusSkipped, Message: "user cancelled"})
			continue
		}
		jobs <- i
	}
	close(jobs)

	wg.Wait()
	return results
}

func (wp *WorkerPool) Cancel() {
	wp.cancelled.Store(true)
}
