package object

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// GetEvents lists cluster events, newest first. An empty namespace reads every
// namespace. The limit is applied after sorting, so a caller asking for ten
// gets the ten most recent rather than the ten the API server happened to
// return first.
func GetEvents(cfg *rest.Config, namespace string, eventType string, limit int) ([]corev1.Event, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}

	opts := metav1.ListOptions{}
	if eventType != "" {
		opts.FieldSelector = "type=" + eventType
	}

	list, err := client.CoreV1().Events(namespace).List(context.Background(), opts)
	if err != nil {
		return nil, err
	}

	items := list.Items
	sort.Slice(items, func(i, j int) bool {
		return eventTime(items[i]).Time.After(eventTime(items[j]).Time)
	})

	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// eventTime prefers the timestamp the event was last seen at. Events recorded
// through the newer events API leave the legacy timestamps empty and carry
// EventTime instead.
func eventTime(event corev1.Event) metav1.Time {
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp
	}
	if !event.EventTime.IsZero() {
		return metav1.Time{Time: event.EventTime.Time}
	}
	return event.FirstTimestamp
}

// EventTimestamp exposes the resolved timestamp to callers outside this package.
func EventTimestamp(event corev1.Event) metav1.Time {
	return eventTime(event)
}
