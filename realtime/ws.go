package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Config struct {
	Pid string `json:"pid"`
}

type Hub struct {
	connections map[string][]*websocket.Conn
	collection  *mongo.Collection
	config      Config
}

type Data struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

type changeEvent struct {
	FullDocument Data `bson:"fullDocument"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func mongoCollection(collection string) *mongo.Collection {
	uri := "mongodb://localhost:27017"
	if uriEnv := os.Getenv("MONGO_URI"); uriEnv != "" {
		uri = uriEnv
	}

	mongoClient, err := mongo.NewClient(options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}

	err = mongoClient.Connect(context.TODO())
	if err != nil {
		panic(err)
	}

	return mongoClient.Database(MONGO_DB).Collection(collection)
}

func (h *Hub) reporter() {
	for {
		for pid, conns := range h.connections {
			fmt.Println(pid)
			for _, conn := range conns {
				fmt.Println(conn.RemoteAddr().String())
			}
		}
		time.Sleep(time.Second)
	}
}

func (h *Hub) listen() {
	cs, err := h.collection.Watch(context.TODO(), mongo.Pipeline{}, options.ChangeStream().SetFullDocument(options.UpdateLookup))
	if err != nil {
		panic(err)
	}

	defer cs.Close(context.TODO())

	for cs.Next(context.TODO()) {
		var changeEvent changeEvent
		err := cs.Decode(&changeEvent)
		if err != nil {
			fmt.Println(err)
			continue
		}

		b, err := json.Marshal(changeEvent.FullDocument)
		if err != nil {
			fmt.Println(err)
			continue
		}

		for _, conn := range h.connections[changeEvent.FullDocument.ID] {
			conn.WriteMessage(websocket.TextMessage, b)
		}
	}
}

func (h *Hub) addListener(pid string, conn *websocket.Conn) bool {
	for p := range h.connections {
		if p == pid {
			return true
		}
	}

	res := h.collection.FindOne(context.TODO(), bson.M{"id": pid})

	if res.Err() != nil {
		return false
	}

	var p Data

	if err := res.Decode(&p); err != nil {
		return false
	}

	b, err := json.Marshal(p)
	if err != nil {
		return false
	}

	conn.WriteMessage(websocket.TextMessage, b)
	h.connections[pid] = append(h.connections[pid], conn)

	return true
}

func New() *Hub {
	h := &Hub{
		connections: make(map[string][]*websocket.Conn),
		collection:  mongoCollection(MONGO_COLL),
	}

	go h.listen()
	go h.reporter()

	return h
}

func (h *Hub) Handler(w http.ResponseWriter, r *http.Request) {
	var pid string

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	key := r.Header.Get("Sec-Websocket-Key")

	defer func() {
		conn.Close()

		conns := h.connections[pid]
		for i, c := range conns {
			// TODO: fix this, implement a struct for connections.
			if c == key {
				h.connections[pid] = append(conns[:i], conns[i+1:]...)
				if len(h.connections[pid]) == 0 {
					delete(h.connections, pid)
				}
				break
			}

		}
	}()

	for {
		messageType, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if messageType == websocket.TextMessage {
			pid = string(msg)

			ok := h.addListener(pid, conn)
			if !ok {
				conn.WriteMessage(websocket.TextMessage, []byte("Invalid PID"))
				conn.Close()
				return
			}

			conn.WriteMessage(websocket.TextMessage, []byte("Connected"))
		}
	}

}
