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
	"flag"
	"strings"
	"time"
)

// Config is the configuration structure for obx.
type Config struct {

	// These fields come from the obx command line

	ControlAddress   string        // Control process IP address
	Broadcast        bool          // Will we actually broadcast or just deliver?
	Bclients         int           // # of broadcast clients
	Dclients         int           // # of deliver clients
	Channels         int           // # of channels
	NumBservers      int           // # of broadcast servers
	NumDservers      int           // # of deliver servers
	Bservers         []string      // # IP:PORT of broadcast servers
	Dservers         []string      // # IP:PORT of deliver servers
	Transactions     int           // # of transactions per server per client
	Payload          int           // Payload size in bytes
	Burst            int           // # of transactions in a burst
	Delay            time.Duration // Broadcast client delay between bursts
	Window           int           // # of blocks that can be delivered w/o ACK
	AckEvery         int           // Deliver clients ack every (this many) blocks
	Timeout          time.Duration // Initializtion timeout
	LatencyAll       bool          // Print all latencies (vs. block latencies)?
	LatencyDir       string        // Directory for latency files
	LatencyPrefix    string        // Prefix for latency file names
	ControlLogging   string        // Control application logging level
	BroadcastLogging string        // Broadcast application logging level
	DeliverLogging   string        // Deliver application logging level

	// These fields cache simple computations for convenience

	TotalBroadcastClients   uint64 // The total # of broadcast clients
	TxBroadcastPerClient    uint64 // # of TX broadcast by each delivery client
	BytesBroadcastPerClient uint64 // Payload bytes broadcast by each broadcast client
	TotalTxBroadcast        uint64 // The total # of Tx Broadcast
	TotalBytesBroadcast     uint64 // Payload bytes (including headers) broadcast

	TotalDeliverClients     uint64 // The total # of deliver clients
	TxDeliveredPerClient    uint64 // # of TX delivered to each delivery client
	BytesDeliveredPerClient uint64 // Payload bytes delivered to each delivery client
	TotalTxDelivered        uint64 // The total # of Tx Delivered
	TotalBytesDelivered     uint64 // Payload bytes (including headers) delivered
}

func bogus(flag string, why string) {
	logger.Fatalf("The value for the flag -%s must be %s.\n", flag, why)
}

func requireUint16(flag string, val int) {
	if (val < 0) || (val > 0x7fff) {
		bogus(flag, "representable as an unsigned 16-bit number")
	}
}

func requireUint32(flag string, val int) {
	if (val < 0) || (val > 0x7fffffff) {
		bogus(flag, "representable as an unsigned 32-bit number")
	}
}

func requireNonEmpty(flag string, val string) {
	if val == "" {
		bogus(flag, "a non-empty string")
	}
}

func requirePosInt(flag string, val int) {
	if val < 0 {
		bogus(flag, "a positive integer")
	}
}

func requirePosDuration(flag string, val time.Duration) {
	if val < 0 {
		bogus(flag, "a positive duration")
	}
}

func requireLE(flag1, flag2 string, val1, val2 int) {
	if val1 > val2 {
		bogus(flag1, "less than or equal to the value of -"+flag2)
	}
}

// Parse and validate the command-line flags and create the configuration.
func newConfig() *Config {

	c := &Config{}
	var logLevel, bServers, dServers string

	flag.StringVar(&c.ControlAddress, "controlAddress", "localhost:4000",
		"Control process IP address, default localhost:4000")

	flag.BoolVar(&c.Broadcast, "broadcast", true,
		"Set to false to squash actual broadcast.")

	flag.IntVar(&c.Bclients, "bClients", 1,
		"The number of broadcast clients; Default 1")

	flag.IntVar(&c.Dclients, "dClients", 1,
		"The number of deliver clients; Default 1")

	flag.IntVar(&c.Channels, "channels", 1,
		"The number of channels; Default 1")

	flag.StringVar(&bServers, "bServers", "",
		"A comma-separated list of IP:PORT of broadcast servers to target; Required")

	flag.StringVar(&dServers, "dServers", "",
		"A comma-separated list of IP:PORT of deliver servers to target; Defaults to broadcast szervers")

	flag.IntVar(&c.Transactions, "transactions", 1,
		"The number of transactions broadcast to each client's servers; Default 1")

	flag.IntVar(&c.Payload, "payload", TxHeaderSize,
		"Payload size in bytes; Minimum/default is the performance header size (56 bytes)")

	flag.IntVar(&c.Burst, "burst", 1,
		"The number of transactions burst to each server during broadcast; Dafault 1")

	flag.DurationVar(&c.Delay, "delay", 0,
		"The delay between bursts, in the form required by time.ParseDuration(); Default is no delay")

	flag.IntVar(&c.Window, "window", 100,
		"The number of blocks allowed to be delivered without an ACK; Default 100")

	flag.IntVar(&c.AckEvery, "ackEvery", 70,
		"The deliver client will ACK every (this many) blocks; Default 70")

	flag.DurationVar(&c.Timeout, "timeout", 30*time.Second,
		"The initialization timeout, in the form required by time.ParseDuration(); Default 30s")

	flag.BoolVar(&c.LatencyAll, "latencyAll", false,
		"By default, only block latencies are reported. Set -latencyAll=true to report all transaction latencies")

	flag.StringVar(&c.LatencyDir, "latencyDir", "",
		"The directory to contain latency files; These files are only created if -latencyDir is specified")

	flag.StringVar(&c.LatencyPrefix, "latencyPrefix", "client",
		"Prefix for latency file names")

	flag.StringVar(&logLevel, "logLevel", "info",
		"The global logging level; Default 'info'")

	flag.StringVar(&c.ControlLogging, "controlLogging", "",
		"Override logging level for the 'control' process")

	flag.StringVar(&c.BroadcastLogging, "broadcastLogging", "",
		"Override logging level for the 'broadcast' processes")

	flag.StringVar(&c.DeliverLogging, "deliverLogging", "",
		"Override logging level for the 'deliver' processes")

	flag.Parse()

	if c.ControlLogging == "" {
		c.ControlLogging = logLevel
	}
	if c.BroadcastLogging == "" {
		c.BroadcastLogging = logLevel
	}
	if c.DeliverLogging == "" {
		c.DeliverLogging = logLevel
	}

	initLogging(c.ControlLogging)

	requireUint16("bclients", c.Bclients)
	requireUint16("dclients", c.Dclients)
	requireUint16("channels", c.Channels)
	requireNonEmpty("bServers", bServers)
	if dServers == "" {
		dServers = bServers
	}
	requireUint32("transactions", c.Transactions)
	requirePosInt("payload", c.Payload)
	if c.Payload < TxHeaderSize {
		logger.Infof("Payload size will be set to the default (%d bytes)\n",
			TxHeaderSize)
		c.Payload = TxHeaderSize
	}
	requirePosInt("burst", c.Burst)
	requirePosDuration("delay", c.Delay)
	requirePosInt("window", c.Window)
	requirePosInt("ackevery", c.AckEvery)
	requireLE("ackevery", "window", c.AckEvery, c.Window)
	requirePosDuration("timeout", c.Timeout)

	c.Bservers = strings.Split(bServers, ",")
	c.NumBservers = len(c.Bservers)

	c.Dservers = strings.Split(dServers, ",")
	c.NumDservers = len(c.Dservers)

	logger.Infof("Configuration")
	logger.Infof("    Broadcast Servers: %d: %v", c.NumBservers, c.Bservers)
	logger.Infof("    Broadcast Clients: %d", c.Bclients)
	logger.Infof("    Deliver Servers  : %d: %v", c.NumDservers, c.Dservers)
	logger.Infof("    Deliver Clients  : %d", c.Dclients)
	logger.Infof("    Channels         : %d", c.Channels)
	logger.Infof("    Transactions     : %d", c.Transactions)
	logger.Infof("    Payload          : %d", c.Payload)
	logger.Infof("    Burst            : %d", c.Burst)
	logger.Infof("    Delay            : %s", c.Delay.String())
	logger.Infof("    Window           : %d", c.Window)
	logger.Infof("    AckEvery         : %d", c.AckEvery)
	logger.Infof("    Broadcast?       : %v", c.Broadcast)

	c.TotalBroadcastClients =
		uint64(c.NumBservers) * uint64(c.Channels) * uint64(c.Bclients)
	c.TxBroadcastPerClient = uint64(c.Transactions)
	c.BytesBroadcastPerClient = c.TxBroadcastPerClient * uint64(c.Payload)
	c.TotalTxBroadcast = uint64(c.TotalBroadcastClients) * c.TxBroadcastPerClient
	c.TotalBytesBroadcast = c.TotalTxBroadcast * uint64(c.Payload)

	c.TotalDeliverClients =
		uint64(c.NumDservers) * uint64(c.Channels) * uint64(c.Dclients)
	c.TxDeliveredPerClient =
		uint64(c.NumBservers) * uint64(c.Bclients) * uint64(c.Transactions)
	c.BytesDeliveredPerClient = c.TxDeliveredPerClient * uint64(c.Payload)
	c.TotalTxDelivered = c.TxDeliveredPerClient * c.TotalDeliverClients
	c.TotalBytesDelivered = c.TotalTxDelivered * uint64(c.Payload)

	return c
}
