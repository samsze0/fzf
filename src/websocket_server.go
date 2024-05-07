package fzf

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type websocketServer struct {
	apiKey           []byte
	actionChannel    chan []*action
	clients          map[*websocket.Conn]string
	broadcastChannel chan string
	relayBuffer      []string
	replay           bool
	replayMutex      sync.Mutex
}

var defaultWebsocketListenAddr = listenAddress{"localhost", 0}

func startWebsocketServer(address listenAddress, actionChannel chan []*action, broadcastChannel chan string) (int, error, *websocketServer) {
	host := address.host
	port := address.port
	apiKey := os.Getenv("FZF_API_KEY")
	if !address.IsLocal() && len(apiKey) == 0 {
		return port, errors.New("FZF_API_KEY is required to allow remote access"), nil
	}
	addrStr := fmt.Sprintf("%s:%d", host, port)

	listener, err := net.Listen("tcp", addrStr)
	if err != nil {
		return 0, err, nil
	}

	server := websocketServer{
		apiKey:           []byte(apiKey),
		actionChannel:    actionChannel,
		clients:          make(map[*websocket.Conn]string),
		broadcastChannel: broadcastChannel,
		relayBuffer:      make([]string, 0),
		replay:           true,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", server.handleConnections)

	go server.handleBroadcastMessages()

	log.Println("websocket server started on", listener.Addr().(*net.TCPAddr).Port)

	go func() {
		err := http.Serve(listener, mux)
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
		listener.Close()
	}()

	return listener.Addr().(*net.TCPAddr).Port, nil, &server
}

var upgrader = websocket.Upgrader{
	// Allow CORS
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (server *websocketServer) bad(ws *websocket.Conn, message string) {
	response := fmt.Sprintf("[bad] %s", message)
	log.Println("sending bad response to", server.clients[ws], ":", response)
	err := ws.WriteMessage(websocket.TextMessage, []byte(response))
	if err != nil {
		delete(server.clients, ws)
	}
}

func (server *websocketServer) good(ws *websocket.Conn, message string) {
	response := fmt.Sprintf("[good] %s", message)
	log.Println("sending good response to", server.clients[ws], ":", response)
	err := ws.WriteMessage(websocket.TextMessage, []byte(response))
	if err != nil {
		delete(server.clients, ws)
	}
}

func (server *websocketServer) handleConnections(w http.ResponseWriter, r *http.Request) {
	log.Println("new connection from", r.RemoteAddr, "with Fzf-Api-Key", r.Header.Get("Fzf-Api-Key"))

	if r.Header.Get("Fzf-Api-Key") != os.Getenv("FZF_API_KEY") {
		log.Println("unauthorized connection from", r.RemoteAddr)
		w.WriteHeader(http.StatusUnauthorized) // 401
		return
	}

	log.Println("upgrading connection to websocket")

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}

	defer ws.Close()

	server.clients[ws] = fmt.Sprintf("Client-%s", r.RemoteAddr)

	// Send all messages in the relay buffer to the new client if replay is enabled
	log.Println("replaying", len(server.relayBuffer), "messages")
	if server.replay {
		for _, msg := range server.relayBuffer {
			err := ws.WriteMessage(websocket.TextMessage, []byte(msg))
			if err != nil {
				log.Printf("error: %v", err)
				ws.Close()
				delete(server.clients, ws)
				return
			}
		}
	}

	for {
		messageType, message, err := ws.ReadMessage()
		if err != nil {
			delete(server.clients, ws)
			break
		}

		log.Println("received message from", r.RemoteAddr, ":", string(message))

		if messageType == websocket.CloseMessage {
			delete(server.clients, ws)
			break
		}

		if messageType == websocket.BinaryMessage {
			log.Fatal("binary message is not supported")
		}

		if messageType == websocket.PingMessage {
			err := ws.WriteMessage(websocket.PongMessage, nil)
			if err != nil {
				log.Printf("error: %v", err)
				delete(server.clients, ws)
				break
			}
			continue
		}

		if messageType == websocket.PongMessage {
			continue
		}

		if messageType != websocket.TextMessage {
			log.Fatal("unexpected message type", messageType)
		}

		actions, err := parseSingleActionList(strings.Trim(string(message), "\r\n"))
		if err != nil {
			server.bad(ws, err.Error())
			continue
		}
		if len(actions) == 0 {
			server.bad(ws, "no action specified")
			continue
		}
		actionNames := []string{}
		for _, action := range actions {
			actionNames = append(actionNames, action.t.Name())
		}
		log.Printf("received actions: %v", actionNames)

		server.actionChannel <- actions
	}
}

func (server *websocketServer) handleBroadcastMessages() {
	for {
		msg := <-server.broadcastChannel
		server.relayBuffer = append(server.relayBuffer, msg)
		for client := range server.clients {
			err := client.WriteMessage(websocket.TextMessage, []byte(msg))
			if err != nil {
				log.Printf("error: %v", err)
				client.Close()
				delete(server.clients, client)
			}
		}
	}
}
