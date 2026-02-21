package playground

import (
	"encoding/json"
	"html/template"
	"net/http"
)

const exampleQuery = `# List all Key-Value stores
# Returns bucket name, configuration, and current state
{
  keyValues {
    bucket
    history
    ttl
    storage
    bytes
    values
    isCompressed
  }
}

# -----------------------------------------------
# List keys in a KV bucket
#
# {
#   kvKeys(bucket: "my-bucket")
# }

# -----------------------------------------------
# Get value for a specific key
#
# {
#   kvGet(bucket: "my-bucket", key: "my-key") {
#     key
#     value
#     revision
#     created
#   }
# }

# -----------------------------------------------
# Create a new KV bucket (mutation)
#
# mutation {
#   kvCreate(bucket: "my-bucket", history: 5, ttl: 3600) {
#     bucket
#     history
#     ttl
#     storage
#   }
# }

# -----------------------------------------------
# Put a value (mutation)
#
# mutation {
#   kvPut(bucket: "my-bucket", key: "my-key", value: "hello") {
#     key
#     value
#     revision
#     created
#   }
# }

# -----------------------------------------------
# Delete a key (leaves tombstone, key appears deleted)
#
# mutation {
#   kvDelete(bucket: "my-bucket", key: "my-key")
# }

# -----------------------------------------------
# Purge a key (fully removes key + all history)
#
# mutation {
#   kvPurge(bucket: "my-bucket", key: "my-key")
# }

# -----------------------------------------------
# Delete an entire KV bucket (mutation)
#
# mutation {
#   kvDeleteBucket(bucket: "my-bucket")
# }

# -----------------------------------------------
# Create a new stream (mutation)
#
# mutation {
#   streamCreate(name: "my-stream", subjects: ["orders.>"]) {
#     name
#     subjects
#     retention
#     storage
#   }
# }

# -----------------------------------------------
# Delete a stream (mutation)
#
# mutation {
#   streamDelete(name: "my-stream")
# }

# -----------------------------------------------
# Purge all messages from a stream (mutation)
# Stream itself is preserved, only messages are removed
#
# mutation {
#   streamPurge(name: "my-stream")
# }

# -----------------------------------------------
# Purge only messages matching a subject (mutation)
#
# mutation {
#   streamPurge(name: "my-stream", subject: "orders.error")
# }

# -----------------------------------------------
# Update stream settings (mutation)
# Only provided fields will be changed
#
# mutation {
#   streamUpdate(
#     name: "my-stream"
#     maxMsgs: 10000
#     maxAge: 86400
#   ) {
#     name
#     maxMsgs
#     maxAge
#     subjects
#   }
# }

# -----------------------------------------------
# Update KV bucket settings (mutation)
# Only provided fields will be changed
#
# mutation {
#   kvUpdate(bucket: "my-bucket", history: 10, ttl: 7200) {
#     bucket
#     history
#     ttl
#   }
# }

# -----------------------------------------------
# Read last N messages from a stream (max 100)
#
# {
#   streamMessages(stream: "my-stream", last: 5) {
#     sequence
#     subject
#     data
#     published
#   }
# }

# -----------------------------------------------
# Read messages with filters (all optional)
#
# {
#   streamMessages(
#     stream: "my-stream"
#     last: 10
#     startSeq: 100
#     startTime: "2026-01-01T00:00:00Z"
#     endTime: "2026-12-31T23:59:59Z"
#     subject: "orders.new"
#   ) {
#     sequence
#     subject
#     data
#     published
#   }
# }

# -----------------------------------------------
# Subscribe to new messages in real-time (WebSocket)
#
# subscription {
#   streamSubscribe(stream: "my-stream", subject: "orders.>") {
#     sequence
#     subject
#     data
#     published
#   }
# }

# -----------------------------------------------
# Publish a message to a subject (mutation)
#
# mutation {
#   publish(subject: "orders.new", data: "{\"id\": 1}") {
#     stream
#     sequence
#   }
# }

# -----------------------------------------------
# Publish a message with delay (mutation)
# The message will be published after 30 seconds
#
# mutation {
#   publishScheduled(subject: "orders.new", data: "{\"id\": 1}", delay: 30)
# }

# -----------------------------------------------
# List all JetStream streams
# Returns stream config and runtime statistics
#
# {
#   streams {
#     name
#     subjects
#     retention
#     storage
#     replicas
#     maxConsumers
#     maxMsgs
#     maxBytes
#     messages
#     bytes
#     consumers
#     created
#   }
# }

# -----------------------------------------------
# List consumers on a stream
#
# {
#   consumers(stream: "my-stream") {
#     name
#     stream
#     deliverPolicy
#     ackPolicy
#     numPending
#     numAckPending
#     paused
#   }
# }

# -----------------------------------------------
# Get info about a specific consumer
#
# {
#   consumerInfo(stream: "my-stream", name: "my-consumer") {
#     name
#     stream
#     deliverPolicy
#     ackPolicy
#     maxDeliver
#     maxAckPending
#     numPending
#     numAckPending
#     paused
#   }
# }

# -----------------------------------------------
# Create or update a consumer (mutation)
#
# mutation {
#   consumerCreate(
#     stream: "my-stream"
#     name: "my-consumer"
#     filterSubject: "orders.>"
#     deliverPolicy: "new"
#     ackPolicy: "explicit"
#     maxDeliver: 5
#     description: "Process new orders"
#   ) {
#     name
#     stream
#     deliverPolicy
#     ackPolicy
#     maxDeliver
#   }
# }

# -----------------------------------------------
# Delete a consumer (mutation)
#
# mutation {
#   consumerDelete(stream: "my-stream", name: "my-consumer")
# }

# -----------------------------------------------
# Pause a consumer until a specific time (mutation)
#
# mutation {
#   consumerPause(
#     stream: "my-stream"
#     name: "my-consumer"
#     pauseUntil: "2026-02-15T14:00:00Z"
#   )
# }

# -----------------------------------------------
# Resume a paused consumer (mutation)
#
# mutation {
#   consumerResume(stream: "my-stream", name: "my-consumer")
# }
`

var page = template.Must(template.New("playground").Parse(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>{{ .Title }}</title>
  <style>
    body { height: 100%; margin: 0; width: 100%; overflow: hidden; }
    #graphiql { height: 100vh; }
  </style>
  <link rel="stylesheet" href="https://unpkg.com/graphiql@3/graphiql.min.css" />
</head>
<body>
  <div id="graphiql"></div>
  <script crossorigin src="https://unpkg.com/react@18/umd/react.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/graphiql@3/graphiql.min.js"></script>
  <script>
    // Clear saved state so default query always shows
    for (var key in localStorage) {
      if (key.startsWith('graphiql')) {
        localStorage.removeItem(key);
      }
    }

    var fetcher = GraphiQL.createFetcher({ url: {{ .Endpoint }} });
    var defaultQuery = {{ .DefaultQuery }};
    var defaultHeaders = {{ .DefaultHeaders }};

    ReactDOM.createRoot(document.getElementById('graphiql')).render(
      React.createElement(GraphiQL, {
        fetcher: fetcher,
        defaultQuery: defaultQuery,
        defaultHeaders: defaultHeaders,
        headerEditorEnabled: true,
      })
    );
  </script>
</body>
</html>`))

// Handler returns an HTTP handler that serves the GraphiQL playground
// with a pre-loaded example query.
func Handler(title, endpoint string) http.HandlerFunc {
	endpointJSON, _ := json.Marshal(endpoint)
	queryJSON, _ := json.Marshal(exampleQuery)
	headersJSON, _ := json.Marshal(`{"Authorization": "Bearer <your-token>"}`)

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		page.Execute(w, map[string]template.JS{
			"Title":          template.JS(title),
			"Endpoint":       template.JS(endpointJSON),
			"DefaultQuery":   template.JS(queryJSON),
			"DefaultHeaders": template.JS(headersJSON),
		})
	}
}
