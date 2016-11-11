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

	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"

	"github.com/op/go-logging"

	context "golang.org/x/net/context"
	"google.golang.org/grpc"
)

// The deliver client is called as
//     obx deliver <control address> <server> <channel> <client>
func deliver() {

	logger = logging.MustGetLogger("deliver")

	// Parse args

	control := os.Args[2]
	server, _ := strconv.Atoi(os.Args[3])
	channel, _ := strconv.Atoi(os.Args[4])
	clientIndex, _ := strconv.Atoi(os.Args[5])
	client := Client{
		Type:    Deliver,
		Server:  server,
		Channel: channel,
		Client:  clientIndex,
	}

	// Connect back to the control process for RPC, then get the full
	// configuration and initialize logging.

	rpcClient, err := rpc.DialHTTP("tcp", control)
	if err != nil {
		logger.Fatalf("RPC connection to control process failed: %s", err)
	}

	var cfg Config
	err = rpcClient.Call("Control.GetConfig", client, &cfg)
	if err != nil {
		logger.Fatalf(
			"Deliver client %v: RPC call for Control.GetConfig failed: %s",
			client, err)
	}

	initLogging(cfg.DeliverLogging)
	logger.Debugf("Deliver client %v: Configuration %v\n", client, cfg)

	// Open the gRPC connection to the orderer

	connection, err :=
		grpc.Dial(cfg.Dservers[server], grpc.WithInsecure())
	if err != nil {
		logger.Fatalf("Deliver client %v could not connect to %s: %s\n",
			client, cfg.Dservers[server], err)
	}
	iface := ab.NewAtomicBroadcastClient(connection)
	stream, err := iface.Deliver(context.Background())
	if err != nil {
		logger.Fatalf("Deliver client %v to server %s; Failed to invoke deliver RPC: %s",
			client, cfg.Dservers[server], err)
	}

	// Make the seek request. Then call back to signal that we're ready to
	// run, obtaining the coordinated start time.

	updateSeek := &ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Seek{
			Seek: &ab.SeekInfo{
				WindowSize: uint64(cfg.Window),
			},
		},
	}
	updateSeek.GetSeek().Start = ab.SeekInfo_OLDEST

	err = stream.Send(updateSeek)
	if err != nil {
		logger.Fatalf("Deliver client %v: Failed to send updateSeek: %s",
			client, err)
	}

	var tStart time.Time
	err = rpcClient.Call("Control.Start", client, &tStart)
	if err != nil {
		logger.Fatalf("Deliver client %v: RPC Control.Start failed: %s",
			client, err)
	}

	// Do it

	var block int
	var tx uint64
	txDB := make([]TxHeader, cfg.TxDeliveredPerClient)

	updateAck := &ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Acknowledgement{
			Acknowledgement: &ab.Acknowledgement{}, // Has a Number field
		},
	}

	for tx < cfg.TxDeliveredPerClient {

		reply, err := stream.Recv()
		if err != nil {
			logger.Fatalf("Deliver client %v: Reply error at block %d: %s",
				client, block, err)
		}

		switch t := reply.GetType().(type) {
		case *ab.DeliverResponse_Block:

			timestamp := uint64(time.Since(tStart))

			logger.Debugf("Deliver client %v: Block %d @ TX %d holds %d new TX",
				client, t.Block.Header.Number, tx, len(t.Block.Data.Data))

			block++
			if block%cfg.AckEvery == 0 {
				updateAck.GetAcknowledgement().Number = t.Block.Header.Number
				err = stream.Send(updateAck)
				if err != nil {
					logger.Fatalf(
						"Deliver client %v: "+
							"Failed to send ACK update to orderer: %s",
						err)
				}
				logger.Debugf("Deliver client %v: Sent ACK for block %d",
					client, t.Block.Header.Number)
			}

			for _, message := range t.Block.Data.Data {
				if len(message) != cfg.Payload {
					logger.Debugf(
						"Deliver client %v: "+
							"Unexpected message size %d at TX %d; "+
							"Message ignored",
						client, len(message), tx)
					continue // Genesis messages are ignored
				}
				txDB[tx].Get(message)
				txDB[tx].Tdelivered = timestamp
				logger.Debugf("Deliver client %v: Header: %v", client, txDB[tx])
				tx++
				if tx == cfg.TxDeliveredPerClient {
					break
				}
			}

		case *ab.DeliverResponse_Error:
			logger.Errorf(
				"Deliver client %v: Orderer delivered error response: %s",
				client, t.Error.String())
		}
	}

	// We're out

	var ignore int
	err = rpcClient.Call("Control.Done", client, &ignore)
	if err != nil {
		logger.Fatalf("Deliver client %v: RPC Control.Done failed: %s",
			client, err)
	}
}
