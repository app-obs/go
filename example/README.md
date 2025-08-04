# Running the Example

This directory contains a complete, runnable example of how to use the `go-observability` library.

The `main.go` file is now hardcoded to send traces and metrics to a local OpenTelemetry collector for demonstration purposes. However, the library is designed to be configured via environment variables, which will always override the settings in the code.

---

### Scenario 1: Running the Default OTLP Example

This is the primary way to run the example. It will send traces and metrics to a local OTLP collector.

**Prerequisites:**
- An OTLP collector listening for HTTP connections (e.g., on port `4318`). The [example-observability-server](https://github.com/app-obs/example-observability-server) is the recommended way to do this.

**Command:**
```sh
go run .
```

After running the command, send a request to the example service:
```sh
curl http://localhost:8080/hello
```

You can then view the trace for your request in Grafana (if using the example server) at [http://localhost:3000](http://localhost:3000).

---

### Scenario 2: Overriding with Environment Variables (e.g., Datadog)

You can override the in-code configuration by setting environment variables. This example shows how to switch to the Datadog APM.

**Prerequisites:**
- A Datadog Agent running on your local machine.

**Command:**

This command will disable metrics (since Datadog is not an OTLP provider) and point the APM to the Datadog agent.

```sh
OBS_APM_TYPE="datadog" OBS_METRICS_TYPE="none" go run .
```

You can then view the trace in your Datadog APM dashboard.

---

### Scenario 3: Disabling APM and Metrics

To run the example with only structured logging, you can disable both APM and metrics.

**Command:**
```sh
OBS_APM_TYPE="none" OBS_METRICS_TYPE="none" go run .
```
This will output structured JSON logs to your console, and tracing/metrics will be disabled.