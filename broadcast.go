/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"net/rpc"
	"os"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"

	"github.com/op/go-logging"

	"golang.org/x/net/context"

	"google.golang.org/grpc"
)

// The broadcast client is called as
//     obx broadcast <control address> <server> <channel> <client>
func broadcast() {

	logger = logging.MustGetLogger("broadcast")

	// Parse args

	control := os.Args[2]
	server, _ := strconv.Atoi(os.Args[3])
	channel, _ := strconv.Atoi(os.Args[4])
	clientIndex, _ := strconv.Atoi(os.Args[5])
	client := Client{
		Type:    Broadcast,
		Server:  server,
		Channel: channel,
		Client:  clientIndex,
	}

	// Connect back to the control process for RPC, get the full
	// configuration and start time, then initialize logging.

	rpcClient, err := rpc.DialHTTP("tcp", control)
	if err != nil {
		logger.Fatalf("RPC connection to control process failed: %s", err)
	}

	var cfg Config
	err = rpcClient.Call("Control.GetConfig", client, &cfg)
	if err != nil {
		logger.Fatalf("RPC call for Control.GetConfig failed: %s", err)
	}

	var Tstart time.Time
	err = rpcClient.Call("Control.Tstart", client, &Tstart)
	if err != nil {
		logger.Fatalf("RPC call for Control.Tstart failed: %s", err)
	}

	initLogging(cfg.BroadcastLogging)
	logger.Debugf("Broadcast client %v: Configuration %v\n", client, cfg)

	// Open the gRPC connection to the orderer

	connection, err :=
		grpc.Dial(cfg.Bservers[server], grpc.WithInsecure())
	if err != nil {
		client.fail(rpcClient,
			"Broadcast client %v did not connect to %s: %s\n",
			client, cfg.Bservers[server], err)
	}
	iface := orderer.NewAtomicBroadcastClient(connection)
	stream, err := iface.Broadcast(context.Background())
	if err != nil {
		client.fail(rpcClient,
			"Broadcast client %v to server %s; Failed to invoke broadcast RPC: %s",
			client, cfg.Bservers[server], err)
	}

	// Start the ACK thread

	done := make(chan int)
	go broadcastReplies(&client, stream, cfg.Transactions, done, rpcClient)

	// Do the broadcast

	message := &common.Envelope{}
	data := make([]byte, cfg.Payload)

	header := TxHeader{
		Server:  uint16(server),
		Channel: uint16(channel),
		Client:  uint16(clientIndex),
	}

	for tx := 0; tx < cfg.Transactions; {
		for i := 0; i < cfg.Burst; i++ {

			logger.Debugf("Broadcast client %v: Send Tx %d", client, tx)

			timestamp := uint64(time.Since(Tstart))

			header.Sequence = uint32(tx)
			header.Tbroadcast = timestamp
			header.Put(data)

			payload, err := proto.Marshal(&common.Payload{Data: data})
			if err != nil {
				client.fail(rpcClient,
					"Broadcast client %v: Payload marshaling failed: %s",
					client, err)
			}
			message.Payload = payload

			err = stream.Send(message)
			if err != nil {
				client.fail(rpcClient,
					"Broadcast client %v: Send() error: %s",
					client, err)
			}

			tx++
			if tx == cfg.Transactions {
				break
			}
		}

		if (tx < cfg.Transactions) && (cfg.Delay != 0) {
			time.Sleep(cfg.Delay)
		}
	}

	// Wait for the ACK thread, signal Done, and we're oot.

	<-done

	var ignore int
	err = rpcClient.Call("Control.BroadcastDone", client, &ignore)
	if err != nil {
		logger.Fatalf(
			"Broadcast client %v: RPC Control.BroadcastDone failed: %s",
			client, err)
	}

	stream.CloseSend()
}

// broadcastReplies handles the broadcast ACKs
func broadcastReplies(
	client *Client, stream orderer.AtomicBroadcast_BroadcastClient,
	tx int, done chan int, rpcClient *rpc.Client) {

	for count := 0; count < tx; count++ {

		reply, err := stream.Recv()
		if err != nil {
			client.fail(rpcClient,
				"Ack client %v: Reply error at count %d: %s",
				client, count, err)
		}

		logger.Debugf("Ack client %v: Reply from orderer at count %d: %s",
			client, count, reply.Status.String())
	}

	done <- 0
}
