/*

Usage:

// Start Websocket Listener
	go webskt.Init()

		// Stream in Kafka Messages
ConsumerLoop:
	for {
		fmt.Println("starting loop")
		select {
		case msg, ok := <-kafkaPartition.Messages():
			if ok {
				// For every new message we have to do a json decode
				json.Unmarshal(msg.Value, &kafkaMessage)
				//fmt.Println("\r\n\r\n", kafkaMessage.NET_SRC, "-", kafkaMessage.NET_DST)
				// We need to decode the sip message
				fromtag, totag, src := sipDecode(kafkaMessage.Payload)

				fmt.Println("Got packet", fromtag, totag, string(kafkaMessage.Payload))
				if ws, isset := webskt.ListenFor[fromtag]; isset {
					var sendMessage = webskt.WebsocketMessage{
						Event:  "listen_data",
						Data:   src,
						Param1: kafkaMessage.ETH_SRC,
						Param2: kafkaMessage.NET_SRC,
						Param3: kafkaMessage.TSP_SRC,
						Param4: kafkaMessage.ETH_DST,
						Param5: kafkaMessage.NET_DST,
						Param6: kafkaMessage.TSP_DST,
					}

					webskt.Send(sendMessage, ws)
				}
				if ws, isset := webskt.ListenFor[totag]; isset {
					var sendMessage = webskt.WebsocketMessage{
						Event:  "listen_data",
						Data:   src,
						Param1: kafkaMessage.ETH_SRC,
						Param2: kafkaMessage.NET_SRC,
						Param3: kafkaMessage.TSP_SRC,
						Param4: kafkaMessage.ETH_DST,
						Param5: kafkaMessage.NET_DST,
						Param6: kafkaMessage.TSP_DST,
					}

					webskt.Send(sendMessage, ws)
				}
			}
		case <-signals:
			break ConsumerLoop
		}
	}

*/

package webskt

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var clients = make(map[*websocket.Conn]bool)     // connected clients
var ListenFor = make(map[string]*websocket.Conn) // the values your are looking for

type Config struct {
	sync.RWMutex
	topic map[string]*struct {
		sync.RWMutex
		ws map[*websocket.Conn]struct{}
	}
}

// Configure the upgrader
var upgrader = websocket.Upgrader{}

// Define our message object
type WebsocketMessage struct {
	Event  string `json:"event"`
	Data   string `json:"data"`
	Param1 string `json:"param1"`
	Param2 string `json:"param2"`
	Param3 string `json:"param3"`
	Param4 string `json:"param4"`
	Param5 string `json:"param5"`
	Param6 string `json:"param6"`
}

// NewConfig New: returns a config file
func NewConfig() (*Config, error) {
	return &Config{
		topic: make(map[string]*struct {
			sync.RWMutex
			ws map[*websocket.Conn]struct{}
		}),
	}, nil
}

// Delete a specific server
func (c *Config) deleteWS(t string, w *websocket.Conn) error {
	thisTopic, ok := c.topic[t]
	if !ok {
		return fmt.Errorf("Listen topic does not exist")
	}
	thisTopic.Lock()
	defer thisTopic.Unlock()
	delete(thisTopic.ws, w)
	if len(thisTopic.ws) == 0 {
		c.Lock()
		defer c.Unlock()
		delete(c.topic, t)
	}
	return nil
}

// get a list of server pointers
func (c *Config) addWS(t string, w *websocket.Conn) error {
	thisTopic, ok := c.topic[t]
	if !ok {
		// Create topic if does not exist
		p := &struct {
			sync.RWMutex
			ws map[*websocket.Conn]struct{}
		}{
			ws: make(map[*websocket.Conn]struct{}),
		}
		c.Lock()
		defer c.Unlock()
		c.topic[t] = p
		return nil
	}
	thisTopic.Lock()
	defer thisTopic.Unlock()
	thisTopic.ws[w] = struct{}{}
	return nil
}

func (c *Config) getWS(t string) (map[*websocket.Conn]struct{}, error) {
	thisTopic, ok := c.topic[t]
	if !ok {
		return nil, fmt.Errorf("Listen topic does not exist")
	}

	r := make(map[*websocket.Conn]struct{})
	thisTopic.RLock()
	defer thisTopic.RUnlock()
	for k, _ := range thisTopic.ws {
		r[k] = struct{}{}
	}
	return r, nil
}

// Init New: starts the service
func (c *Config) Init() error {

	// Create a simple file server
	fs := http.FileServer(http.Dir("public"))
	http.Handle("/", fs)

	// Configure websocket route
	http.HandleFunc("/ws", c.newConnections)

	// Start the server on localhost port 8000 and log any errors
	return fmt.Errorf("ERR, Websocket service, error was: %v", http.ListenAndServe(":80", nil))
}

func (c *Config) newConnections(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("ERR, upgrade websocket connection, error was %v", err)
	}
	// Make sure we close the connection when the function returns
	defer ws.Close()

	for {
		var msg WebsocketMessage
		// Read in a new message as JSON and map it to a Message object
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("error: %v", err)
			break
		}

		// We will take an action depending on the event we just received.
		if msg.Event == "Listen" {
			// Add an identifier to the channel
			c.addWS(msg.Data, ws)
			fmt.Println("Listening for", msg.Data)
		}
		if msg.Event == "StopListen" {
			// Remove an identifier from the channel
			c.deleteWS(msg.Data, ws)
			fmt.Println("Stop Listening for", msg.Data)
		}
	}
}

// Send New: Sends message to all of listener type
func (c *Config) Send(sendStruct WebsocketMessage, t string) error {

	ws, err := c.getWS(t)
	if err != nil {
		return err
	}

	for w, _ := range ws {
		err := w.WriteJSON(sendStruct)
		if err != nil {
			log.Printf("error: %v", err)
			w.Close()
			c.deleteWS(t, w)
		}
	}
	return nil
}

// Init: Old Deprecated functions please migrate
func Init() {

	// Create a simple file server
	fs := http.FileServer(http.Dir("public"))
	http.Handle("/", fs)

	// Configure websocket route
	http.HandleFunc("/ws", newConnections)

	// Start the server on localhost port 8000 and log any errors
	log.Println("http server started on :80")
	err := http.ListenAndServe(":80", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func newConnections(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	// Make sure we close the connection when the function returns
	defer ws.Close()

	// Register our new client
	clients[ws] = true

	for {
		var msg WebsocketMessage
		// Read in a new message as JSON and map it to a Message object
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("error: %v", err)
			delete(clients, ws)
			break
		}

		// We will take an action depending on the event we just received.
		if msg.Event == "Listen" {
			// Add an identifier to the channel
			ListenFor[msg.Data] = ws
			fmt.Println("Listening for", msg.Data)
		}
		if msg.Event == "StopListen" {
			// Remove an identifier from the channel
			delete(ListenFor, msg.Data)
			fmt.Println("Stop Listening for", msg.Data)
		}
	}
}

// Send: Old depreciated function, please migrate
func Send(sendStruct WebsocketMessage, ws *websocket.Conn) {

	err := ws.WriteJSON(sendStruct)
	if err != nil {
		log.Printf("error: %v", err)
		ws.Close()
		delete(clients, ws)
	}

}
