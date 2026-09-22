package messaging

import (
	"errors"
	"net/http"
	"ride-sharing/shared/contracts"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	ErrConnectionNotFound = errors.New("connection not found")
)

// connWrapper e um wrapper em volta da conecao de websocket que permite operacoes thread-safe
type connWrapper struct {
	conn  *websocket.Conn
	mutex sync.Mutex
}

type ConnectionManager struct {
	connections map[string]*connWrapper //conexoes locais (userId -> connection)
	mutex       sync.Mutex
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true //permite conexao de todas as origens
	},
}

// NewConnectionManager cria um gerenciador de conexoes WebSocket.
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[string]*connWrapper),
	}
}

// Upgrader transforma uma requisicao HTTP em uma conexao WebSocket.
func (cm *ConnectionManager) Upgrader(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// Add adiciona uma conexao WebSocket ao gerenciador.
func (cm *ConnectionManager) Add(id string, conn *websocket.Conn) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.connections[id] = &connWrapper{conn: conn, mutex: sync.Mutex{}}
}

// Remove remove uma conexao WebSocket pelo identificador.
func (cm *ConnectionManager) Remove(id string) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	delete(cm.connections, id)
}

// Get busca uma conexao WebSocket pelo identificador.
func (cm *ConnectionManager) Get(id string) (*websocket.Conn, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	conn, exists := cm.connections[id]
	if !exists {
		return nil, ErrConnectionNotFound
	}
	return conn.conn, nil
}

// SendMessage envia uma mensagem pela conexao WebSocket informada.
func (cm *ConnectionManager) SendMessage(id string, message contracts.WSMessage) error {
	cm.mutex.Lock()

	wrapper, exists := cm.connections[id]

	cm.mutex.Unlock()

	if !exists {
		return ErrConnectionNotFound
	}

	wrapper.mutex.Lock()
	defer wrapper.mutex.Unlock()

	return wrapper.conn.WriteJSON(message)
}
