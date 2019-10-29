package event
import "time"
import "encoding/json"
import "errors"
var ErrUnknownVariant = errors.New("event: unknown discriminator tag value")
type EventType = string

const EventTypePageViewed EventType = "Page Viewed"

const EventTypeOrderCompleted EventType = "Order Completed"

const EventTypeHeartbeat EventType = "Heartbeat"

type Event struct {
	Type EventType `json:"type"`
	EventPageViewed
	EventOrderCompleted
	EventHeartbeat
}

func (v Event) MarshalJSON() ([]byte, error) {
	switch v.Type {
	case "Page Viewed":
		return json.Marshal(struct { Tag string `json:"type"`; EventPageViewed }{ Tag: "Page Viewed", EventPageViewed: v.EventPageViewed });
	case "Order Completed":
		return json.Marshal(struct { Tag string `json:"type"`; EventOrderCompleted }{ Tag: "Order Completed", EventOrderCompleted: v.EventOrderCompleted });
	case "Heartbeat":
		return json.Marshal(struct { Tag string `json:"type"`; EventHeartbeat }{ Tag: "Heartbeat", EventHeartbeat: v.EventHeartbeat });
	}
	return nil, ErrUnknownVariant
}
func (v *Event) UnmarshalJSON(b []byte) error {
	var obj map[string]interface{}
	if err := json.Unmarshal(b, &obj); err != nil { return err }
	tag, ok := obj["type"].(string)
	if !ok { return ErrUnknownVariant }
	v.Type = tag
	switch tag {
	case "Page Viewed":
		return json.Unmarshal(b, &v.EventPageViewed)
	case "Order Completed":
		return json.Unmarshal(b, &v.EventOrderCompleted)
	case "Heartbeat":
		return json.Unmarshal(b, &v.EventHeartbeat)
	}
	return ErrUnknownVariant
}
type EventPageViewed struct {
	Timestamp time.Time `json:"timestamp"`
	Url string `json:"url"`
	UserId string `json:"userId"`
}
type EventOrderCompleted struct {
	Timestamp time.Time `json:"timestamp"`
	UserId string `json:"userId"`
	Revenue float64 `json:"revenue"`
}
type EventHeartbeat struct {
	Timestamp time.Time `json:"timestamp"`
	UserId string `json:"userId"`
}

