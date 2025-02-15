package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"grpc-benchmark-study/internal/messagesigning"
	"grpc-benchmark-study/internal/resources"
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"grpc-benchmark-study/internal/calculation"
	"grpc-benchmark-study/internal/jwtutil" // Assumed JWT utility package

	pb "grpc-benchmark-study/protos/grpc-benchmark-study/calculator" // Update with your actual module/import path.
)

// calcServer implements the CalculatorService gRPC interface.
type calcServer struct {
	pb.UnimplementedCalculatorServiceServer

	// mu protects access to the clients map.
	mu sync.Mutex
	// clients maps a clientId (extracted from metadata) to its subscription channel.
	clients map[string]chan *pb.CalcMessage
}

func newCalcServer() *calcServer {
	return &calcServer{
		clients: make(map[string]chan *pb.CalcMessage),
	}
}

// validateJWT extracts the "authorization" header from the context and validates the JWT token.
func validateJWT(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "no metadata in context")
	}
	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return status.Error(codes.Unauthenticated, "no authorization header provided")
	}
	tokenStr := authHeaders[0]
	const prefix = "Bearer "
	if len(tokenStr) < len(prefix) || tokenStr[:len(prefix)] != prefix {
		return status.Error(codes.Unauthenticated, "invalid authorization header format")
	}
	tokenStr = tokenStr[len(prefix):]

	// Validate token using the jwtutil package.
	token, err := jwtutil.ValidateToken(tokenStr)
	if err != nil || token == nil || !token.Valid {
		return status.Error(codes.Unauthenticated, "invalid token")
	}
	return nil
}

// PerformCalculationBi implements a bidirectional streaming RPC.
// It first validates the JWT token, then simply echoes each incoming CalcMessage back to the client.
func (s *calcServer) PerformCalculationBi(stream pb.CalculatorService_PerformCalculationBiServer) error {
	// Validate JWT from the stream context.
	if err := validateJWT(stream.Context()); err != nil {
		log.Printf("PerformCalculationBi: JWT validation failed: %v", err)
		return err
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			log.Printf("PerformCalculationBi: error receiving: %v", err)
			return err
		}
		log.Printf("PerformCalculationBi: Received message from client")

		payload, err := messagesigning.Verify(msg.GetPayload())
		if err != nil {
			log.Printf("Failed to verify response: %v", err)
			return err
		}

		results, err := calculation.PerformCalculation(payload)
		if err != nil {
			log.Printf("PerformCalculationBi: error performing calculation: %v", err)
			return err
		}

		signedMessage, err := messagesigning.Sign(results)
		if err != nil {
			log.Fatalf("Failed to sign message: %v", err)
		}

		response := &pb.CalcMessage{
			Payload: signedMessage,
		}

		if err := stream.Send(response); err != nil {
			log.Printf("PerformCalculationBi: error sending: %v", err)
			return err
		}
	}
}

// PerformCalculationTo implements a unary RPC.
// It validates the JWT token, then receives a CalcMessage and sends it only to the intended recipient based on msg.ClientId.
func (s *calcServer) PerformCalculationTo(ctx context.Context, msg *pb.CalcMessage) (*emptypb.Empty, error) {
	// Validate JWT.
	if err := validateJWT(ctx); err != nil {
		log.Printf("PerformCalculationTo: JWT validation failed: %v", err)
		return &emptypb.Empty{}, err
	}

	// Extract the client ID from metadata.
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		log.Printf("PerformCalculationTo: no metadata found")
		return &emptypb.Empty{}, status.Error(codes.InvalidArgument, "metadata not provided")
	}
	clientIDs := md.Get("clientId")
	if len(clientIDs) == 0 {
		log.Printf("PerformCalculationTo: clientId not provided in metadata")
		return &emptypb.Empty{}, status.Error(codes.InvalidArgument, "clientId header is missing")
	}
	clientID := clientIDs[0]

	log.Printf("PerformCalculationTo: Received message for client %s", clientID)

	// Look up the client for the given clientId.
	s.mu.Lock()
	ch, exists := s.clients[clientID]
	s.mu.Unlock()

	if exists {

		payload, err := messagesigning.Verify(msg.GetPayload())
		if err != nil {
			log.Printf("Failed to verify response: %v", err)
			return &emptypb.Empty{}, status.Error(codes.Internal, "unable to verify message")
		}

		results, err := calculation.PerformCalculation(payload)
		if err != nil {
			log.Printf("PerformCalculationTo: error performing calculation: %v", err)
			return &emptypb.Empty{}, status.Error(codes.Internal, "unable to perform calculation")
		}

		signedMessage, err := messagesigning.Sign(results)
		if err != nil {
			log.Fatalf("Failed to sign message: %v", err)
		}

		response := &pb.CalcMessage{
			Payload: signedMessage,
		}

		select {
		case ch <- response:
			log.Printf("PerformCalculationTo: Sent message to client %s", clientID)
		default:
			log.Printf("PerformCalculationTo: Channel for client %s is full, dropping message", clientID)
		}
	} else {
		log.Printf("PerformCalculationTo: No subscriber for client %s", clientID)
	}

	return &emptypb.Empty{}, nil
}

// PerformCalculationFrom implements a server streaming RPC.
// It validates the JWT token, then expects the client to provide its clientId in the metadata headers.
// All messages (from PerformCalculationTo) are streamed back to the client.
func (s *calcServer) PerformCalculationFrom(empty *emptypb.Empty, stream pb.CalculatorService_PerformCalculationFromServer) error {
	// Validate JWT.
	if err := validateJWT(stream.Context()); err != nil {
		log.Printf("PerformCalculationFrom: JWT validation failed: %v", err)
		return err
	}

	// Extract metadata.
	md, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		log.Printf("PerformCalculationFrom: no metadata found")
		return status.Errorf(codes.InvalidArgument, "metadata not provided")
	}
	clientIDs := md.Get("clientId")
	if len(clientIDs) == 0 {
		log.Printf("PerformCalculationFrom: clientId not provided in metadata")
		return status.Errorf(codes.InvalidArgument, "clientId header is missing")
	}
	clientID := clientIDs[0]

	// Create a new channel for this client.
	ch := make(chan *pb.CalcMessage, 10)

	// Register the client.
	s.mu.Lock()
	s.clients[clientID] = ch
	s.mu.Unlock()

	log.Printf("PerformCalculationFrom: New client %s connected", clientID)

	// Ensure cleanup when stream ends.
	defer func() {
		s.mu.Lock()
		delete(s.clients, clientID)
		s.mu.Unlock()
		close(ch)
		log.Printf("PerformCalculationFrom: Client %s disconnected", clientID)
	}()

	// Stream messages to the client.
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(msg); err != nil {
				log.Printf("PerformCalculationFrom: error sending to client %s: %v", clientID, err)
				return err
			}
		case <-stream.Context().Done():
			log.Printf("PerformCalculationFrom: client %s context done", clientID)
			return stream.Context().Err()
		}
	}
}

func main() {
	// CLI flags for listen IP and port.
	listenIP := flag.String("ip", "0.0.0.0", "Listen IP address")
	port := flag.String("port", "50051", "Listen port")
	flag.Parse()

	// Load JWT Pub Key
	err := jwtutil.LoadKeys("jwt/jwt.key", "jwt/jwt.pub")
	if err != nil {
		log.Fatalf("Unable to load public key: %v", err)
	}

	//Message Signing
	err = messagesigning.LoadSigner("cms/signer.crt", "cms/signer.key", "cms/ca.crt")
	if err != nil {
		log.Fatalf("Failed to load signer key: %v", err)
	}

	// Build listen address.
	addr := net.JoinHostPort(*listenIP, *port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}
	log.Printf("Server listening on %s", addr)

	// Load server certificate and key.
	certBytes, err := resources.Certs.ReadFile("certs/server.crt")
	if err != nil {
		log.Fatalf("Failed to read embedded server.crt: %v", err)
	}
	keyBytes, err := resources.Certs.ReadFile("certs/server.key")
	if err != nil {
		log.Fatalf("Failed to read embedded server.key: %v", err)
	}
	cert, err := tls.X509KeyPair(certBytes, keyBytes)
	if err != nil {
		log.Fatalf("Failed to load X509 key pair from embedded certs: %v", err)
	}

	// Load CA certificate for client validation.
	caCert, err := resources.Certs.ReadFile("certs/ca.crt")
	if err != nil {
		log.Fatalf("Failed to read CA certificate: %v", err)
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		log.Fatalf("Failed to append CA certificate")
	}

	// Create TLS configuration for mutual TLS.
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}

	// Create gRPC credentials.
	creds := credentials.NewTLS(tlsConfig)

	// Create a new gRPC server with TLS enabled.
	grpcServer := grpc.NewServer(grpc.Creds(creds))
	pb.RegisterCalculatorServiceServer(grpcServer, newCalcServer())

	// Start serving.
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
