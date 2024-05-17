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
	"time"

	"github.com/gorilla/websocket"
)

func init() {
	logFilePath := os.Getenv("FZF_LOG_FILE_PATH")
	if logFilePath == "" {
		logFilePath = "/tmp/fzf.log"
	}
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(logFile)
}

type websocketServer struct {
	apiKey           []byte
	actionChannel    chan []*action
	responseChannel  chan string
	clients          map[*websocket.Conn]string
	broadcastChannel chan string
	relayBuffer      []string
	replay           bool
	replayMutex      sync.Mutex
}

var defaultWebsocketListenAddr = listenAddress{"localhost", 0}

func startWebsocketServer(address listenAddress, actionChannel chan []*action, responseChannel chan string, broadcastChannel chan string) (int, error, *websocketServer) {
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
		responseChannel:  responseChannel,
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
	response := fmt.Sprintf("bad %s", message)
	log.Println("sending bad response to", server.clients[ws], ":", response)
	err := ws.WriteMessage(websocket.TextMessage, []byte(response))
	if err != nil {
		delete(server.clients, ws)
	}
}

func (server *websocketServer) good(ws *websocket.Conn, message string) {
	response := fmt.Sprintf("good %s", message)
	log.Println("sending good response to", server.clients[ws], ":", response)
	err := ws.WriteMessage(websocket.TextMessage, []byte(response))
	if err != nil {
		delete(server.clients, ws)
	}
}

func (server *websocketServer) handleConnections(w http.ResponseWriter, r *http.Request) {
	log.Println("new connection from", r.RemoteAddr, "with Fzf-Api-Key", r.Header.Get("Fzf-Api-Key"))

	if r.Header.Get("Fzf-Api-Key") != os.Getenv("FZF_API_KEY") {
		w.WriteHeader(http.StatusUnauthorized) // 401
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}

	defer ws.Close()

	server.clients[ws] = fmt.Sprintf("Client-%s", r.RemoteAddr)

	// Send all messages in the relay buffer to the new client if replay is enabled
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
		_, message, err := ws.ReadMessage()
		if err != nil {
			delete(server.clients, ws)
			break
		}

		log.Println("received message from", r.RemoteAddr, ":", string(message))

		errorMessage := ""
		actions := parseSingleActionList(strings.Trim(string(message), "\r\n"), func(message string) {
			errorMessage = message
		})
		if len(errorMessage) > 0 {
			server.bad(ws, errorMessage)
			continue
		}
		if len(actions) == 0 {
			server.bad(ws, "no action specified")
			continue
		}
		select {
		case response := <-server.responseChannel:
			server.good(ws, response)
			continue

		case <-time.After(channelTimeout):
			go func() {
				// Drain the channel
				<-server.responseChannel
			}()
			server.bad(ws, "timeout")
			continue
		}
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
