# JDDF Example: golang-postgres-analytics

This repo is how you can use JDDF in the real world. It's meant to emulate a
Golang server which stores analytics data into a Postgres backend.

Some cool aspects of this example:

- 100% in Golang, and 100% type-safe. If the JDDF validator says the data is
  valid, then it's guaranteed that `json.Unmarshal` will not return any errors.
- Data is validated before being inserted into a generic `jsonb` column in
  Postgres.
- Data is safely deserialized into a Golang struct when read out of a Postgres
  `jsonb` column. We don't have to worry about invalid data, or have to
  manipulate instances of `interface{}`.
- The schema of analytics events is described in
  [`event.jddf.yaml`](./event.jddf.yaml).
- Inputted events are validated against that schema using
  [`jddf-go`](https://github.com/jddf/jddf-go)
- [`jddf-codegen`](https://github.com/jddf/jddf-codegen) generates Golang
  structs for analytics events from the schema.

The code for this example is thoroughly documented, describing some of the
subtle things JDDF does for you. All of the interesting logic is in
[`cmd/golang-postgres-analytics/main.go`](./cmd/golang-postgres-analytics/main.go).

## Highlight: type-safe discriminated unions in Golang!

> Even if you're not using discriminated unions, JDDF can help a lot in Golang.
> Jump straight to the section ["How JDDF helps you
> scale"](#how-jddf-helps-you-scale) for more.

It's exceedingly common for JSON-based messages to use this sort of pattern:

```js
// In this example, all of the events in this list are from the same endpoint or
// queue -- you're supposed to use the "type" field to determine what kind of
// message you're dealing with, and so what kind of fields are available to use.

{ "type": "User Deleted", /* ... fields related to "User Deleted" */ }
{ "type": "User Created", /* ... fields related to "User Created" */ }
{ "type": "Order Completed", /* ... fields related to "Order Completed" */ }
// etc
```

In Golang, representing these kinds of fields are a bit of a pain. The most
common way of dealing with this is just to do:

```go
type MessageType string

var (
  MessageTypeUserDeleted MessageType = "User Deleted"
  MessageTypeUserCreated = "User Created"
  MessageTypeOrderCompleted = "Order Completed"
)

type Message struct {
  Type MessageType
  MessageUserDeleted
  MessageUserCreated
  MessageOrderCompleted
}

// and then define structs for each variant ...

// Not shown: MarshalJSON and UnmarshalJSON implementations. They're
// straightforward, just tedious -- read the "type", and from that decide which
// variant to marshal/unmarshal.
```

### The problem

This approach works quite nicely, and has the advantage of dealing with plain
old Golang structs. But it's error-prone and tiresome to maintain this sort of
struct. This is true of _any_ approach you take for making a discriminated union
in Golang.

If you make a mistake when implementing a discrimianted union written by hand,
the errors you get are weird. You'll run into issues like:

1. Losing data because you updated `UnmarshalJSON` but forgot to update
   `MarshalJSON`, so you successfully read in data but don't write it out
   correctly.
2. Being unable to read back messages you wrote yourself, because you updated
   `MarshalJSON` but not `UnmarshalJSON`.

Other issues can arise. The problem here is you're doing something that's
error-prone and frequently updated in a non-automated way. But if you're system
is using discriminated unions in JSON -- and it's very common to do that,
especially with things like analytics or event sourcing -- then not solving this
problem isn't an option.

The solution, then, is to automate.

### How JDDF solves this

With JDDF, you don't need to write validators or discriminated unions by hand.
Instead, describe your schema in a convenient format like this:

```yaml
discriminator:
  # The "tag" is the field we use to tell what kind of message this is.
  #
  # In our example above, we use the "type" field to do this.
  tag: type

  # The "mapping" is a sort of switch based on the tag's value.
  mapping:
    # When the tag's value is "User Deleted", we expect a field called userId.
    "User Deleted":
      properties:
        userId:
          type: string
    # When the tag's value is "User Created", we expect a field called userId.
    "User Created":
      properties:
        userId:
          type: string
    # When the tag's value is "Order Completed", we expect fields called
    # productId and revenue.
    "Order Completed":
      properties:
        productId:
          type: string
        revenue:
          type: float64
    # ... and if the tag's value is anything other than "User Deleted", "User
    # Created", or "Order Completed", then the message is invalid.
```

That YAML is a declarative version of the relatively obscure Golang code we were
writing by hand before. With this schema, you can:

- Automate your **validation** with [`jddf-go`](https://github.com/jddf/jddf-go)
- Generate your **types** with
  [`jddf-codegen`](https://github.com/jddf/jddf-codegen)

The type generation is as easy as:

```
yaml2json event.jddf.yaml > event.jddf.json
jddf-codegen --go-out=internal/event -- event.jddf.json
```

That'll generate a `internal/event/event.go` like this:

```go
var ErrUnknownVariant = errors.New("event: unknown discriminator tag value")

type EventType = string

const EventTypeOrderCompleted EventType = "Order Completed"

const EventTypeUserCreated EventType = "User Created"

const EventTypeUserDeleted EventType = "User Deleted"

type Event struct {
	Type EventType `json:"type"`
	EventOrderCompleted
	EventUserCreated
	EventUserDeleted
}

func (v Event) MarshalJSON() ([]byte, error) {
	switch v.Type {
	case "Order Completed":
		return json.Marshal(struct {
			Tag string `json:"type"`
			EventOrderCompleted
		}{Tag: "Order Completed", EventOrderCompleted: v.EventOrderCompleted})
	case "User Created":
		return json.Marshal(struct {
			Tag string `json:"type"`
			EventUserCreated
		}{Tag: "User Created", EventUserCreated: v.EventUserCreated})
	case "User Deleted":
		return json.Marshal(struct {
			Tag string `json:"type"`
			EventUserDeleted
		}{Tag: "User Deleted", EventUserDeleted: v.EventUserDeleted})
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
	case "User Created":
		return json.Unmarshal(b, &v.EventUserCreated)
	case "User Deleted":
		return json.Unmarshal(b, &v.EventUserDeleted)
	}
	return ErrUnknownVariant
}

type EventOrderCompleted struct {
	ProductId string  `json:"productId"`
	Revenue   float64 `json:"revenue"`
}
type EventUserCreated struct {
	UserId string `json:"userId"`
}
type EventUserDeleted struct {
	UserId string `json:"userId"`
}
```

It ain't the prettiest, but it's completely automated. Add a field or a whole
new variant, re-run the commands above, and the code automatically reflects your
schema.

You can do validation one of two ways:

1. With `encoding/json` as usual -- the `UnmarshalJSON` and `MarshalJSON`
   methods will ensure that only valid messages are written/read.
2. Using [`jddf-go`](github.com/jddf/jddf-go), the Golang implementation of
   JDDF. With `jddf-go`, you can get consistent error messages, which is often
   crucial when implementing public APIs -- more on that in ["How JDDF helps you
   scale"](#how-jddf-helps-you-scale).

Here's what usage of `jddf-go` looks like. It's quite lightweight:

```go
import "github.com/jddf/jddf-go"

// Omitted here is reading the schema out from a file, or hard-coding it, or
// whatever approach you prefer.
var schema jddf.Schema
json.Unmarshal(YOUR_SCHEMA_AS_BYTES, &schema)

// Construct a jddf validator.
validator := jddf.Validator{}

// If your schema doesn't use "ref", then it's guaranteed that Validate will not
// return an error. Otherwise, an error might be raised if your schema is
// defined cyclically.
validationErrors, _ := validator.Validate(schema, THE_USER_INPUT)

// You can now iterate over a list of validation errors!
for _, validationError := range validationErrors {
  fmt.Println(validationError.InstancePath) // the bad part of the input
  fmt.Println(validationError.SchemaPath) // the part of the schema that raised an error
}
```

For more details on `jddf-go` usage, see its [README](github.com/jddf/jddf-go)
or [GoDoc](https://godoc.org/github.com/jddf/jddf-go).

### How JDDF helps you scale

It's not obvious at first, but there are two common issues when relying on
`encoding/json` or similar packages to create APIs in Golang:

1. You can't re-use your validation logic in non-Golang systems
2. You can't easily change validation backends without breaking clients

Problem (1) is probably obvious. `encoding/json` only works for Golang. If your
JavaScript frontend, or some JVM-based MapReduce system also needed to work with
the same data, they'd have to re-implement their validation logic with a
different approach.

To illustrate problem (2), consider what you return when a client of your API
sends data that is valid JSON, just that it happens to have the wrong shape:

```go
// Let's say this is the shape of our request payload.
type WidgetRequest struct {
  ID string `json:"id"`
}

// And let's say the client sends some data with the wrong type for ID -- a
// number instead of a string.
rawReq := []byte(`{"id": 123}`)

// When we parse the request, we return back to the user the error we get from
// encoding/json.
var req WidgetRequest
if err := json.Unmarshal(rawReq, &req); err != nil {
  return err
}

// ...
//
// See https://play.golang.org/p/YTMgi1xtJe- for what this sort of code does.
```

Here's the problem with that approach. You return errors like this to the user:

```text
json: cannot unmarshal number into Go struct field WidgetRequest.id of type string
```

For internal purposes, that might be fine. But if you're shipping this API to
customers, you're going to want an approach that:

1. Returns _all_ the validation errors with the request. [`encoding/json` only
   returns the first error.](https://play.golang.org/p/qp561bFsLQN).
2. Returns errors in a portable way. Otherwise, you can't rename the fields of
   your Golang structs, because customers will come to depend on parts of your
   error message like `WidgetRequest.id`.

In summary, the `encoding/json` approach works just fine. But down the road, you
might run into issues with inconsistencies between your systems in different
languages, less-than-ideal user experience when using your API, or reduced
velocity when it comes time to refactoring your systems.

**But JDDF solves these problems.** JDDF is a fundamentally portable solution.
This is because:

1. In JDDF, validation errors are standardized. Every implementation returns the
   exact same errors. So there's no lock-in.
2. The `jddf-codegen` tool can generate code in multiple languages from the same
   schema. So the schema can be your cross-language source of truth.

In Golang, we used the [`jddf-go`](github.com/jddf/jddf-go) module to do
validation, and [`jddf-codegen`](github.com/jddf/jddf-codegen) to generate
Golang types. In TypeScript, you can use the
[`@jddf/jddf`](github.com/jddf/jddf-js) package to do the same thing, and
`jddf-codegen` can generate TypeScript interfaces.

And JDDF does so without meaningfully slowing down your velocity. It's a faster
and safer alternative.

## Demo

### Starting the server

Let's start up the server! We'll need a Postgres to talk to, and the included
`docker-compose.yml` has you covered:

```bash
docker-compose up -d
```

We'll need to seed the Postgres with a schema. There's already one set up in
`schema.sql`, let's insert that in:

```bash
cat schema.sql | psql -U postgres -h localhost
```

Next, let's do the code-generation of Golang structs from JDDF schemas. We use
`go generate` to do this:

```bash
go generate ./...
```

(For that command to work, you'll need the `jddf-codegen` tool. On Mac, you can
install that with `brew install jddf/jddf/jddf-codegen`.)

And now we can start the server:

```bash
go run ./cmd/golang-postgres-analytics/main.go
```

### Sending a valid event

Let's first demonstrate the happy case by sending a valid event.

```bash
curl localhost:3000/v1/events \
  -H "Content-Type: application/json" \
  -d '{"type": "Order Completed", "userId": "bob", "timestamp": "2019-09-12T03:45:24+00:00", "revenue": 9.99}'
```

The server echoes back what it inserted into Mongo:

```
{"type":"Order Completed","userId":"bob","timestamp":"2019-09-12T03:45:24+00:00","revenue":9.99,"_id":"5d79cbc30dbb30514f87c1a5"}
```

### Invalid events get consistent validation errors

But what if we sent nonsense data? The answer: the JDDF validator will reject
that data with a standardized error.

```bash
curl localhost:3000/v1/events \
  -H "Content-Type: application/json" \
  -d '{}'
```

The returned status code is 400 (Bad Request), and the error message describes
what part of the input ("instance") and schema didn't play well together:

```json
[{"instancePath":[],"schemaPath":["discriminator","tag"]}
```

This error indicates that the `discriminator.tag` we specified in
`event.jddf.json` was missing from the inputted event.

Here's another example of bad data. What if we used a string instead of a number
for `revenue`, and forgot to include a timestamp?

```bash
curl localhost:3000/v1/events \
  -H "Content-Type: application/json" \
  -d '{"type": "Order Completed", "userId": "bob", "revenue": "100"}' | jq
```

There's now a few problems with the input, so we piped it to `jq` to make it
more human-readable:

```json
[
  {
    "instancePath": [],
    "schemaPath": [
      "discriminator",
      "mapping",
      "Order Completed",
      "properties",
      "timestamp"
    ]
  },
  {
    "instancePath": ["revenue"],
    "schemaPath": [
      "discriminator",
      "mapping",
      "Order Completed",
      "properties",
      "revenue",
      "type"
    ]
  }
]
```

The first error indicates that the instance is missing `timestamp`. The second
error indicates that `revenue` has the wrong type.

### Reading data back out in a type-safe way

Since we're validating the data before putting it into Postgres, we can safely
parse the data into our Golang structs when fetching it back out.

That means we can write some sweet code like this:

```go
func (s *server) getLTV(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	userID := r.URL.Query().Get("userId")

	var dbEvents []dbEvent
	err := s.DB.SelectContext(r.Context(), &dbEvents, `
		select
			payload
		from
			events
		where
			payload->>'type' = 'Order Completed' and
			payload->>'userId' = $1
	`, userID)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	events := make([]event.Event, len(dbEvents))
	for i, dbEvent := range dbEvents {
    // No possibility of errors here.
		json.Unmarshal(dbEvent.Payload, &events[i])
	}

	sum := 0.0
	for _, event := range events {
		sum += event.EventOrderCompleted.Revenue
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%f", sum)
}
```

That is the entire body of logic that lets use calculate the life-time value, or
"LTV", of a user -- basically, the sum of all the purchases they've made with
us. Here's an example:

```bash
# Let's have alice make two purchases -- one for $40, another for $2.
curl localhost:3000/v1/events \
  -H "Content-Type: application/json" \
  -d '{"type": "Order Completed", "userId": "alice", "timestamp": "2019-09-12T03:45:24+00:00", "revenue": 40}'
curl localhost:3000/v1/events \
  -H "Content-Type: application/json" \
  -d '{"type": "Order Completed", "userId": "alice", "timestamp": "2019-09-12T03:45:24+00:00", "revenue": 2}'
```

Here's us calculating Alice's LTV:

```bash
curl localhost:3000/v1/ltv?userId=alice
```

```text
42
```

## Bonus: Automatically generating random events

Oftentimes, it's useful to seed a system like this with some reasonable data,
just test stuff like performance, logging, stats, or other things that require a
bit of volume to test with.

The [`jddf-fuzz`](https://github.com/jddf/jddf-fuzz) tool lets you do exactly
this. Feed `jddf-fuzz` a schema, and it'll generate some random data which
satisfies the schema. For example, here are five randomized analytics events:

```bash
jddf-fuzz -n 5 event.jddf.json
```

```json
{"timestamp":"2005-12-19T06:25:48+00:00","type":"Heartbeat","userId":"4\\"}
{"timestamp":"2015-04-27T23:10:53+00:00","type":"Heartbeat","userId":"Lj"}
{"revenue":0.023312581581551584,"timestamp":"2010-02-10T18:26:48+00:00","type":"Order Completed","userId":"7HJE]G"}
{"timestamp":"1951-09-09T01:18:47+00:00","type":"Page Viewed","url":"F","userId":"RA"}
{"revenue":0.636091000399497,"timestamp":"1919-03-13T10:25:49+00:00","type":"Order Completed","userId":"vh)c"}
```

It ain't beautiful data, but it'll do. Let's insert a thousand of these events
into our server with this command:

```bash
for _ in {0..1000}; do
  jddf-fuzz -n 1 event.jddf.json |
    curl localhost:3000/v1/events -H "Content-Type: application/json" -d @-
done
```

This will hammer the service with events that all will go into Postgres. Pretty
nifty how easy it is to do that!

You can install `jddf-fuzz` on Mac with `brew install jddf/jddf/jddf-fuzz`.
