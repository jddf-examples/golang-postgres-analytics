package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/jddf-examples/golang-postgres-analytics/internal/event"
	"github.com/jddf/jddf-go"
	"github.com/jmoiron/sqlx"
	"github.com/julienschmidt/httprouter"
	_ "github.com/lib/pq"
)

// The comments below are meant to be used with the "go generate" command.
//
// For more on this, see: https://blog.golang.org/generate
//
//go:generate ../../node_modules/.bin/yaml2json --save ../../event.jddf.yaml
//go:generate jddf-codegen --go-out=../../internal/event -- ../../event.jddf.json

// main is the entrypoint of the server.
func main() {
	// Construct a new "server"; its methods are HTTP endpoints.
	server, err := newServer()
	if err != nil {
		panic(err)
	}

	// Construct a router which binds URLs + HTTP verbs to methods of server.
	router := httprouter.New()
	router.POST("/v1/events", server.createEvent)
	router.GET("/v1/ltv", server.getLTV)

	// Listen and serve HTTP traffic on port 3000.
	if err := http.ListenAndServe(":3000", router); err != nil {
		panic(err)
	}
}

// server holds together all the things we need to run an analytics-event
// server.
type server struct {
	EventSchema jddf.Schema
	DB          *sqlx.DB
}

// newServer constructs a new instance of a server using hard-coded defaults.
func newServer() (server, error) {
	// Connect to postgresql.
	db, err := sqlx.Open("postgres", "postgres://postgres@localhost?sslmode=disable")
	if err != nil {
		return server{}, err
	}

	// Load a schema from disk. JDDF and jddf-go are agnostic to how you load your
	// schemas; ultimately, you could hard-code them, pass them in from
	// environment variables, download them from the network, or whatever other
	// approach best meets your requirements.
	eventSchemaFile, err := os.Open("event.jddf.json")
	if err != nil {
		return server{}, err
	}

	defer eventSchemaFile.Close()

	// Here, we parse a jddf.Schema from the JSON inside the "event.jddf.json"
	// file.
	//
	// You can, if you prefer, also hard-code schemas using native Golang syntax.
	// The README of jddf-go shows you how:
	//
	// https://github.com/jddf/jddf-go
	var eventSchema jddf.Schema
	schemaDecoder := json.NewDecoder(eventSchemaFile)
	if err := schemaDecoder.Decode(&eventSchema); err != nil {
		return server{}, err
	}

	// Return the server with everything it needs. The main function will handle
	// serving HTTP traffic using this server.
	return server{
		EventSchema: eventSchema,
		DB:          db,
	}, nil
}

// dbEvent is how we represent a single event in this API in the database layer.
//
// The `db` tag on this struct's fields is a convenience offered by the
// github.com/jmoiron/sqlx package, and simply provides a lightweight
// implementation the standard library's sql.Scanner interface.
type dbEvent struct {
	Payload []byte `db:"payload"`
}

// createEvent reads in an analytics event, persists it, and returns a
// representation of that stored event. It's bound POST /v1/events.
func (s *server) createEvent(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	defer r.Body.Close()

	// Read the body out into a buffer.
	buf, err := ioutil.ReadAll(r.Body)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	// Read the body as generic JSON, so we can perform JDDF validation on it.
	//
	// If the request body is invalid JSON, send the user a 400 Bad Request.
	var eventRaw interface{}
	if err := json.Unmarshal(buf, &eventRaw); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	// Validate the event (in eventRaw) against our schema for JDDF events.
	//
	// In practice, there will never be errors arising here -- see the jddf-go
	// docs for details, but basically jddf.Validator.Validate can only error if
	// you use "ref" in a cyclic manner in your schemas.
	//
	// Therefore, we ignore the possibility of an error here.
	validator := jddf.Validator{}
	validationResult, _ := validator.Validate(s.EventSchema, eventRaw)

	// If there were validation errors, then we write them out to the response
	// body, and send the user a 400 Bad Request.
	if len(validationResult.Errors) != 0 {
		encoder := json.NewEncoder(w)
		if err := encoder.Encode(validationResult.Errors); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "%s", err)
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// If we made it here, the request body contained JSON that passed our schema.
	// Let's now write it into the database.
	//
	// The events table has a "payload" column of type "jsonb". In Golang-land,
	// you can send that to Postgres by just using []byte. The user's request
	// payload is already in that format, so we'll use that.
	_, err = s.DB.ExecContext(r.Context(), `
		insert into events (payload) values ($1)
	`, buf)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	// We're done!
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s", buf)
}

// This is the endpoint for getting the lifetime value ("LTV", in marketing
// parlance) of a user ID. It's just the sum of all the revenue from a user.
//
// This lives at GET /v1/ltv?userId=XXX
func (s *server) getLTV(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	// Get a user ID from the query parameters.
	userID := r.URL.Query().Get("userId")

	// Get all events, in raw format, from the database.
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

	// Convert the raw database events into friendlier Golang events.
	//
	// Thankfully for us, JDDF makes this super easy to do. The auto-generated
	// types for an event already have appropriate "json" tags and
	// MarhshalJSON/UnmarshalJSON implementations.
	events := make([]event.Event, len(dbEvents))
	for i, dbEvent := range dbEvents {
		// The only way this json.Unmarshal operation can fail is if someone other
		// than this service inserted data into the database, and they didn't follow
		// the JDDF schema we use.
		//
		// Thanks to JDDF, it's guaranteed that if everyone who uses this database
		// uses the same JDDF schema, then parsing out raw Postgres jsonb data into
		// our Golang structs is a safe and error-proof operation.
		if err := json.Unmarshal(dbEvent.Payload, &events[i]); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "%s", err)
			return
		}
	}

	// Now that we have our raw jsonb data parsed into something conveninent for
	// Golang manipulation, let's sum over the revenue of all the returned events.
	sum := 0.0
	for _, event := range events {
		// We happen to know, from how we wrote our SQL, that all of these events
		// are of the "Order Completed" type. But if you're feeling cautious, you
		// could do an assertion to ensure the event.Type is always
		// EventTypeOrderCompleted.
		sum += event.EventOrderCompleted.Revenue
	}

	// Send back the calculated sum to the user.
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%f", sum)
}
