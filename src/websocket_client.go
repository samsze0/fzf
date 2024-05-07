package fzf

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
)

type websocketClient struct {
	apiKey           []byte
	actionChannel    chan []*action
	wsConn           *websocket.Conn
	broadcastChannel chan string
}

func startWebsocketClient(connectAddress string, actionChannel chan []*action, broadcastChannel chan string) (error, *websocketClient) {
	apiKey := os.Getenv("FZF_API_KEY")
	if len(apiKey) == 0 {
		return errors.New("FZF_API_KEY is required to use websocket client"), nil
	}

	dialer := websocket.DefaultDialer

	req, err := http.NewRequest("GET", connectAddress, nil)
	if err != nil {
		log.Println("failed to send HTTP request to websocket server's address:", err)
		return err, nil
	}

	wsConn, res, err := dialer.DialContext(context.Background(), req.URL.String(), req.Header)
	if err != nil {
		log.Println("failed to dial websocket server:", err)
		return err, nil
	}
	// defer wsConn.Close()

	k := res.Header.Get("Fzf-Api-Key")
	log.Println("server provided fzf api key:", k)
	if k != os.Getenv("FZF_API_KEY") {
		log.Println("server did not provide valid fzf api key")
		wsConn.Close()
		return errors.New("server did not provide valid fzf api key"), nil
	}

	client := websocketClient{
		apiKey:           []byte(apiKey),
		actionChannel:    actionChannel,
		wsConn:           wsConn,
		broadcastChannel: broadcastChannel,
	}

	go client.handleBroadcastMessages()

	log.Println("websocket client started listening to", connectAddress)

	go func() {
		for {
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				log.Println("err while reading message from connection:", err, "Exiting...")
				os.Exit(68)
			}
			log.Printf("received message: %s", message)

			actions, err := parseSingleActionList(strings.Trim(string(message), "\r\n"))
			if err != nil {
				client.bad(wsConn, err.Error())
				continue
			}
			if len(actions) == 0 {
				client.bad(wsConn, "no action specified")
				continue
			}
			actionNames := []string{}
			for _, action := range actions {
				actionNames = append(actionNames, action.t.Name())
			}
			log.Printf("received actions: %v", actionNames)

			client.actionChannel <- actions
		}
	}()

	return nil, &client
}

func (client *websocketClient) bad(ws *websocket.Conn, message string) {
	response := fmt.Sprintf("[bad] %s", message)
	log.Println("sending bad response", response)
	err := ws.WriteMessage(websocket.TextMessage, []byte(response))
	if err != nil {
		log.Println("error when sending response:", err)
		ws.Close()
	}
}

func (client *websocketClient) good(ws *websocket.Conn, message string) {
	response := fmt.Sprintf("[good] %s", message)
	log.Println("sending good response", response)
	err := ws.WriteMessage(websocket.TextMessage, []byte(response))
	if err != nil {
		log.Println("error when sending response:", err)
		ws.Close()
	}
}

func (client *websocketClient) handleBroadcastMessages() {
	for {
		msg := <-client.broadcastChannel
		err := client.wsConn.WriteMessage(websocket.TextMessage, []byte(msg))
		if err != nil {
			log.Printf("error: %v", err)
			client.wsConn.Close()
		}
	}
}
