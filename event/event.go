package event

import "time"
import "encoding/json"
import "errors"

var ErrUnknownVariant = errors.New("event: unknown discriminator tag value")

type EventType = string

const EventTypeOrderCompleted EventType = "Order Completed"

const EventTypeHeartbeat EventType = "Heartbeat"

const EventTypePageViewed EventType = "Page Viewed"

type Event struct {
	Type EventType `json:"type"`
	EventOrderCompleted
	EventHeartbeat
	EventPageViewed
}

func (v Event) MarshalJSON() ([]byte, error) {
	switch v.Type {
	case "Order Completed":
		return json.Marshal(struct {
			Tag string `json:"type"`
			EventOrderCompleted
		}{Tag: "Order Completed", EventOrderCompleted: v.EventOrderCompleted})
	case "Heartbeat":
		return json.Marshal(struct {
			Tag string `json:"type"`
			EventHeartbeat
		}{Tag: "Heartbeat", EventHeartbeat: v.EventHeartbeat})
	case "Page Viewed":
		return json.Marshal(struct {
			Tag string `json:"type"`
			EventPageViewed
		}{Tag: "Page Viewed", EventPageViewed: v.EventPageViewed})
	}
	return nil, ErrUnknownVariant
}
func (v *Event) UnmarshalJSON(b []byte) error {
	var obj map[string]interface{}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	tag, ok := obj["type"].(string)
	if !ok {
		return ErrUnknownVariant
	}
	v.Type = tag
	switch tag {
	case "Order Completed":
		return json.Unmarshal(b, &v.EventOrderCompleted)
	case "Heartbeat":
		return json.Unmarshal(b, &v.EventHeartbeat)
	case "Page Viewed":
		return json.Unmarshal(b, &v.EventPageViewed)
	}
	return ErrUnknownVariant
}

type EventOrderCompleted struct {
	UserId    string    `json:"userId"`
	Timestamp time.Time `json:"timestamp"`
	Revenue   float64   `json:"revenue"`
}
type EventHeartbeat struct {
	UserId    string    `json:"userId"`
	Timestamp time.Time `json:"timestamp"`
}
type EventPageViewed struct {
	UserId    string    `json:"userId"`
	Url       string    `json:"url"`
	Timestamp time.Time `json:"timestamp"`
}
