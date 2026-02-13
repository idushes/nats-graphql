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

    ReactDOM.createRoot(document.getElementById('graphiql')).render(
      React.createElement(GraphiQL, {
        fetcher: fetcher,
        defaultQuery: defaultQuery,
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

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		page.Execute(w, map[string]template.JS{
			"Title":        template.JS(title),
			"Endpoint":     template.JS(endpointJSON),
			"DefaultQuery": template.JS(queryJSON),
		})
	}
}
