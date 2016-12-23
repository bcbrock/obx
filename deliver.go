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
	"fmt"
	"math"
	"net/rpc"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"

	"github.com/op/go-logging"

	"golang.org/x/net/context"

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
	client := &Client{
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

	cfg := &Config{}
	err = rpcClient.Call("Control.GetConfig", client, cfg)
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
		client.fail(rpcClient,
			"Deliver client %v could not connect to %s: %s\n",
			client, cfg.Dservers[server], err)
	}
	iface := orderer.NewAtomicBroadcastClient(connection)
	stream, err := iface.Deliver(context.Background())
	if err != nil {
		client.fail(rpcClient,
			"Deliver client %v to server %s; Failed to invoke deliver RPC: %s",
			client, cfg.Dservers[server], err)
	}

	// Make the seek request. Then call back to signal that we're ready to
	// run, obtaining the coordinated start time.

	seek := &orderer.SeekInfo{
		ChainID: provisional.TestChainID,
		Start: &orderer.SeekPosition{
			Type: &orderer.SeekPosition_Oldest{
				Oldest: &orderer.SeekOldest{},
			},
		},
		Stop: &orderer.SeekPosition{
			Type: &orderer.SeekPosition_Specified{
				Specified: &orderer.SeekSpecified{
					Number: math.MaxUint64},
			},
		},
		Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
	}

	err = stream.Send(seek)
	if err != nil {
		client.fail(rpcClient,
			"Deliver client %v: Failed to send updateSeek: %s",
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
	checkDB := make([]bool, cfg.TxDeliveredPerClient)
	envelope := new(common.Envelope)
	payload := new(common.Payload)

	for tx < cfg.TxDeliveredPerClient {

		reply, err := stream.Recv()
		if err != nil {
			client.fail(rpcClient,
				"Deliver client %v: Reply error at block %d: %s",
				client, block, err)
		}

		switch t := reply.Type.(type) {
		case *orderer.DeliverResponse_Block:

			timestamp := uint64(time.Since(tStart))

			logger.Debugf("Block %v", t)
			logger.Debugf("Deliver client %v: Block %d @ TX %d holds %d new TX",
				client, t.Block.Header.Number, tx, len(t.Block.Data.Data))

			block++

			for _, transaction := range t.Block.Data.Data {
				err := proto.Unmarshal(transaction, envelope)
				if err != nil {
					client.fail(rpcClient,
						"Unmarshal to Envelope failed: %s", err)
				} else {
					err = proto.Unmarshal(envelope.Payload, payload)
					if err != nil {
						client.fail(rpcClient,
							"Unmarshal to Payload failed: %s", err)
					}
					message := payload.Data
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
			}

		case *orderer.DeliverResponse_Status:
			client.fail(rpcClient,
				"Deliver client %v: Orderer delivered status response: %s",
				client, t.Status.String())
		}
	}

	elapsed := time.Since(tStart).Seconds() // Final timestamp

	// Check the results, that is to say, make sure that the TX received are
	// the TX expected, and only those. Any errors are reported by the control
	// process.

	var wrongChannel, missing uint64
	for tx = 0; tx < cfg.TxDeliveredPerClient; tx++ {
		t := &txDB[tx]
		x :=
			(uint64(t.Server) * uint64(cfg.Bclients) * uint64(cfg.Transactions)) +
				(uint64(t.Client) * uint64(cfg.Transactions)) +
				uint64(t.Sequence)
		checkDB[x] = true
		if int(t.Channel) != channel {
			wrongChannel++
		}
	}
	for tx = 0; tx < cfg.TxDeliveredPerClient; tx++ {
		if !checkDB[tx] {
			missing++
		}
	}

	// If the user requested latency statistics, dump them.

	if cfg.LatencyDir != "" {
		if err := dumpLatencies(client, cfg, txDB); err != nil {
			client.fail(rpcClient,
				"Deliver client %v: Error dumping latencies: %s",
				client, err)
		}
	}

	// We're out

	done := &DeliverClient{
		Client:       *client,
		Elapsed:      elapsed,
		Missing:      missing,
		WrongChannel: wrongChannel,
	}
	var ignore int
	err = rpcClient.Call("Control.DeliverDone", done, &ignore)
	if err != nil {
		logger.Fatalf("Deliver client %v: RPC Control.DeliverDone failed: %s",
			client, err)
	}
}

// Dump latency statistics to a CSV file. The default is to report summary
// statistics for blocks, where blocks are inferred by the delivery
// timestamps. But if requested we can also print all latencies.
func dumpLatencies(client *Client, cfg *Config, txDB []TxHeader) (err error) {
	fileName :=
		cfg.LatencyPrefix + "." +
			strconv.Itoa(client.Server) + "." +
			strconv.Itoa(client.Channel) + "." +
			strconv.Itoa(client.Client) + "." +
			"csv"
	path := filepath.Join(cfg.LatencyDir, fileName)
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	if cfg.LatencyAll {

		fmt.Fprintf(f,
			"Server,Channel,Client,Sequence,Tbroadcast,Tdelivered,Latency\n")
		for _, tx := range txDB {
			fmt.Fprintf(f, "%d,%d,%d,%d,%.9f,%.9f,%.9f\n",
				tx.Server, tx.Channel, tx.Client, tx.Sequence,
				float64(tx.Tbroadcast)/1e9, float64(tx.Tdelivered)/1e9,
				float64(tx.Tdelivered-tx.Tbroadcast)/1e9)
		}

	} else {

		fmt.Fprintf(f, "Block,NumTX,Tdelivered,MinLatency,MaxLatency\n")

		var block, numTX, blockTimestamp, minLatency, maxLatency uint64
		minLatency = 0xffffffffffffffff

		var tx TxHeader
		for _, tx = range txDB {

			if tx.Tdelivered != blockTimestamp {

				print := blockTimestamp != 0
				blockTimestamp = tx.Tdelivered

				if print {

					fmt.Fprintf(f, "%d,%d,%.9fd,%.9f,%.9f\n",
						block, numTX, float64(tx.Tdelivered)/1e9,
						float64(minLatency)/1e9, float64(maxLatency)/1e9)

					block++
					numTX = 0
					minLatency = 0xffffffffffffffff
					maxLatency = 0
				}
			}

			numTX++
			latency := tx.Tdelivered - tx.Tbroadcast
			if latency > maxLatency {
				maxLatency = latency
			}
			if latency < minLatency {
				minLatency = latency
			}

		}
		fmt.Fprintf(f, "%d,%d,%.9fd,%.9f,%.9f\n",
			block, numTX, float64(tx.Tdelivered)/1e9,
			float64(minLatency)/1e9, float64(maxLatency)/1e9)
	}
	return
}
