# Twitch IRC Ingest Pipeline

A Go/Redpanda backbone for real-time ingestion of Twitch chat into Kafka. This service forms the ingestion layer of a scalable analytics and ML pipeline, providing high-quality streaming data for downstream processing.

## Overview

This repository contains a backend streaming system that connects to Twitch’s IRC interface, manages channel subscriptions, and publishes structured chat events into Kafka (via Redpanda). It acts as the ingestion spine of a larger data and machine-learning stack, responsible for the reliable, fault-tolerant capture and normalization of live chat data.

The system is composed of three focused services:

- **`irc_collector`** – Maintains a WebSocket connection to Twitch IRC, joins and parts channels on demand, parses IRC traffic into structured events, and writes those events to Kafka.
- **`oauth_server`** – Handles the Twitch OAuth2 authorization code flow, exchanges authorization codes for access tokens, and persists the token for the collector to use.
- **`kafka_consumer`** – A small diagnostic consumer that reads chat events from Kafka and prints them, verifying that ingestion is working end-to-end.

Channel membership is controlled via a lightweight HTTP API (`/join` and `/part`) and persisted in `channels.json`. A reconciler loop continuously compares the desired membership state to the actual IRC state and issues JOIN/PART commands with rate limiting, timeouts, and exponential backoff. All services expose health and readiness endpoints for monitoring.

## Architecture

The system is organized into two cooperating planes:

- **Data Plane** — maintains the live connection to Twitch IRC, parses messages, and streams structured events into Kafka.
- **Control Plane** — manages which channels the collector should join, persists the desired set, and reconciles it with observed IRC membership using a controller-style loop.

### Data Flow

```
Twitch IRC
   |
   |  (WebSocket: PASS/NICK/CAP, PRIVMSG, JOIN/PART, PING/PONG)
   v
[irc_collector]
   ├─ connector.go      (WebSocket dial + Twitch auth handshake)
   ├─ reader.go         (socket reader; handles PING → PONG)
   ├─ classifier.go     (parse IRC lines → structured events)
   ├─ writer.go         (JOIN/PART/PONG → socket)
   └─ main.go           (pipeline wiring via errgroup)
      |
      v
Kafka / Redpanda topic (e.g. "chat-messages")
      |
      v
[kafka_consumer]
   (prints events for validation)
```

`irc_collector` is built as a multi-stage concurrent pipeline. Each stage communicates over Go channels and is supervised by an `errgroup` to ensure coordinated shutdown when a stage errors or the process receives a signal. Parsed messages are converted into `PrivMsg` events (`internal/irc_events`) and published through a thin Kafka writer abstraction (`internal/kafka`).


### Control Plane

```
     HTTP client
       |
       |  GET /join?channel=chess
       |  GET /part?channel=chess
       v
  [internal/httpapi]
       |
       v
controlCh (types.IRCCommand)
       |
       v
[internal/channel_record.Controller]
   - persists desired channels to channels.json
   - emits snapshot updates
       |
       v
[internal/channel_record.Rectifier]
   - compares desired vs actual membership
   - rate-limits JOIN/PART commands
   - handles timeouts + exponential backoff
       |
       v
[internal/scheduler]
   - formats JOIN/PART lines for IRC
       |
       v
IRCWriter → Twitch IRC
```
The reconciler follows a pattern similar to Kubernetes controllers: desired state is persisted, observed state is fed in via membership events, and the loop drives the system toward convergence with rate-limiting and retry logic. This ensures channel membership stays correct even across disconnects, missed events, or network glitches.

### Authentication

The `oauth_server` runs independently and provides the access token used by the collector:

```
Browser → [oauth_server] → Twitch OAuth2 → token saved → used by irc_collector
```

The collector mounts the token file at runtime and uses it to authenticate its IRC session.


## Components

**cmd/irc_collector/**

The main ingestion service. It maintains the authenticated WebSocket connection to Twitch IRC, processes incoming IRC lines through a multi-stage concurrent pipeline (reader → classifier → rectifier → scheduler → writer), and publishes structured chat events into Kafka. All stages communicate via Go channels and are supervised with an errgroup for coordinated shutdown.

**cmd/oauth_server/**

A standalone HTTP server that implements Twitch’s OAuth2 authorization code flow. It exchanges authorization codes for access tokens, persists the resulting token file, and exposes health and readiness endpoints. The collector mounts this token file at runtime for authenticated IRC sessions.

**cmd/kafka_consumer/**

A diagnostic utility that consumes events from Kafka and prints them. Used to validate that live ingestion, parsing, and Kafka publication are functioning end-to-end.

**internal/channel_record/**

Responsible for desired channel state.
The Controller persists the set of desired channels to channels.json using atomic write-and-rename semantics and exposes immutable snapshots for safe concurrent access.
The Rectifier implements a controller loop: it reconciles desired state with observed IRC membership, applying rate limits, join/part timeouts, exponential backoff, and scheduled retries. Tests use a fake clock for deterministic verification of timing logic.

**internal/httpapi/**

Implements the control-plane HTTP interface (/join and /part). It validates requests, enqueues channel commands, and provides liveness/readiness probes for container orchestration. This is the public entrypoint for modifying which channels the collector follows.

**internal/kafka/**

Thin abstractions over kafka-go.
writer.go provides a configurable Kafka writer, while producer.go handles marshalling IRC events and publishing them to the configured Kafka topic. This decouples the ingest pipeline from the underlying Kafka client.

**internal/irc_events/**

Defines strongly-typed IRC event structures such as PrivMsg. These types act as the internal event schema and Kafka payload format, keeping raw IRC parsing separated from downstream consumers.

**internal/scheduler/**

Formats IRC control commands (JOIN, PART, and PONG responses) and forwards them to the IRC writer. This layer isolates command construction from connection-level concerns.

**internal/observe/**

Centralized structured logging built on slog.
Provides component-scoped loggers and environment-driven log levels, ensuring consistent observability across all services.

**internal/types/**

Pure data models shared across components, including account configuration, IRC command types, membership events, Kafka payloads, and channel-file schema definitions. These types define the contracts between subsystems.

---

## Getting Started (Docker Compose)

This guide walks you through running the Twitch IRC Ingest Pipeline from a clean clone to seeing live Twitch chat messages flowing through Kafka/Redpanda. Follow the steps in order with no guesswork.

---

### Prerequisites

You need:

- **Docker** and **Docker Compose**
- A **Twitch account**
- A **Twitch Developer Application** with:
  - `TWITCH_CLIENT_ID`
  - `TWITCH_CLIENT_SECRET`
  - Redirect URI set to:  
    `http://localhost:3000/callback`

---

### 1. Clone the Repository

```bash
git clone https://github.com/Jamie-38/twitch-irc-ingest-pipeline.git
cd twitch-irc-ingest-pipeline
```

---

### 2. Configure Environment and Account Files

#### Copy and edit the `.env` file

```bash
cp internal/templates/env.example .env
```

Edit `.env` and fill in:

- `TWITCH_CLIENT_ID`
- `TWITCH_CLIENT_SECRET`
- `TWITCH_REDIRECT_URI` (usually already correct)
- `HTTP_API_HOST`, `HTTP_API_PORT` (usually fine as-is)
- `KAFKA_BROKERS` (default: `redpanda:9092`)
- `KAFKA_TOPIC` (default: `chat-messages`)
- `LOG_LEVEL` (set to `DEBUG` for development)

#### Create account config

```bash
mkdir -p accounts
cp internal/templates/account.example.json accounts/account.config.json
```

Edit the file and set your Twitch login:

```json
{
  "accountname": "your_twitch_login",
  "username": "your_twitch_login"
}
```

#### Channels file (optional)

The system will create `internal/channel_record/channels.json` automatically.  
You do *not* need to create it manually.

---

### 3. Obtain a Twitch OAuth Token

You must authenticate with Twitch *before* running the full pipeline.

#### Start only the OAuth server and Redpanda

```bash
docker compose up --build oauth_server redpanda
```

When logs show the OAuth server is listening on port **3000**, open:

```
http://localhost:3000/
```

Then:

1. Click the “authenticate with Twitch” link.
2. Log in and approve the app.
3. When complete, the token is written to:

```
tokens/default.token.json
```

This file is mounted into the `irc_collector` container at runtime.

Press **Ctrl+C** to shut down once the token is saved.

---

### 4. Start the Full Pipeline

Now that authentication is done:

```bash
docker compose up --build
```

This starts:

- `redpanda` — Kafka-compatible message broker  
- `oauth_server` — now idle but healthy  
- `irc_collector` — Twitch IRC ingest pipeline  
- `kafka_consumer` — diagnostic consumer printing chat messages  

You can inspect logs with:

```bash
docker logs -f irc_collector
docker logs -f kafka_consumer
```

---

### 5. Join a Twitch Channel

Use the HTTP API to tell the collector which channels to join.

Example:

```bash
curl "http://localhost:6060/join?channel=chess"
```

The system will:

- Persist the channel in `channels.json`
- Reconcile desired vs actual IRC membership
- Rate-limit JOIN commands
- Issue a JOIN to Twitch

To part a channel:

```bash
curl "http://localhost:6060/part?channel=chess"
```

---

### 6. Verify Chat Messages Are Flowing

Open a terminal and follow the Kafka consumer logs:

```bash
docker logs -f kafka_consumer
```

Now send a message in the Twitch channel you joined.  
You should see output like:

```
message at topic/partition/offset chat-messages/0/42: <key> = {"UserID":"...","UserLogin":"...","ChannelID":"...","Text":"..."}
```

This confirms the end-to-end pipeline works:

**Twitch IRC → irc_collector → parsing + normalization → Kafka → kafka_consumer**

---

### 7. Stopping and Cleanup

Press **Ctrl+C** in the compose terminal to shut down containers.

To clean up containers but keep Redpanda volumes:

```bash
docker compose down
```

If you want to delete all stored data (including token and channels file):

```bash
docker compose down -v
```

## Testing

Run all tests:

``` bash
go test ./...
```

The tests are pure Go unit tests -- they do **not** require Twitch,
Kafka, or Docker to be running.

Key areas covered:

-   **IRC parsing (`cmd/irc_collector/classifier.go`)**
    -   Unit tests for parsing PRIVMSG/JOIN/PART lines, handling
        malformed input, and correctly extracting tags, prefixes, and
        trailing text.
    -   A fuzz test for the IRCv3 tag unescape function to ensure it
        never panics on arbitrary input.
-   **Channel reconciliation (`internal/channel_record`)**
    -   Tests for the controller-style reconciler that manages desired
        vs actual channel membership.
    -   Uses a **fake clock** to deterministically verify rate limiting,
        join/part timeouts, exponential backoff, and retry scheduling.
-   **HTTP control API (`internal/httpapi`)**
    -   Tests for `/join` and `/part`:
        -   Enqueued commands are lowercased and normalized with `#`.
        -   Missing or invalid parameters are rejected with
            `400 Bad Request`.
-   **OAuth handlers (`internal/oauth`)**
    -   Tests that the index page renders a valid Twitch auth URL using
        the configured client ID and redirect URI.
    -   Tests that the callback handler correctly rejects requests
        missing the `code` parameter.

What is **not** currently covered by tests:

-   The WebSocket connector, IRC reader/writer, and Kafka writer are
    exercised indirectly in manual end-to-end runs, but do not yet have
    dedicated integration tests.
-   Docker Compose / Redpanda startup is not tested automatically.


## Tech Stack, Limitations, and Future Extensions

### Tech Stack

This project is built on a backend stack designed for real-time streaming and distributed processing:

- **Go 1.24** — Core implementation language for all services.
- **Redpanda (Kafka API compatible)** — Message broker for high-throughput chat event streaming.
- **kafka-go** — Go client for writing/reading Kafka messages.
- **gorilla/websocket** — WebSocket implementation used for Twitch IRC communication.
- **Docker & Docker Compose** — Service orchestration and reproducible local environments.
- **Twitch IRC & OAuth2 APIs** — Real-time chat ingestion and authentication.
- **slog (structured logging)** — Unified and context-aware logging across components.

These choices emphasize correctness, observability, concurrency, and the ability to scale or extend into full data/ML workflows.


## Limitations / Non‑Goals

This repository focuses on the ingestion spine of a Twitch analytics/ML system. The following aspects are intentionally out of scope for this stage:

- **Single-user authentication** — Only one Twitch account/token is supported at a time. Token refresh and multi-account orchestration are not implemented.
- **Limited security hardening** — The `/join` and `/part` endpoints do not require authentication and are intended for local/dev use only.
- **No long-term persistence layer** — Kafka events are consumed via a diagnostic consumer; no warehouse, data lake, or database storage layer is included.
- **No horizontal scaling logic** — The collector runs as a single instance; coordination across multiple ingest workers is future work.
- **Minimal Kafka configuration** — The producer uses simple per-message writes without batching or advanced delivery semantics.

These constraints keep the project focused on demonstrating a clean ingestion architecture and streaming pipeline.


## Future Work / Extensions

This ingestion backbone is intended to support further data engineering and machine‑learning development. Possible extensions include:

- **Analytical storage**  
  Export Kafka messages into ClickHouse, PostgreSQL, S3/Parquet, or a data lake for queryable historical datasets.

- **Real‑time feature extraction**  
  Compute aggregates, message embeddings, rate statistics, sentiment, or keyword features in a stream processor.

- **Model training and inference**  
  Train NLP or classification models on chat messages, then run inference either batch-wise or streaming.

- **Multi‑account ingest workers**  
  Support multiple Twitch accounts, token refresh, and horizontal scaling across channels.

- **Observability upgrades**  
  Metrics for Kafka throughput, IRC reconnects, join/part attempts, and reconciler outcomes.

- **Extended IRC event types**  
  Capture USERNOTICE, ROOMSTATE, raids, subscriptions, etc., with schema evolution support.

These extensions can evolve the repository into a full data/ML engineering portfolio project while keeping the ingestion layer stable.

