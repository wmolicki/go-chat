package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/google/uuid"
	pb "github.com/wmolicki/go-chat/pkg/message/proto"
	"google.golang.org/grpc"
)

const ListenAddr = "localhost:8081"

type client struct {
	clientId  uuid.UUID
	name      string
	messageCh chan chatMessage
}

func (c client) String() string {
	return fmt.Sprintf("Client[%s (%s)]", c.name, c.clientId)
}

func getClientById(id uuid.UUID, clients map[uuid.UUID]*client) (*client, error) {
	for _, c := range clients {
		if c.clientId == id {
			return c, nil
		}
	}
	return nil, fmt.Errorf("no such client: %s", id)
}

type chatMessage struct {
	recipient string
	sender    string
	text      string
}

type server struct {
	pb.UnimplementedChatServerServer
	clientCount   int32
	clientCountMu sync.Mutex

	clients   map[uuid.UUID]*client
	clientsMu sync.Mutex
}

func (s *server) Connect(ctx context.Context, in *pb.ConnectRequest) (*pb.ConnectResponse, error) {
	s.clientCountMu.Lock()
	defer s.clientCountMu.Unlock()
	s.clientCount += 1

	id, err := uuid.NewRandom()
	if err != nil {
		log.Fatalf("could not generate uuid: %v\n", err)
	}
	c := client{
		clientId:  id,
		name:      in.GetName(),
		messageCh: make(chan chatMessage, 100),
	}
	s.clients[c.clientId] = &c
	log.Printf("client %s connected\n", c)
	return &pb.ConnectResponse{ClientId: id.String()}, nil
}

func (s *server) GetConnectedClients(ctx context.Context, in *pb.ConnectedClientsRequest) (*pb.ConnectedClientsResponse, error) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	clients := []*pb.ConnectedClientsResponse_ConnectedClient{}
	for _, client := range s.clients {
		clients = append(clients, &pb.ConnectedClientsResponse_ConnectedClient{Name: client.name, Id: client.clientId.String()})
	}

	resp := pb.ConnectedClientsResponse{Clients: clients}

	return &resp, nil
}

func (s *server) Message(ctx context.Context, in *pb.ChatMessage) (*pb.MessageResponse, error) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	recipient, err := getClientById(uuid.MustParse(in.RecipientId), s.clients)
	if err != nil {
		return nil, err
	}
	sender, err := getClientById(uuid.MustParse(in.SenderId), s.clients)
	if err != nil {
		return nil, err
	}

	recipient.messageCh <- chatMessage{recipient: recipient.clientId.String(), text: in.GetText(), sender: sender.clientId.String()}
	return &pb.MessageResponse{}, nil
}

func (s *server) ReceiveMessages(in *pb.ReceiveRequest, stream pb.ChatServer_ReceiveMessagesServer) error {
	s.clientsMu.Lock()
	receiver, err := getClientById(uuid.MustParse(in.ClientId), s.clients)
	s.clientsMu.Unlock()
	if err != nil {
		return err
	}

	for m := range receiver.messageCh {
		err := stream.Send(&pb.ChatMessage{Text: m.text, SenderId: m.sender, RecipientId: m.recipient})
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	listener, err := net.Listen("tcp", ListenAddr)
	if err != nil {
		log.Fatalf("can not listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	s := server{clients: make(map[uuid.UUID]*client)}
	pb.RegisterChatServerServer(grpcServer, &s)

	log.Printf("started server at %s\n", ListenAddr)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("cant serve grpc: %v", err)
	}

}
