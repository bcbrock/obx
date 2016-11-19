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
	"sort"
	"time"
)

// Stats stores times and durations for performance reporting.
type Stats struct {
	Tstart        time.Time     // The time all broadcast clients start
	DbroadcastAll float64       // The duration of all broadcast clients
	DdeliverAll   float64       // The duration of all deliver clients
	Dbroadcast    [][][]float64 // The duration of each broadcast client
	Ddeliver      [][][]float64 // The duration of each deliver client
	Missing       uint64		// The composite # of missing TX
	WrongChannel  uint64		// The composite # of TX on the wrong channel
}

// newStats initializes a Stats object.
func newStats(cfg *Config) *Stats {

	s := &Stats{}

	s.Dbroadcast = make([][][]float64, cfg.NumBservers)
	for server := 0; server < cfg.NumBservers; server++ {
		s.Dbroadcast[server] = make([][]float64, cfg.Channels)
		for channel := 0; channel < cfg.Channels; channel++ {
			s.Dbroadcast[server][channel] = make([]float64, cfg.Bclients)
		}
	}

	s.Ddeliver = make([][][]float64, cfg.NumDservers)
	for server := 0; server < cfg.NumDservers; server++ {
		s.Ddeliver[server] = make([][]float64, cfg.Channels)
		for channel := 0; channel < cfg.Channels; channel++ {
			s.Ddeliver[server][channel] = make([]float64, cfg.Dclients)
		}
	}

	return s
}

// report prints the performance report.
func (s *Stats) report(cfg *Config) {

	fmt.Printf("****************************************************************************\n")
	fmt.Printf("* OBX Report                                                               *\n")
	fmt.Printf("****************************************************************************\n")

	// Report configuration and summary information

	tpsb := float64(cfg.TotalTxBroadcast) / s.DbroadcastAll
	tpsd := float64(cfg.TotalTxDelivered) / s.DdeliverAll
	bpsb := float64(cfg.TotalBytesBroadcast) / s.DbroadcastAll
	bpsd := float64(cfg.TotalBytesDelivered) / s.DdeliverAll

	fmt.Printf("Configuration\n")
	fmt.Printf("    Broadcast Servers	   : %d: %v\n", cfg.NumBservers, cfg.Bservers)
	fmt.Printf("    Broadcast Clients	   : %d\n", cfg.Bclients)
	fmt.Printf("    Deliver Servers  	   : %d: %v\n", cfg.NumDservers, cfg.Dservers)
	fmt.Printf("    Deliver Clients  	   : %d\n", cfg.Dclients)
	fmt.Printf("    Channels         	   : %d\n", cfg.Channels)
	fmt.Printf("    Transactions     	   : %d\n", cfg.Transactions)
	fmt.Printf("    Payload          	   : %d\n", cfg.Payload)
	fmt.Printf("    Burst            	   : %d\n", cfg.Burst)
	fmt.Printf("    Delay            	   : %s\n", cfg.Delay.String())
	fmt.Printf("    Window           	   : %d\n", cfg.Window)
	fmt.Printf("    AckEvery         	   : %d\n", cfg.AckEvery)
	fmt.Printf("    Broadcast?         	   : %v\n", cfg.Broadcast)

	if cfg.Broadcast {
		fmt.Printf("****************************************************************************\n")
		fmt.Printf("Broadcast Statistics\n")
		fmt.Printf("    Broadcast Duration     : %0.3f seconds\n", s.DbroadcastAll)
		fmt.Printf("    Tx Broadcast           : %s\n", commafy(int64(cfg.TotalTxBroadcast)))
		fmt.Printf("    Tx Broadcast Rate      : %s TPS\n", commafy(int64(tpsb)))
		fmt.Printf("    Payload Bytes Broadcast: %s\n", commafy(int64(cfg.TotalBytesBroadcast)))
		fmt.Printf("    Payload Broadcast Rate : %s BPS\n", commafy(int64(bpsb)))
	}

	if cfg.Dclients != 0 {
		fmt.Printf("****************************************************************************\n")
		fmt.Printf("Deliver Statistics\n")
		fmt.Printf("    Deliver Duration       : %0.3f seconds\n", s.DdeliverAll)
		fmt.Printf("    Tx Delivered           : %s\n", commafy(int64(cfg.TotalTxDelivered)))
		fmt.Printf("    Tx Delivery Rate       : %s TPS\n", commafy(int64(tpsd)))
		fmt.Printf("    Payload Bytes Delivered: %s\n", commafy(int64(cfg.TotalBytesDelivered)))
		fmt.Printf("    Payload Delivery Rate  : %s BPS\n", commafy(int64(bpsd)))
	}
	// Report broadcast percentiles

	if cfg.Broadcast {

		fmt.Printf("****************************************************************************\n")

		bDuration := make([]float64, cfg.TotalBroadcastClients)
		bTPS := make([]float64, cfg.TotalBroadcastClients)
		bBPS := make([]float64, cfg.TotalBroadcastClients)

		var index int
		for server := 0; server < cfg.NumBservers; server++ {
			for channel := 0; channel < cfg.Channels; channel++ {
				for client := 0; client < cfg.Bclients; client++ {
					bDuration[index] = s.Dbroadcast[server][channel][client]
					bTPS[index] = float64(cfg.TxBroadcastPerClient) / bDuration[index]
					bBPS[index] = float64(cfg.BytesBroadcastPerClient) / bDuration[index]
					index++
				}
			}
		}

		dBest, dMedian, d90, d95, dWorst := percentiles(bDuration, 1)
		tBest, tMedian, t90, t95, tWorst := percentiles(bTPS, -1)
		bBest, bMedian, b90, b95, bWorst := percentiles(bBPS, -1)

		fmt.Printf("Broadcast Clients  :       Best     Median        90%%        95%%      Worst\n")
		fmt.Printf("    Duration Sec.  : %10.3f %10.3f %10.3f %10.3f %10.3f\n",
			dBest, dMedian, d90, d95, dWorst)
		fmt.Printf("    Tx Per Sec.    : %10s %10s %10s %10s %10s\n",
			commafy(int64(tBest)), commafy(int64(tMedian)), commafy(int64(t90)),
			commafy(int64(t95)), commafy(int64(tWorst)))
		fmt.Printf("    Bytes Per Sec. : %10s %10s %10s %10s %10s\n",
			commafy(int64(bBest)), commafy(int64(bMedian)), commafy(int64(b90)),
			commafy(int64(b95)), commafy(int64(bWorst)))
	}

	// Report delivery percentiles

	if cfg.Dclients != 0 {

		fmt.Printf("****************************************************************************\n")

		dDuration := make([]float64, cfg.TotalDeliverClients)
		dTPS := make([]float64, cfg.TotalDeliverClients)
		dBPS := make([]float64, cfg.TotalDeliverClients)

		var index int
		for server := 0; server < cfg.NumDservers; server++ {
			for channel := 0; channel < cfg.Channels; channel++ {
				for client := 0; client < cfg.Dclients; client++ {
					dDuration[index] = s.Ddeliver[server][channel][client]
					dTPS[index] = float64(cfg.TxDeliveredPerClient) / dDuration[index]
					dBPS[index] = float64(cfg.BytesDeliveredPerClient) / dDuration[index]
					index++
				}
			}
		}

		dBest, dMedian, d90, d95, dWorst := percentiles(dDuration, 1)
		tBest, tMedian, t90, t95, tWorst := percentiles(dTPS, -1)
		bBest, bMedian, b90, b95, bWorst := percentiles(dBPS, -1)

		fmt.Printf("Deliver Clients    :       Best     Median      90%%        95%%      Worst\n")
		fmt.Printf("    Duration Sec.  : %10.3f %10.3f %10.3f %10.3f %10.3f\n",
			dBest, dMedian, d90, d95, dWorst)
		fmt.Printf("    Tx Per Sec.    : %10s %10s %10s %10s %10s\n",
			commafy(int64(tBest)), commafy(int64(tMedian)), commafy(int64(t90)),
			commafy(int64(t95)), commafy(int64(tWorst)))
		fmt.Printf("    Bytes Per Sec. : %10s %10s %10s %10s %10s\n",
			commafy(int64(bBest)), commafy(int64(bMedian)), commafy(int64(b90)),
			commafy(int64(b95)), commafy(int64(bWorst)))
	}

	fmt.Printf("****************************************************************************\n")
}

// Compute best, median, 90th and 95th percentiles and worst case from a slice
// of float64s. If the direction is negative, we sort in decreasing order.
func percentiles(in []float64, direction int) (best, median, p90, p95, worst float64) {
	if direction >= 0 {
		sort.Sort(sort.Float64Slice(in))
	} else {
		sort.Sort(sort.Reverse(sort.Float64Slice(in)))
	}
	best = in[0]
	median = in[int(.5*float64(len(in)))]
	p90 = in[int(.9*float64(len(in)))]
	p95 = in[int(.95*float64(len(in)))]
	worst = in[len(in)-1]
	return
}

// commafy adds American-style comma separators to an integer. We should
// consider adding an open-source Go package that does things like this more
// generally.
func commafy(n int64) string {
	if n == 0 {
		return "0"
	}
	sDigits := map[int]string{
		0: "0",
		1: "1", -1: "1",
		2: "2", -2: "2",
		3: "3", -3: "3",
		4: "4", -4: "4",
		5: "5", -5: "5",
		6: "6", -6: "6",
		7: "7", -7: "7",
		8: "8", -8: "8",
		9: "9", -9: "9"}
	var sign string
	if n < 0 {
		sign = "-"
	} else {
		sign = ""
	}
	nDigits := 0
	s := ""
	for n != 0 {
		if nDigits == 3 {
			s = "," + s
			nDigits = 0
		}
		nDigits++
		s = sDigits[int(n%10)] + s
		n = n / 10
	}
	return sign + s
}
