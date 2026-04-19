package deploykit

import (
	"context"
	"time"
)

// EventType identifies the kind of event flowing through the EventBus.
type EventType string

const (
	EventServiceCreated       EventType = "service.created"
	EventServiceUpdated       EventType = "service.updated"
	EventServiceDeleted       EventType = "service.deleted"
	EventServiceStatusChanged EventType = "service.status.changed"
	EventContainerCreated     EventType = "container.created"
	EventContainerDeleted     EventType = "container.deleted"
	EventDeploymentCreated    EventType = "deployment.created"
)

// Event is a single message broadcast on the EventBus.
// ProjectID is used by subscribers to filter per-project streams; leave empty
// for global events.
type Event struct {
	Type      EventType
	ProjectID string
	Payload   any
	At        time.Time
}

// EventBus is an in-process publish/subscribe broker.
//
// Publish must be non-blocking — slow subscribers drop messages rather than
// backpressure the publisher.
type EventBus interface {
	Publish(ctx context.Context, evt Event)
	Subscribe(buffer int) Subscription
}

// Subscription is a single subscriber's receive channel. Close to unsubscribe
// and release resources; after Close the channel is closed.
type Subscription interface {
	C() <-chan Event
	Close()
}

// Event payload types.

type ServiceCreatedPayload struct {
	Service *Service `json:"service"`
}

type ServiceUpdatedPayload struct {
	Service *Service `json:"service"`
}

type ServiceDeletedPayload struct {
	ServiceID string `json:"service_id"`
}

type ServiceStatusChangedPayload struct {
	ServiceID string `json:"service_id"`
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
}

type ContainerCreatedPayload struct {
	ServiceID   string `json:"service_id"`
	ContainerID string `json:"container_id"`
	Status      string `json:"status"`
}

type ContainerDeletedPayload struct {
	ServiceID   string `json:"service_id"`
	ContainerID string `json:"container_id"`
}

type DeploymentCreatedPayload struct {
	Deployment *Deployment `json:"deployment"`
}
