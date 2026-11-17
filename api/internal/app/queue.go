package app

import (
	"context"
	"encoding/json"
	"time"
)

// queueTimeout bounds a single Redis round trip.
const queueTimeout = 3 * time.Second

// job is what the api pushes to Redis and the worker pops back off.
type job struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (application *Application) enqueue(parent context.Context, record itemRecord) error {
	payload, failure := json.Marshal(job{ID: record.ID, Name: record.Name})
	if failure != nil {
		return failure
	}
	pushContext, cancel := context.WithTimeout(parent, queueTimeout)
	defer cancel()
	return application.Queue.LPush(pushContext, queueName, payload).Err()
}
