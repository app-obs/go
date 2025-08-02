# Running the Example

This directory contains a complete, runnable example of how to use the `go-observability` library.

The `main.go` file is configured via environment variables. Below are instructions for running it in several common scenarios.

---

### Scenario 1: No APM Backend (`APMType="none"`)

This is the simplest way to run the example. It requires no external dependencies and will output structured JSON logs to your console. Tracing will be disabled.

**Command:**
```sh
go run .
```
*(This works because `APMType="none"` is the default in `main.go`)*

---

### Scenario 2: With a Generic OTLP Collector

If you have any OpenTelemetry collector running that is accessible from your machine.

**Prerequisites:**
- An OTLP collector listening for HTTP connections (e.g., on port `4318`).

**Command:**

Replace `<your-collector-host>` with the actual hostname or IP of your collector.

```sh
APM_TYPE="otlp" APM_URL="http://<your-collector-host>:4318" go run .
```

---

### Scenario 3: With the Example Observability Server (Tempo)

This is the recommended setup for a rich local development experience.

**Prerequisites:**
- The [example-observability-server](https://github.com/app-obs/example-observability-server) must be running.

**Command:**
```sh
APM_TYPE="otlp" APM_URL="http://localhost:4318" go run .
```

After running the command, send a request to the example service:
```sh
curl http://localhost:8080/hello
```

You can then view the trace for your request in the Grafana UI at [http://localhost:3000](http://localhost:3000). Navigate using the left-hand menu: `Drilldown -> Traces`, then click the "Traces" tab near the bottom of the main panel to see a list of recent traces.

---

### Scenario 4: With a Datadog Agent

If you are a Datadog user and have the Datadog Agent running locally.

**Prerequisites:**
- A Datadog Agent running on your local machine.

**Command:**

The library will connect to the default agent address (`localhost:8126`).

```sh
APM_TYPE="datadog" go run .
```

You can then view the trace in your Datadog APM dashboard.
