package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/jddf-examples/golang-mongo-analytics/event"
	"github.com/jddf/jddf-go"
	"github.com/julienschmidt/httprouter"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

//go:generate node_modules/.bin/yaml2json --save event.jddf.yaml
//go:generate jddf-codegen --go-out event -- event.jddf.json

func main() {
	server, err := newServer(context.Background())
	if err != nil {
		panic(err)
	}

	router := httprouter.New()
	router.POST("/v1/events", server.createEvent)
	router.GET("/v1/ltv", server.getLTV)

	if err := http.ListenAndServe(":3000", router); err != nil {
		panic(err)
	}
}

type server struct {
	EventSchema jddf.Schema
	Mongo       *mongo.Client
}

func newServer(ctx context.Context) (server, error) {
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		return server{}, err
	}

	eventSchemaFile, err := os.Open("event.jddf.json")
	if err != nil {
		return server{}, err
	}

	defer eventSchemaFile.Close()

	var eventSchema jddf.Schema
	schemaDecoder := json.NewDecoder(eventSchemaFile)
	if err := schemaDecoder.Decode(&eventSchema); err != nil {
		return server{}, err
	}

	return server{
		EventSchema: eventSchema,
		Mongo:       mongoClient,
	}, nil
}

func (s *server) createEvent(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	defer r.Body.Close()

	// Read the body out into a buffer.
	buf, err := ioutil.ReadAll(r.Body)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	// Read the body as generic JSON, and run JDDF validation.
	var eventRaw interface{}
	if err := json.Unmarshal(buf, &eventRaw); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	validator := jddf.Validator{}
	validationResult, err := validator.Validate(s.EventSchema, eventRaw)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	if len(validationResult.Errors) != 0 {
		errorsOut, err := json.Marshal(validationResult.Errors)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "%s", err)
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", errorsOut)
		return
	}

	// The body is now known to be safe according to the schema. We can now write
	// it into Mongo.
	_, err = s.Mongo.Database("example").Collection("events").InsertOne(r.Context(), eventRaw)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}
}

func (s *server) getLTV(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	userID := r.URL.Query().Get("userId")

	cursor, err := s.Mongo.Database("example").Collection("events").Find(ctx, map[string]string{}{
		"type": event.EventTypeOrderCompleted,
		"userId": userID,
	})

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	cursor.get
}
