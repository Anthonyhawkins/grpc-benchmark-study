# Client-Unary/Server-Stream vs Binary Stream - gRPC Study
This study aims to compare the use of a client-unary/server-stream vs a binary, client/server stream using gRPC.  
## Simulated Overhead
To simulate some real-world use-cases, the client and server will perform a significant amount of overhead to facilitate communication. They are listed as follows:
- **TLS with Client Auth**: The client and server will establish a single TLS connection, which will be re-used for every gRPC invocation.
- **JWT Authentication**: The client will send a JWT Token to be validated by the server.  This overhead can be adjusted in the following ways:
  - **Once**: The client will generate a single JWT token, and use it for every rpc invocation. (less overhead, default)
  - **Every**: The client will generate a new JWT token to be used for every new rpc invocation. (more overhead)
- **Cryptographic Message Signing**: Each message sent by the client and server will be signed. Each message received by the client/server will be verified.
- **Serializing/Deserializing**: Each message being sent is JSON serialized into `[]byte` to be deserialized by the other side.
- **Number Crunching**: The gRPC service being implemented is a simple calculator with two functions, they are: add two numbers together (easy), or determine if the first number provided in the message is a prime (scalable difficulty). You can make the server work harder or easier by providing it a bigger number to determine a `isPrime` result.
- **Headers**: Attaching some gRPC metdata to each gRPC invocation.

## Description of RPCs
The Service implemented is as follows
```protobuf
syntax = "proto3";

option go_package = "grpc-benchmark-study/calculator";

package calculator;

import "google/protobuf/empty.proto";

message CalcMessage {
  bytes payload = 1;
}

service CalculatorService {
  rpc performCalculationBi (stream CalcMessage) returns (stream CalcMessage);
  rpc performCalculationTo (CalcMessage) returns (google.protobuf.Empty);
  rpc performCalculationFrom (google.protobuf.Empty) returns (stream CalcMessage);
}

```
and each `CalcMessage` represents a `[]byte` serialized representation of the following struct
```go
type Calculation struct {
	ID        int32  `json:"id"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Operation string `json:"operation"`
	Result    int    `json:"result"`
	Prime     bool   `json:"isPrime"`
}
```
### Comparison of RPCs
**Client Unary/Server Stream**
```protobuf
  rpc performCalculationTo (CalcMessage) returns (google.protobuf.Empty);
  rpc performCalculationFrom (google.protobuf.Empty) returns (stream CalcMessage);
```
The client will send one or more requests concurrently to the server via the `rpc performCalculationTo`. All responses, will be returned eventually by the server via the `rpc performCalculationFrom`.

There are a few advantages to this setup.  They are as follows:
- The client can take advantage of gRPC load-balancing features using plugins such as xDS.  As each invocation would potentially be balanced to a different gRPC endpoint, a noisy client using a bidirectional stream would not potentially overload a single endpoint.
- Unary requests can be scaled up or down using worker routines to increase send rate to the server.  This could potentially account for any disadvantages, listed below.
- The client can subscribe to response messages on a separate stream. Response 

There are a few disadvantages to this method. They are as follows:
- Each invocation of an RPC requires the following overhead
  - Generating a new JWT token (if setup to generate on each invocation), validating each 
  - Setup and Teardown of HTTP/2 stream (even unary is an underlying HTTP/2 stream)
  - Attaching and parsing of headers/metadata on each rpc invocation.

**Binary Client/Server Stream**
```protobuf
  rpc performCalculationBi (stream CalcMessage) returns (stream CalcMessage);
```
The client will establish a single bidirectional stream to the server and send messages through it to the server.  The server will send responses back on the stream, similar to the server stream of the unary method.

There are a few advantages to this setup. They are as follows:
- HTTP/2 stream is established once.
- JWT token creation/validating occurs once.
- Attached headers/parsing headers occurs once.

The disadvantages of this setup are as follows:
- bidirectional streams cannot be effectively load-balanced. Because each RPC is load-balanced, and not message. A single stream between a client and a server could potentially overload a single-node.  The load-balanced with current out-of-the-box solutions.
- Client is limited to a single HTTP/2 stream for requests. With unary, we can scale up workers to make concurrent unary requests.  Creating multiple bidirectional streams could add additional complexity. 

## Test Environment
These tests were run on two Google Cloud compute VMS, specifically:
```bash
MACHINE_TYPE="e2-medium"                # Machine type.
IMAGE_FAMILY="debian-11"                # OS image family.
```
The client was deployed on the `grpc-client` VM, while the server was deployed on the `grpc-server` VM.  

NOTE: These VMs are effectively in the same subnet. Network overhead is not a part of this test.



### Setup
If you'd like to run the tests yourself, you'll need a Google Cloud project and the gcloud CLI installed on your system. Once that is done, edit the `scripts/gcloud-ctl.sh` script to change the top-level variables to suit your Google Cloud project, VM types, etc.

You can then create and destroy the test environment using the following commands.
```bash
scripts/gcloud-ctl.sh create
scripts/gcloud-ctl.sh destroy
```

The `create` arg on the `gcloud-ctl.sh` script will print you out some `ssh` commands you can use to log into the VMs. I like to have a terminal for the client and the server.  

**Starting the gRPC Server**
Once you're in the server VM, you can start the server by running `$ ./server`
```bash
anthony@grpc-server:~$ ./server 
2025/02/15 03:24:36 Server listening on 0.0.0.0:50051
```

### Running Tests
Once you're on the client VM, you can begin running some tests. Here's a simple one, let's run through it. (Your Host IP will be different! This is the grpc-server's internal IP.)

**Example Test**
```bash
./client -host=10.128.0.2:50051 -mode=bidirectional -interval=1 -transactions=1000 -client-id=myClient -x 3 -y 1 -operation=isprime -latency-gt=10 -jwt-gen=once
```
- `-mode=bidirectional`: This test will run using the binary client/server RPCs. Single binary stream, with the client and server both sending and receiving concurrently.
- `interval=1`: The client will send a request on the stream every 1ms.
- `-transactions=1000`: Run the test until 1000 messages have been sent by the client, wait a little for responses.
- `-client-id=myClient`: This gets attached as a metadata gRPC header, used by server to uniquely identify clients and response streams.
- `-x 3 -y 1 -operation=isprime`: What two numbers, and operations are being fed to the calculator.  `isPrime` will only determine if `x` is prime or not. 
- `-latency-gt=10`: When the summary is printed, only show responses where the round-trip was greater than the int specified. This value is `milliseconds`.
- `jwt-gen=once`: Only generate a JWT token once, use on every gRPC invocation, more important for `unary` mode.

**Example Results**
```bash
anthony@grpc-client:~$ ./client -host=10.128.0.2:50051 -mode=bidirectional -interval=1 -transactions=1000 -client-id=myClient -x 3 -y 1 -operation=isprime -latency-gt=5 -jwt-gen=once
2025/02/15 03:31:46 Generated JWT token (once mode)
2025/02/15 03:31:46 Running in bidirectional mode with client-id=myClient, interval=1ms, total transactions=1000
2025/02/15 03:31:46 Sent bidirectional message 0
2025/02/15 03:31:46 Sent bidirectional message 1
2025/02/15 03:31:46 Received response for ID=0, Latency=6ms, Response: id=0, x=3, y=1, operation=isprime, result=0, isPrime=true
2025/02/15 03:31:46 Sent bidirectional message 2
2025/02/15 03:31:46 Received response for ID=1, Latency=5ms, Response: id=1, x=3, y=1, operation=isprime, result=0, isPrime=true
2025/02/15 03:31:46 Sent bidirectional message 3
...
2025/02/15 03:31:49 Bidirectional receive error: rpc error: code = Unknown desc = EOF
2025/02/15 03:31:49 All transactions sent. Waiting for pending responses...
2025/02/15 03:31:51 ==== SUMMARY ====
2025/02/15 03:31:51 Duration: 4.87s
2025/02/15 03:31:51 Average Request TPS: 347.00, Max Request TPS: 355
2025/02/15 03:31:51 Average Response TPS: 346.50, Max Response TPS: 355
2025/02/15 03:31:51 Tracking summary (only entries with latency > 5ms):
2025/02/15 03:31:51 ID=701, Sent=id=701, x=3, y=1, operation=isprime, result=0, isPrime=false, Response=id=701, x=3, y=1, operation=isprime, result=0, isPrime=true, Received=true, Latency=8ms
2025/02/15 03:31:51 ID=132, Sent=id=132, x=3, y=1, operation=isprime, result=0, isPrime=false, Response=id=132, x=3, y=1, operation=isprime, result=0, isPrime=true, Received=true, Latency=6ms
2025/02/15 03:31:51 ID=968, Sent=id=968, x=3, y=1, operation=isprime, result=0, isPrime=false, Response=id=968, x=3, y=1, operation=isprime, result=0, isPrime=true, Received=true, Latency=6ms
2025/02/15 03:31:51 ID=21, Sent=id=21, x=3, y=1, operation=isprime, result=0, isPrime=false, Response=id=21, x=3, y=1, operation=isprime, result=0, isPrime=true, Received=true, Latency=6ms
...
2025/02/15 03:31:51 ID=0, Sent=id=0, x=3, y=1, operation=isprime, result=0, isPrime=false, Response=id=0, x=3, y=1, operation=isprime, result=0, isPrime=true, Received=true, Latency=6ms
2025/02/15 03:31:51 ID=39, Sent=id=39, x=3, y=1, operation=isprime, result=0, isPrime=false, Response=id=39, x=3, y=1, operation=isprime, result=0, isPrime=true, Received=true, Latency=6ms
2025/02/15 03:31:51 ID=88, Sent=id=88, x=3, y=1, operation=isprime, result=0, isPrime=false, Response=id=88, x=3, y=1, operation=isprime, result=0, isPrime=true, Received=true, Latency=6ms
```

### Reading Results
When a client is done sending a gRPC `EOF` error is returned, which will close down the stream.  
```bash
2025/02/15 03:31:49 Bidirectional receive error: rpc error: code = Unknown desc = EOF
```
Next you can see how long the test ran for, in addition to average TPS, and max TPS for requests and responses.
```bash
2025/02/15 03:31:51 Duration: 4.87s
2025/02/15 03:31:51 Average Request TPS: 347.00, Max Request TPS: 355
2025/02/15 03:31:51 Average Response TPS: 346.50, Max Response TPS: 355
```

Lastly, all requests and corresponding responses are tracked and displayed at the end.  Adding the `-latency-gt=<INT>` is a good way to only care about round trips greater than a specified threshold. 


## Generate gRPC Stubs
```bash
cd protos
protoc --go_out=. --go-grpc_out=. calculator.proto
```

### Example Commands
A Prime number which would result in some CPU crunch and cause latency > 10ms on Mac Mini M4
```bash
go run client.go -host=127.0.0.1:50051 -mode=bidirectional -interval=2000 -transactions=3 -client-id=myClient -x 1000000000000037 -y 1 -operation=isprime
```