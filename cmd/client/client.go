package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"grpc-benchmark-study/internal/calculation"
	"grpc-benchmark-study/internal/jwtutil" // Assumed JWT utility package
	"grpc-benchmark-study/internal/messagesigning"
	"grpc-benchmark-study/internal/resources"
	"grpc-benchmark-study/internal/tracking"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "grpc-benchmark-study/protos/grpc-benchmark-study/calculator" // Update with your actual module/import path.
)

var requestCounter int64
var responseCounter int64

// Global variables for JWT mode.
var (
	jwtGenMode string // "once" or "every"
	storedJWT  string
)

// getJWTToken returns a JWT token string according to the selected mode.
func getJWTToken(clientID string) string {
	if jwtGenMode == "once" {
		return storedJWT
	}
	// For "every", generate a new token with simple claims.
	claims := jwt.MapClaims{
		"sub": clientID,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	}
	token, err := jwtutil.GenerateToken(claims)
	if err != nil {
		log.Fatalf("Failed to generate JWT token: %v", err)
	}
	return token
}

var verbose *bool

func main() {
	// Command-line flags.
	host := flag.String("host", "localhost:50051", "Server host:port")
	mode := flag.String("mode", "unary", "Mode: unary or bidirectional")
	workers := flag.Int("workers", 1, "Number of workers (only in unary mode)")
	interval := flag.Int("interval", 1000, "Interval between transactions in milliseconds")
	xFlag := flag.Int("x", 3, "Value of the X number")
	yFlag := flag.Int("y", 1, "Value of the Y number")
	operationFlag := flag.String("operation", "ADD", "Operation: ADD, SUBTRACT, ISPRIME")
	transactions := flag.Int("transactions", 10, "Number of transactions (per worker in unary mode, total in bidirectional)")
	clientID := flag.String("client-id", "default-client", "Client ID")
	latencyGt := flag.Int("latency-gt", 5, "Only print entries with latency greater than this (ms)")
	jwtGen := flag.String("jwt-gen", "once", "JWT generation mode: once or every")
	verbose = flag.Bool("verbose", false, "Verbose output")
	flag.Parse()

	// Set the JWT generation mode.
	jwtGenMode = *jwtGen
	if jwtGenMode != "once" && jwtGenMode != "every" {
		log.Fatalf("Invalid jwt-gen mode: %s. Allowed values are 'once' or 'every'.", jwtGenMode)
	}
	err := jwtutil.LoadKeys("jwt/jwt.key", "jwt/jwt.pub")
	if err != nil {
		log.Fatalf("Unable to load private key: %v", err)
	}

	// If "once", generate the JWT token at startup.
	if jwtGenMode == "once" {
		claims := jwt.MapClaims{
			"sub": *clientID,
			"iat": time.Now().Unix(),
			"exp": time.Now().Add(1 * time.Hour).Unix(),
		}
		token, err := jwtutil.GenerateToken(claims)
		if err != nil {
			log.Fatalf("Failed to generate JWT token: %v", err)
		}
		storedJWT = token
		log.Printf("Generated JWT token (once mode)")
	}

	//Message Signing
	err = messagesigning.LoadSigner("cms/signer.crt", "cms/signer.key", "cms/ca.crt")
	if err != nil {
	}
	if err != nil {
		log.Fatalf("Failed to load signer key: %v", err)
	}

	// --- TLS Setup ---
	// Load client certificate and key.
	certBytes, err := resources.Certs.ReadFile("certs/client.crt")
	if err != nil {
		log.Fatalf("Failed to read embedded client.crt: %v", err)
	}
	keyBytes, err := resources.Certs.ReadFile("certs/client.key")
	if err != nil {
		log.Fatalf("Failed to read embedded client.key: %v", err)
	}
	clientCert, err := tls.X509KeyPair(certBytes, keyBytes)
	if err != nil {
		log.Fatalf("Failed to load X509 key pair from embedded certs: %v", err)
	}
	// Load CA certificate.
	caCert, err := resources.Certs.ReadFile("certs/ca.crt")
	if err != nil {
		log.Fatalf("Failed to read CA certificate: %v", err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		log.Fatalf("Failed to append CA certificate")
	}
	// Create TLS configuration.
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
		ServerName:   "localhost", //Ensures it works even on different hosts, i.e. cloud env
		// Optionally set ServerName if needed.
	}
	creds := credentials.NewTLS(tlsConfig)
	// --- End TLS Setup ---

	// Create gRPC connection with TLS.
	conn, err := grpc.Dial(*host, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("Failed to connect to %s: %v", *host, err)
	}
	defer conn.Close()

	client := pb.NewCalculatorServiceClient(conn)

	switch *mode {
	case "unary":
		runUnaryMode(client, *clientID, *workers, *interval, *transactions, *xFlag, *yFlag, *operationFlag, *latencyGt)
	case "bidirectional":
		runBidiMode(client, *clientID, *interval, *transactions, *xFlag, *yFlag, *operationFlag, *latencyGt)
	default:
		log.Fatalf("Unknown mode: %s", *mode)
	}
}

// runUnaryMode sets up a PerformCalculationFrom stream to receive responses,
// spawns worker goroutines to call PerformCalculationTo, and tracks TPS.
func runUnaryMode(client pb.CalculatorServiceClient, clientID string, workers, interval, totalTransactions, x, y int, operation string, latencyThreshold int) {
	log.Printf("Running in unary mode with client-id=%s, workers=%d, interval=%dms, total transactions=%d",
		clientID, workers, interval, totalTransactions)

	// Create a new tracker.
	tracker := tracking.NewTracker()
	tracker.Start()

	// Create a ticker to measure TPS with a done channel.
	ticker := time.NewTicker(1 * time.Second)
	done := make(chan struct{})
	var tWg sync.WaitGroup
	tWg.Add(1)
	var totalTicks int64
	var totalReq, totalRes int64
	var maxReq, maxRes int64

	go func() {
		defer tWg.Done()
		for {
			select {
			case <-ticker.C:
				req := atomic.SwapInt64(&requestCounter, 0)
				res := atomic.SwapInt64(&responseCounter, 0)
				totalTicks++
				totalReq += req
				totalRes += res
				if req > maxReq {
					maxReq = req
				}
				if res > maxRes {
					maxRes = res
				}
			case <-done:
				return
			}
		}
	}()

	// Set up the response stream (attach JWT token as well).
	jwtToken := getJWTToken(clientID)
	md := metadata.Pairs("clientid", clientID, "authorization", "Bearer "+jwtToken)
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	respStream, err := client.PerformCalculationFrom(ctx, &emptypb.Empty{})
	if err != nil {
		log.Fatalf("Failed to open PerformCalculationFrom stream: %v", err)
	}

	// Goroutine to process responses.
	go func() {
		for {
			resp, err := respStream.Recv()
			if err != nil {
				log.Printf("Response stream closed: %v", err)
				return
			}

			payload, err := messagesigning.Verify(resp.GetPayload())
			if err != nil {
				log.Printf("Failed to verify response: %v", err)
				continue
			}

			respCalc, err := calculation.Read(payload)
			if err != nil {
				log.Printf("Failed to read response: %v", err)
				continue
			}
			tracker.RecordResponse(*respCalc)
			if entry, ok := tracker.GetEntry(respCalc.ID); ok {
				if *verbose {
					log.Printf("Received response for ID=%d, Latency=%dms, Response: %s", respCalc.ID, entry.LatencyMs, respCalc.String())
				}
				atomic.AddInt64(&responseCounter, 1)
			} else {
				log.Printf("Received response for unknown ID=%d", respCalc.ID)
			}
		}
	}()

	// Create a channel to act as a task queue.
	tasks := make(chan int, totalTransactions)
	for i := 0; i < totalTransactions; i++ {
		tasks <- i
	}
	close(tasks)

	// Spawn worker goroutines to process tasks.
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for task := range tasks {
				// Use the task as the transaction index.
				calc := calculation.Calculation{
					ID:        int32(task), // Unique transaction ID.
					X:         x,
					Y:         y,
					Operation: operation,
				}
				tracker.AddSent(calc)
				message, err := calc.Bytes()
				if err != nil {
					log.Fatalf("Failed to serialize message: %v", err)
				}
				signedMessage, err := messagesigning.Sign(message)
				if err != nil {
					log.Fatalf("Failed to sign message: %v", err)
				}
				msg := &pb.CalcMessage{
					Payload: signedMessage,
				}
				// Generate (or re-use) JWT token as per mode.
				jwtToken := getJWTToken(clientID)
				reqMd := metadata.Pairs("clientid", clientID, "authorization", "Bearer "+jwtToken)
				reqCtx := metadata.NewOutgoingContext(context.Background(), reqMd)
				_, err = client.PerformCalculationTo(reqCtx, msg)
				if err != nil {
					if *verbose {
						log.Printf("Worker %d: error sending transaction %d: %v", workerID, task, err)
					}
				} else {
					atomic.AddInt64(&requestCounter, 1)
					if *verbose {
						log.Printf("Worker %d: sent transaction %d", workerID, task)
					}
				}
				time.Sleep(time.Duration(interval) * time.Millisecond)
			}
		}(i)
	}
	wg.Wait()

	// All transactions sent—stop the ticker and signal the ticker goroutine to exit.
	ticker.Stop()
	close(done)
	tWg.Wait()

	// Optionally, wait a bit for any pending responses.
	log.Printf("All workers done. Waiting for pending responses...")
	time.Sleep(2 * time.Second)
	tracker.Stop()

	// Compute summary metrics.
	avgReq := float64(totalReq) / float64(totalTicks)
	avgRes := float64(totalRes) / float64(totalTicks)
	log.Printf("==== SUMMARY ====")
	log.Printf("Duration: %0.2fs", tracker.Duration().Seconds())
	log.Printf(tracker.SentReceivedSummary())
	log.Printf("Average Request TPS: %.2f, Max Request TPS: %d", avgReq, maxReq)
	log.Printf("Average Response TPS: %.2f, Max Response TPS: %d", avgRes, maxRes)

	log.Printf(tracker.LatencySummary().String())
	log.Printf("Tracking summary (only entries with latency > %dms):", latencyThreshold)
	// Print tracking summary for entries with latency greater than the threshold.
	for id, entry := range tracker.Data() {
		if entry.LatencyMs > int64(latencyThreshold) {
			log.Printf("ID=%d, Sent=%s, Response=%s, Received=%t, Latency=%dms",
				id, entry.Sent.String(), entry.Response.String(), entry.Received, entry.LatencyMs)
		}
	}
}

// runBidiMode establishes a PerformCalculationBi stream for bidirectional messaging,
// and uses similar TPS tracking as in unary mode.
func runBidiMode(client pb.CalculatorServiceClient, clientID string, interval, transactions, x, y int, operation string, latencyThreshold int) {
	log.Printf("Running in bidirectional mode with client-id=%s, interval=%dms, total transactions=%d", clientID, interval, transactions)

	// Create a new tracker.
	tracker := tracking.NewTracker()
	tracker.Start()

	// Create a ticker for TPS tracking with a done channel.
	ticker := time.NewTicker(1 * time.Second)
	done := make(chan struct{})
	var tWg sync.WaitGroup
	tWg.Add(1)
	var totalTicks int64
	var totalReq, totalRes int64
	var maxReq, maxRes int64

	go func() {
		defer tWg.Done()
		for {
			select {
			case <-ticker.C:
				req := atomic.SwapInt64(&requestCounter, 0)
				res := atomic.SwapInt64(&responseCounter, 0)
				totalTicks++
				totalReq += req
				totalRes += res
				if req > maxReq {
					maxReq = req
				}
				if res > maxRes {
					maxRes = res
				}
			case <-done:
				return
			}
		}
	}()

	// Establish bidirectional stream (attach JWT token as well).
	jwtToken := getJWTToken(clientID)
	md := metadata.Pairs("clientid", clientID, "authorization", "Bearer "+jwtToken)
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	stream, err := client.PerformCalculationBi(ctx)
	if err != nil {
		log.Fatalf("Failed to establish PerformCalculationBi stream: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	// Goroutine for receiving messages.
	go func() {
		defer wg.Done()
		for {
			resp, err := stream.Recv()
			if err != nil {
				log.Printf("Bidirectional receive error: %v", err)
				return
			}

			payload, err := messagesigning.Verify(resp.GetPayload())
			if err != nil {
				log.Printf("Failed to verify response: %v", err)
				continue
			}

			respCalc, err := calculation.Read(payload)
			if err != nil {
				log.Printf("Error reading response: %v", err)
				continue
			}
			tracker.RecordResponse(*respCalc)
			if entry, ok := tracker.GetEntry(respCalc.ID); ok {
				if *verbose {
					log.Printf("Received response for ID=%d, Latency=%dms, Response: %s", respCalc.ID, entry.LatencyMs, respCalc.String())
				}
				atomic.AddInt64(&responseCounter, 1)
			} else {
				log.Printf("Received response for unknown ID=%d", respCalc.ID)
			}
		}
	}()

	// Send transactions on the bidirectional stream.
	for i := 0; i < transactions; i++ {
		calc := calculation.Calculation{
			ID:        int32(i),
			X:         x,
			Y:         y,
			Operation: operation,
		}
		tracker.AddSent(calc)
		message, err := calc.Bytes()
		if err != nil {
			log.Fatalf("Failed to serialize message: %v", err)
		}

		signedMessage, err := messagesigning.Sign(message)
		if err != nil {
			log.Fatalf("Failed to sign message: %v", err)
		}

		msg := &pb.CalcMessage{
			Payload: signedMessage,
		}

		if err := stream.Send(msg); err != nil {
			log.Printf("Error sending message %d: %v", i, err)
			break
		}
		atomic.AddInt64(&requestCounter, 1)
		if *verbose {
			log.Printf("Sent bidirectional message %d", i)
		}
		time.Sleep(time.Duration(interval) * time.Millisecond)
	}

	// All transactions sent—stop the ticker and signal the ticker goroutine to exit.
	ticker.Stop()
	close(done)
	tWg.Wait()

	if err := stream.CloseSend(); err != nil {
		log.Printf("Error closing bidirectional stream: %v", err)
	}
	wg.Wait()

	log.Printf("All transactions sent. Waiting for pending responses...")
	time.Sleep(2 * time.Second)
	tracker.Stop()

	// Compute summary metrics.
	avgReq := float64(totalReq) / float64(totalTicks)
	avgRes := float64(totalRes) / float64(totalTicks)
	log.Printf("==== SUMMARY ====")
	log.Printf("Duration: %0.2fs", tracker.Duration().Seconds())
	log.Printf(tracker.SentReceivedSummary())
	log.Printf("Average Request TPS: %.2f, Max Request TPS: %d", avgReq, maxReq)
	log.Printf("Average Response TPS: %.2f, Max Response TPS: %d", avgRes, maxRes)
	log.Printf(tracker.LatencySummary().String())
	log.Printf("Tracking summary (only entries with latency > %dms):", latencyThreshold)
	for id, entry := range tracker.Data() {
		if entry.LatencyMs > int64(latencyThreshold) {
			log.Printf("ID=%d, Sent=%s, Response=%s, Received=%t, Latency=%dms",
				id, entry.Sent.String(), entry.Response.String(), entry.Received, entry.LatencyMs)
		}
	}
}
