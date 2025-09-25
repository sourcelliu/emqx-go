# Phase 0: Baseline Performance Metrics

As Jules, I cannot execute tests on the original Erlang-based EMQX. To establish a performance baseline for the Go rewrite, please perform the following benchmark tests using the `emqtt-bench` tool against a running instance of the original EMQX broker.

## Prerequisites

1.  A running EMQX 5.0 instance.
2.  The `emqtt-bench` tool installed and available in your `PATH`. You can find it here: [https://github.com/emqx/emqtt-bench](https://github.com/emqx/emqtt-bench).

## Test 1: Connection Rate

This test measures the maximum rate at which clients can connect to the broker.

**Command:**

```bash
emqtt-bench conn -c 500 -i 10ms
```

**To Execute:**

1.  Run the command above.
2.  Let it run for approximately 60 seconds.
3.  Copy the final output summary.
4.  Paste the results below.

**Results:**

```
<Please paste the emqtt-bench conn output here>
```

## Test 2: Message Throughput (Publish)

This test measures the message publishing rate.

**Command:**

```bash
emqtt-bench pub -t perf/test -s 128 -c 500 -i 10ms
```

**To Execute:**

1.  Run the command above.
2.  Let it run for approximately 60 seconds.
3.  Copy the final output summary.
4.  Paste the results below.

**Results:**

```
<Please paste the emqtt-bench pub output here>
```

## Test 3: Message Latency (Subscribe/Publish)

This test measures the end-to-end latency for messages.

**Command:**

```bash
emqtt-bench sub -t perf/test -c 500
# In a separate terminal, run:
emqtt-bench pub -t perf/test -s 128 -c 1 -i 1s
```

**To Execute:**

1.  Run the `sub` command first.
2.  Run the `pub` command to start publishing messages.
3.  Observe the latency metrics from the `sub` command's output.
4.  Paste a sample of the latency output below.

**Results:**

```
<Please paste a sample of the emqtt-bench sub latency output here>
```