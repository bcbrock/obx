Observations made during testing and development of **obx**. They may or may
not remain valid over time.

<a name="GoroutinesVsProcesses"></a>
# Goroutines vs. Processes

November 11, 2016

The **obx** application is written in Go, which may lead one to wonder why
**obx** spawns the clients as processes rather than simply using
goroutines. The short answer is consistent, and consistently higher
performance.

The original implementation of **obx** _did_ use goroutines. However we
noticed that the throughput results were highly variable. Experiments running
multiple copies of the early **obx** instances against a common ledger showed
that variability was reduced and throughput was significantly increased the
more independent **obx** client processes were running. The performance
difference is stunning - in one case we observed almost 3X the system-level
throughput of the current **obx** implementation vs. the old goroutine-based
code when running both broadcast and deliver clients. In a deliver-only
scenario the goroutine-based code did better, but the multi-process code still
showed 14% better throughput with about half of the client-to-client
variability in performance compared to the goroutine implementation.

This result raises some obvious concerns. It suggests (but certainly does not
prove) that by relying solely on goroutines for parallelism in Hyperledger
fabric (HLF) applications, we may be leaving significant amounts of
performance on the table. It isn't clear yet whether this effect is due to the
way that gRPC over HTTP/2 is implemented, or if it suggests that we may need
to take a hard look at the heuristics and overhead of the current goroutine
scheduler as it relates to HLF applications.


# Observing Storage I/O Effects

November 10, 2016

A typical run of **obx** on a large system may not demonstrate the impact of
I/O bandwidth on performance. Since transaction streams are being
simultaneously broadcast and delivered, it is most likely that all of the data
for delivery is being sourced from the kernel caches rather than from
external storage.

To see the effects (if any) of storage bandwith it is possible to do a first run
normally to set up the ledger, do a second deliver-only run for throughput
measurement (_-broadcast=false_), flush the kernels caches, and finally do
a third deliver-only run to compare against the run with hot caches.

Linux kernel caches can be flushed using the following sequence (documented 
[here](http://www.kernel.org/doc/Documentation/sysctl/vm.txt)):


```
sudo sync
sudo echo 3 > /proc/sys/vm/drop_caches
```

