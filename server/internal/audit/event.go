// Package audit owns operation audit events and persistence.
package audit

import "context"

type Metadata struct {
	ActorID                                                   int64
	RequestID, ClientIP, UserAgent, RequestMethod, RequestURL string
}
type Event struct {
	Action, Resource string
	ResourceID       int64
	// Summary describes the outcome and Params the action's parameters; both
	// are stored beside the event so an audit can answer what changed and why.
	Summary  string
	Params   string
	Metadata Metadata
}
type Recorder interface {
	Record(context.Context, Event) error
}
type noop struct{}

func (noop) Record(context.Context, Event) error { return nil }

func Noop() Recorder { return noop{} }
