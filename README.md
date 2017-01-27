# Introduction

**obx** is an _Ordering_ service _Benchmarking_ and _eXerciser_ application
for the [Hyperledger fabric](https://github.com/hyperledger/fabric)
(HLF). This is a simple application (a glorified script, really) for
performance and correctness testing of the HLF _ordering services_. Services
is plural here because there are currently several ordering servers defined in
the HLF codebase that share a common protocol interface exercised by this
application. **obx** is designed to stress and measure both throughput and
latency of the ordering service.

Some observations on the implementation and use of **obx** can be found
[here](observations.md).

**obx** is currently up-to-date and works with the following Hyperledger
fabric commit: 
```
commit 438700e6a2f05aa1092b58fe87c75fc7cbccacbe
Author: Artem Barger <bartem@il.ibm.com>
Date:   Thu Jan 26 13:36:19 2017 -0500

    [FAB-1872]: Commit genessis block, joining chain.
```

# Installation

This application requires that the
[Hyperledger fabric](https://github.com/hyperledger/fabric)
is installed in your **GOPATH**. The simplest way to install the application
is to clone this repository (or your fork) directly into your Hyperledger fabric
tree:

```
cd $GOPATH/src/github.com/hyperlegder/fabric/orderer/sample_clients
git clone https://github.com/bcbrock/obx
cd obx
go build     # Or 'go install' if you prefer
```

You could also simply

```
go get -u github.com/bcbrock/obx
```

however in this case you would also need to explicitly `go get` several other
packages that are currently "vendored" in the Hyperledger fabric codebase. The
Go compiler will tell you which packages you need to add should you elect to
build **obx** this way.

This application does not provide any support for configuring any of the
several available ordering services. Once you have an ordering service
running, you simply provide **obx** with the network addresses of your
ordering servers using command-line arguments.

# Theory of Operation

This **obx** application runs as a _control_ process, which then creates
multiple client processes that exercise the _broadcast_ and _deliver_
interfaces of the ordering service. The application assumes that ordering
servers are already up and running for broadcast and delivery. You can name
different sets of servers for broadcast and delivery, as long as they share a
common raw ledger.

The application will create a number of _channels_ and _clients_ based on the
command line parameters, and each broadcast client will send a number of
_transactions_ to each broadcast server for each channel. Each deliver client
will target one delivery server, and use that server to deliver all
transactions for a channel. The transaction payload size is parameterizable.

The total number of clients and transactions can be computed as simple
cross-products. For example, if you:

* Name 2 broadcast servers;

* Name 3 delivery servers;

* Request 4 channels;

* Request 5 broadcast clients per server/channel;

* Request 6 deliver clients per server/channel;

* Request 7000 transactions per channel

then 

* (2 * 4 * 5) = 40 broadcast clients are created, each of which will
  broadcast 7000 transactions to one server for one channel;
  
* (3 * 4 * 6) = 72 deliver clients are created, each of which will
  deliver all transactions (2 * 5 * 7000) for a channel from one server
  
Each client is currently implemented as a separate process (see
[Observations](observations.md#GoroutinesVsProcesses)), which is an instance
of the **obx** executable. If something happens that causes this multitude of
processes to not terminate, the system can be cleaned up by executing `pkill
obx`.

Each broadcast client runs until it has discharged its obligation to broadcast
a fixed number of transactions, and each deliver client runs until it has
delivered its required number of transactions. The transactions are tagged
with their originating client and timestamps, allowing the **obx** delivery
clients to verify that they are receiving the expected transactions.

At the end of the run the application prints some performance statistics. You
may also find it interesting to run real-time performance monitoring and
visualization tools such as
[viz_dstat](https://github.com/jschaub30/viz_dstat).

## Ordering Network Setup

Under normal circumstances, **obx** must be used with a freshly
instantiated ordering service that is not being used for any other purpose, in
order to make sure that the transactions being broadcast are the ones actually
being delivered.  We may be able to relax this requirement in the future.

Having said that, **obx** supports the `-broadcast=false` mode which allows
**obx** to be used to test the delivery side only for a precomputed ledger. See
the documentation of [-broadcast](#-broadcast) for details. 

Broadcast-only mode is also possible by setting `-dClients=0`.

# Usage

**obx** is executed as

```
obx ?... args ?...
```

where the optional arguments are processed by the Go
[flag](https://golang.org/pkg/flag) package.

## Required Parameters

* _-bServers_ A comma-separated list of broadcast server addresses. If the
  delivery servers (_-dServers_) are not explicitly specified, then the
  broadcast servers are used for delivery as well.

## Optional Parameters

* _-controlAddress_ The network address used by the controlling process to
  communicate with the clients, defaulting to `localhost:4000`.

* _-dServers_ A comma-separated list of delivery server addresses. If the
  delivery servers are not explicitly specified, then the broadcast servers
  (_-bServers_) are used for delivery as well.

* _-bClients_ The number of broadcast clients, defaulting to 1.

* _-dClients_ The number of deliver clients, defaulting to 1.

* _-channels_ The number of channels, defaulting to 1.

* _-transactions_ The number of transactions broadcast per client, per
  channel, per broadcast server, defaulting to 1. Note these are
  _transactions_, not _blocks_. Block formation is controlled by the
  parameterization of the orderer and the broadcast rate.

* _-payload_ The size of the transaction payload in bytes.  The default (and
  minimum) is currently the 58 bytes required for origin recording and latency
  measurements. Note that performance reports list throughput in payload-bytes
  per second. The actual network bandwidth requirement is higher due to block
  overhead such as hashes, metadata, and serialization overhead.

* _-burst_ -

* _-delay_ When broadcasting, the client bursts _-burst_ transactions
  back-to-back (default 1), then waits for the _-delay_ time before sending
  the next burst. The delay must be specified in a form understood by
  [time.ParseDuration()](https://golang.org/pkg/time/#ParseDuration), and
  defaults to 0.

* _-window_ -

* _-ackEvery_ The _-window_ specifies the number of blocks that can be
  delivered without an ACK, defaulting to 100. The _-ackEvery_ parameter
  specifies the frequency of ACKs to the delivery service. This value must be
  less than or equal to _-window_, and defaults to 70.

* _-timeout_ This is a synchronization timeout used to break hangs that may
  occur if bugs or other issues impede startup. The timeout must be specified
  in a form understood by
  [time.ParseDuration()](https://golang.org/pkg/time/#ParseDuration), and
  defaults to 30s.
  
* _-latencyAll_ -

* _-latencyDir_ -

* _-latencyPrefix_ If _-latencyDir_ is specified, then each deliver client
  that completes successfully will write a CSV file in this directory
  containing latency statistics for that client. The first line of the file
  contains the field names. The _-latencyDir_ must exist. The files are named
  \<latency prefix\>.\<Server\>.\<Channel\>.\<Client\>.csv. The default for
  the \<latency prefix\> is "client", but this prefix can be changed with the
  _-latencyPrefix_ option. The times recorded in the latency files are
  floating-point seconds since the start of timing, and delta-times are also
  in seconds. Times are recorded at a nanosecond resolution.
  
  By default only the block number, number of transactions in the block, block
  delivery time, and the minimum and maximum latency for each block are
  reported. Specify _-latencyAll=true_ to obtain reports that include data for
  every transaction in every block.
  
* _-controlLogging_ -

* _-broadcastLogging_ -

* _-deliverLogging_ -

* _-logLevel_ The logging level is a case-insensitive string chosen from
  `debug`, `info`, `note`, `warning` and `error`. The default is `info`. The
  logging level (_-logLevel_) normally applies to all of the control,
  broadcast and deliver processes, but the logging level for each process type
  can also be specified independently using the eponymous flag.

<a name="-broadcast"></a>

* _-broadcast_ This is a Boolean variable, defaulting to `true`. If
  `_-broadcast=false`, then no broadcast clients are actually created, however
  all of the broadcast client setup is used to inform the delivery clients how
  many transactions they need to deliver. This option is typically used by
  doing a first run against a fresh ledger with _-broadcast=true_ (the
  default), then subsequent runs with the same parameters except for setting
  _-broadcast=false_

# Examples

```
 # Broadcast/deliver a single transaction
 obx -bServers orderer:5151
 
 # Run 16 broadcast and 64 deliver clients against each of 3 servers, sending
 # 100K x 1K payloads to 10 channels. Also get block latency statistic reports.
 mkdir latency
 obx \
	-bServers bcast0:5151,bcast1:5151,bcast2:5151 \
	-dServers dlvr0:5151,dlvr1:5151,dlvr2:5151 \
	-bClients 16 -dClients 64 -channels 10 \
	-payload 1000 -transactions 100000 \
	-latencyDir latency
	
```

# Bugs

* Multiple channels are currently not supported - the ordering servers don't
  implement them yet.

# Todo

These should be considered musings rather than commitments.

* Fix bugs.

* Implement more in the area of latency statistics. For example, statistics on
  the latency of broadcast-to-ack. 

* Modify **obx** to work against non-fresh ledgers.

* Implement an option to run the clients as goroutines, either in a single
  process, or in multiple processes each of which run several goroutines. For
  example it might be more realistic to have a broadcast and deliver client in
  the same process.

* Implement dynamic broadcast throttling that matches the broadcast and
  delivery rates in an attempt to find the true peak steady-state throughput
  of the service under the payload assumptions.

# License and Origin

The code is licensed under the Apache License Version 2.0 - a copy of which
appears in this directory - with Copyright held by IBM. This code was
originally developed based on Kostas Christidis'
[bd_counter](https://github.com/hyperledger/fabric/orderer/sample_clients/bd_counter)
example from the HLF which was licensed under identical terms.


