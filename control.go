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
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/op/go-logging"
)

// Control is an object that represents the state of the control process, and
// is also the target of the RPC calls from clients. Besides the
// configuration and statistics object, it includes several synchronization
// objects used to sequence client operations.
type Control struct {
	cfg         *Config
	stats       *Stats
	startWG     sync.WaitGroup
	releaseWG   sync.WaitGroup
	broadcastWG sync.WaitGroup
	deliverWG   sync.WaitGroup
}

// GetConfig is the RPC callback to get the full configuration.
func (c *Control) GetConfig(client Client, reply *Config) error {
	logger.Debugf("Client %v requests the configuration", client)
	*reply = *c.cfg
	return nil
}

// Start is an RPC callback from deliver clients, where the deliver clients
// pend until all are ready, and the common starting time is returned.
func (c *Control) Start(client *Client, reply *time.Time) error {
	logger.Debugf("Deliver client %v ready to Start", client)
	c.startWG.Done()
	c.releaseWG.Wait()
	*reply = c.stats.Tstart
	return nil
}

// Tstart is an RPC callback from broadcast clients, used to get the start
// time.
func (c *Control) Tstart(client *Client, reply *time.Time) error {
	logger.Debugf("Broadcast client %v ready to Start", client)
	*reply = c.stats.Tstart
	return nil
}

// BroadcastDone is an RPC callback indicating that a broadcast client is done.
func (c *Control) BroadcastDone(client *Client, ignore *int) error {
	logger.Infof("Broadcast client %v signals Done", client)
	c.stats.Dbroadcast[client.Server][client.Channel][client.Client] =
		time.Since(c.stats.Tstart).Seconds()
	c.broadcastWG.Done()
	return nil
}

// DeliverDone is an RPC callback indicating that a deliver client is done.
func (c *Control) DeliverDone(client *DeliverClient, ignore *int) error {
	logger.Infof("Deliver client %v signals Done; Elapsed time %.3f",
		client.Client, client.Elapsed)
	c.stats.Ddeliver[client.Server][client.Channel][client.Client.Client] =
		client.Elapsed
	c.stats.Missing += client.Missing
	c.stats.WrongChannel += client.WrongChannel
	if c.stats.Missing != 0 {
		logger.Errorf("Client %v: %d missing TX",
			client.Client, client.Missing)
	}
	if c.stats.WrongChannel != 0 {
		logger.Errorf("Client %v: %d TX on the wrong channel",
			client.Client, client.WrongChannel)
	}
	if client.Elapsed > c.stats.DdeliverAll {
		c.stats.DdeliverAll = client.Elapsed
	}
	c.deliverWG.Done()
	return nil
}

// Fail in an RPC callback indicating that a client has failed for some
// reason. This callback causes an immediate termination of the program.
// Bug: The client.err is always coming back as NIL here.
func (c *Control) Fail(client *ClientFailed, ignore *int) error {
	logger.Fatalf("Client %v signals failure: %s", client.Client, client.err)
	return nil
}

// newControl initializes a Control object from a Config object.
func newControl(cfg *Config) *Control {
	c := Control{cfg: cfg, stats: newStats(cfg)}
	c.startWG.Add(int(cfg.TotalDeliverClients))
	c.releaseWG.Add(1)
	c.broadcastWG.Add(int(cfg.TotalBroadcastClients))
	c.deliverWG.Add(int(cfg.TotalDeliverClients))
	return &c
}

// The obx control process
func control() {

	logger = logging.MustGetLogger("control")

	// Create the Control object and register it for RPC (from an example in
	// the Golang rpc package documentation.) Once the server is started, we
	// must poll to make sure it is really up and running before starting the
	// client processes.

	cfg := newConfig()
	control := newControl(cfg)
	stats := control.stats

	rpc.Register(control)
	rpc.HandleHTTP()
	listener, err := net.Listen("tcp", cfg.ControlAddress)
	if err != nil {
		logger.Fatalf("net.Listen failed: %s", err)
	}
	go func() {
		logger.Fatalf("RPC service failed: %s", http.Serve(listener, nil))
	}()

	rpcOneShot := time.AfterFunc(cfg.Timeout, func() {
		logger.Fatalf("RPC service did not start within %s",
			cfg.Timeout.String())
	})
	for {
		time.Sleep(time.Second)
		_, err := rpc.DialHTTP("tcp", cfg.ControlAddress)
		if err == nil {
			break
		}
	}
	rpcOneShot.Stop()

	// Start the deliver clients. Once they have all finished seeking, we mark
	// the start of the run and release them.

	if cfg.Dclients != 0 {

		for server := 0; server < cfg.NumDservers; server++ {
			for channel := 0; channel < cfg.Channels; channel++ {
				for client := 0; client < cfg.Dclients; client++ {
					cmd := exec.Command(
						os.Args[0],
						"deliver",
						cfg.ControlAddress,
						strconv.Itoa(server),
						strconv.Itoa(channel),
						strconv.Itoa(client),
					)
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					err = cmd.Start()
					if err != nil {
						logger.Fatalf("Deliver client start failure: %s", err)
					}
				}
			}
		}

		startOneShot := time.AfterFunc(cfg.Timeout, func() {
			logger.Fatalf("Deliver clients did not synchronize within %s",
				cfg.Timeout.String())
		})
		control.startWG.Wait()
		startOneShot.Stop()
	}

	stats.Tstart = time.Now()
	control.releaseWG.Done()

	// Start the broadcast clients, and wait for completion.

	if cfg.Broadcast {

		for server := 0; server < cfg.NumBservers; server++ {
			for channel := 0; channel < cfg.Channels; channel++ {
				for client := 0; client < cfg.Bclients; client++ {
					cmd := exec.Command(
						os.Args[0],
						"broadcast",
						cfg.ControlAddress,
						strconv.Itoa(server),
						strconv.Itoa(channel),
						strconv.Itoa(client),
					)
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					err = cmd.Start()
					if err != nil {
						logger.Fatalf("Broadcast client start failure: %s", err)
					}
				}
			}
		}

		control.broadcastWG.Wait()
		stats.DbroadcastAll = time.Since(stats.Tstart).Seconds()
	}

	// Nothing to do now but wait for delivery to complete, and print
	// statistics. Note that deliver clients also do error checking, so their
	// elapsed times are communicated back through the DeliverDone RPC.

	control.deliverWG.Wait()
	stats.report(cfg)

	if (stats.Missing != 0) || (stats.WrongChannel != 0) {
		logger.Fatalf("Aborting due to missing TX and/or channel errors")
	}
}
