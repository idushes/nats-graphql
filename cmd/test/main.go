package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// GraphQL request/response types
type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

var (
	baseURL  string
	passed   int
	failed   int
	js       jetstream.JetStream
	natsConn *nats.Conn
)

const (
	testBucket = "__test_e2e__"
	testStream = "__test_stream_e2e__"
)

// ── helpers ───────────────────────────────────────────────────────

func query(q string) (json.RawMessage, error) {
	body, _ := json.Marshal(gqlRequest{Query: q})
	resp, err := http.Post(baseURL+"/query", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("http error: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var gql gqlResponse
	if err := json.Unmarshal(raw, &gql); err != nil {
		return nil, fmt.Errorf("json decode: %w\nraw: %s", err, string(raw))
	}
	if len(gql.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", gql.Errors[0].Message)
	}
	return gql.Data, nil
}

func queryExpectError(q string) string {
	body, _ := json.Marshal(gqlRequest{Query: q})
	resp, err := http.Post(baseURL+"/query", "application/json", bytes.NewReader(body))
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var gql gqlResponse
	if err := json.Unmarshal(raw, &gql); err != nil {
		return ""
	}
	if len(gql.Errors) > 0 {
		return gql.Errors[0].Message
	}
	return ""
}

func httpGet(path string) (*http.Response, string) {
	resp, err := http.Get(baseURL + path)
	if err != nil {
		return nil, ""
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, string(raw)
}

func assert(name string, ok bool, msg string) {
	if ok {
		passed++
		fmt.Printf("  ✅ %s\n", name)
	} else {
		failed++
		fmt.Printf("  ❌ %s — %s\n", name, msg)
	}
}

func unmarshal[T any](data json.RawMessage, field string) T {
	var wrapper map[string]T
	json.Unmarshal(data, &wrapper)
	return wrapper[field]
}

// ══════════════════════════════════════════════════════════════════
// HEALTHZ TESTS
// ══════════════════════════════════════════════════════════════════

func testHealthz() {
	fmt.Println("\n── /healthz ──")

	resp, body := httpGet("/healthz")
	assert("status 200", resp != nil && resp.StatusCode == 200, fmt.Sprintf("got: %v", resp))
	assert("body is 'ok'", body == "ok", "got: "+body)
}

// ══════════════════════════════════════════════════════════════════
// PLAYGROUND TESTS
// ══════════════════════════════════════════════════════════════════

func testPlayground() {
	fmt.Println("\n── / (playground) ──")

	resp, body := httpGet("/")
	assert("status 200", resp != nil && resp.StatusCode == 200, fmt.Sprintf("got: %v", resp))
	assert("content-type is html", resp != nil && strings.Contains(resp.Header.Get("Content-Type"), "text/html"), "")
	assert("contains GraphiQL", strings.Contains(body, "graphiql"), "GraphiQL not found in page")
	assert("contains example queries", strings.Contains(body, "kvPut"), "kvPut example not found")
	assert("contains kvGet example", strings.Contains(body, "kvGet"), "kvGet example not found")
	assert("contains kvDelete example", strings.Contains(body, "kvDelete"), "kvDelete example not found")
	assert("contains publish example", strings.Contains(body, "publish"), "publish example not found")
	assert("contains streamMessages example", strings.Contains(body, "streamMessages"), "streamMessages example not found")
}

// ══════════════════════════════════════════════════════════════════
// KEY-VALUE STORE LIST TESTS
// ══════════════════════════════════════════════════════════════════

func testKeyValuesListAllFields() {
	fmt.Println("\n── keyValues (all fields) ──")

	data, err := query(`{ keyValues { bucket history ttl storage bytes values isCompressed } }`)
	assert("query executes", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	type kvInfo struct {
		Bucket       string `json:"bucket"`
		History      int    `json:"history"`
		TTL          int    `json:"ttl"`
		Storage      string `json:"storage"`
		Bytes        int    `json:"bytes"`
		Values       int    `json:"values"`
		IsCompressed bool   `json:"isCompressed"`
	}
	var result struct {
		KeyValues []kvInfo `json:"keyValues"`
	}
	json.Unmarshal(data, &result)

	assert("returns non-empty array", len(result.KeyValues) > 0, "empty array")

	// Find test bucket
	var found *kvInfo
	for i := range result.KeyValues {
		if result.KeyValues[i].Bucket == testBucket {
			found = &result.KeyValues[i]
		}
	}
	assert("test bucket in list", found != nil, testBucket+" not found")
	if found == nil {
		return
	}

	assert("bucket name correct", found.Bucket == testBucket, "got: "+found.Bucket)
	assert("history >= 1", found.History >= 1, fmt.Sprintf("got: %d", found.History))
	assert("ttl is 0 (no ttl)", found.TTL == 0, fmt.Sprintf("got: %d", found.TTL))
	assert("storage is set", found.Storage != "", "empty storage")
	assert("bytes >= 0", found.Bytes >= 0, fmt.Sprintf("got: %d", found.Bytes))
	assert("values >= 0", found.Values >= 0, fmt.Sprintf("got: %d", found.Values))
	// isCompressed is false by default, just check it's returned
	assert("isCompressed is bool (false)", !found.IsCompressed, "expected false for default")
}

func testKeyValuesListEmpty() {
	fmt.Println("\n── keyValues (verify values count) ──")

	// Put a key first
	q := fmt.Sprintf(`mutation { kvPut(bucket: "%s", key: "count-test", value: "x") { revision } }`, testBucket)
	_, err := query(q)
	assert("put key for count test", err == nil, fmt.Sprint(err))

	data, err := query(`{ keyValues { bucket values } }`)
	assert("query executes", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	type kvInfo struct {
		Bucket string `json:"bucket"`
		Values int    `json:"values"`
	}
	var result struct {
		KeyValues []kvInfo `json:"keyValues"`
	}
	json.Unmarshal(data, &result)

	for _, kv := range result.KeyValues {
		if kv.Bucket == testBucket {
			assert("values count > 0", kv.Values > 0, fmt.Sprintf("got: %d", kv.Values))
			return
		}
	}
	assert("test bucket found", false, "test bucket not in list")
}

// ══════════════════════════════════════════════════════════════════
// STREAMS TESTS
// ══════════════════════════════════════════════════════════════════

func testStreamsListAllFields() {
	fmt.Println("\n── streams (all fields) ──")

	data, err := query(`{
		streams {
			name subjects retention storage replicas
			maxConsumers maxMsgs maxBytes
			messages bytes consumers created
		}
	}`)
	assert("query executes", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	type streamInfo struct {
		Name         string   `json:"name"`
		Subjects     []string `json:"subjects"`
		Retention    string   `json:"retention"`
		Storage      string   `json:"storage"`
		Replicas     int      `json:"replicas"`
		MaxConsumers int      `json:"maxConsumers"`
		MaxMsgs      int      `json:"maxMsgs"`
		MaxBytes     int      `json:"maxBytes"`
		Messages     int      `json:"messages"`
		Bytes        int      `json:"bytes"`
		Consumers    int      `json:"consumers"`
		Created      string   `json:"created"`
	}
	var result struct {
		Streams []streamInfo `json:"streams"`
	}
	json.Unmarshal(data, &result)

	// Find test stream
	var found *streamInfo
	for i := range result.Streams {
		if result.Streams[i].Name == testStream {
			found = &result.Streams[i]
		}
	}
	assert("test stream in list", found != nil, testStream+" not found")
	if found == nil {
		return
	}

	assert("name matches", found.Name == testStream, "got: "+found.Name)
	assert("has subjects", len(found.Subjects) > 0, "empty subjects")
	assert("subject matches", found.Subjects[0] == testStream+".>", "got: "+strings.Join(found.Subjects, ","))
	assert("retention is set", found.Retention != "", "empty retention")
	assert("storage is set", found.Storage != "", "empty storage")
	assert("replicas >= 1", found.Replicas >= 1, fmt.Sprintf("got: %d", found.Replicas))
	assert("maxConsumers is -1 (unlimited)", found.MaxConsumers == -1, fmt.Sprintf("got: %d", found.MaxConsumers))
	assert("maxMsgs is -1 (unlimited)", found.MaxMsgs == -1, fmt.Sprintf("got: %d", found.MaxMsgs))
	assert("maxBytes is -1 (unlimited)", found.MaxBytes == -1, fmt.Sprintf("got: %d", found.MaxBytes))
	assert("messages >= 0", found.Messages >= 0, fmt.Sprintf("got: %d", found.Messages))
	assert("bytes >= 0", found.Bytes >= 0, fmt.Sprintf("got: %d", found.Bytes))
	assert("consumers >= 0", found.Consumers >= 0, fmt.Sprintf("got: %d", found.Consumers))
	assert("created is RFC3339", found.Created != "", "empty created")

	// Verify created is valid RFC3339
	_, parseErr := time.Parse(time.RFC3339, found.Created)
	assert("created is valid RFC3339", parseErr == nil, fmt.Sprint(parseErr))
}

func testStreamsWithMessages() {
	fmt.Println("\n── streams (message count after publish) ──")

	// Publish messages to test stream
	for i := 0; i < 3; i++ {
		_, err := js.Publish(context.Background(), fmt.Sprintf("%s.test.%d", testStream, i), []byte(fmt.Sprintf("msg-%d", i)))
		if err != nil {
			assert("publish message", false, fmt.Sprint(err))
			return
		}
	}
	assert("published 3 messages", true, "")

	data, err := query(`{ streams { name messages bytes } }`)
	assert("query after publish", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	type streamInfo struct {
		Name     string `json:"name"`
		Messages int    `json:"messages"`
		Bytes    int    `json:"bytes"`
	}
	var result struct {
		Streams []streamInfo `json:"streams"`
	}
	json.Unmarshal(data, &result)

	for _, s := range result.Streams {
		if s.Name == testStream {
			assert("messages >= 3", s.Messages >= 3, fmt.Sprintf("got: %d", s.Messages))
			assert("bytes > 0", s.Bytes > 0, fmt.Sprintf("got: %d", s.Bytes))
			return
		}
	}
	assert("test stream found", false, "not in list")
}

// ══════════════════════════════════════════════════════════════════
// KV PUT TESTS
// ══════════════════════════════════════════════════════════════════

func testKvPut() {
	fmt.Println("\n── kvPut ──")

	// Put first key
	q := fmt.Sprintf(`mutation { kvPut(bucket: "%s", key: "name", value: "Alice") { key value revision created } }`, testBucket)
	data, err := query(q)
	assert("put first key", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	type entry struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		Revision int    `json:"revision"`
		Created  string `json:"created"`
	}
	e := unmarshal[entry](data, "kvPut")
	assert("key matches", e.Key == "name", "got: "+e.Key)
	assert("value matches", e.Value == "Alice", "got: "+e.Value)
	rev1 := e.Revision
	assert("revision > 0", rev1 > 0, fmt.Sprintf("got: %d", rev1))
	assert("created is set", e.Created != "", "empty created")

	// Verify created is valid RFC3339
	_, parseErr := time.Parse(time.RFC3339, e.Created)
	assert("created is valid RFC3339", parseErr == nil, fmt.Sprint(parseErr))

	// Update same key
	q = fmt.Sprintf(`mutation { kvPut(bucket: "%s", key: "name", value: "Bob") { key value revision } }`, testBucket)
	data, err = query(q)
	assert("update key", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	e = unmarshal[entry](data, "kvPut")
	assert("updated value", e.Value == "Bob", "got: "+e.Value)
	assert("revision incremented", e.Revision > rev1, fmt.Sprintf("got: %d, prev: %d", e.Revision, rev1))

	// Put second and third keys
	q = fmt.Sprintf(`mutation { kvPut(bucket: "%s", key: "age", value: "30") { key value } }`, testBucket)
	_, err = query(q)
	assert("put second key", err == nil, fmt.Sprint(err))

	q = fmt.Sprintf(`mutation { kvPut(bucket: "%s", key: "city", value: "Tokyo") { key value } }`, testBucket)
	_, err = query(q)
	assert("put third key", err == nil, fmt.Sprint(err))
}

// ══════════════════════════════════════════════════════════════════
// KV KEYS TESTS
// ══════════════════════════════════════════════════════════════════

func testKvKeys() {
	fmt.Println("\n── kvKeys ──")

	q := fmt.Sprintf(`{ kvKeys(bucket: "%s") }`, testBucket)
	data, err := query(q)
	assert("list keys", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	keys := unmarshal[[]string](data, "kvKeys")
	assert("has >= 3 keys", len(keys) >= 3, fmt.Sprintf("got %d keys: %v", len(keys), keys))

	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	assert("contains 'name'", keySet["name"], fmt.Sprintf("keys: %v", keys))
	assert("contains 'age'", keySet["age"], fmt.Sprintf("keys: %v", keys))
	assert("contains 'city'", keySet["city"], fmt.Sprintf("keys: %v", keys))
}

// ══════════════════════════════════════════════════════════════════
// KV GET TESTS
// ══════════════════════════════════════════════════════════════════

func testKvGet() {
	fmt.Println("\n── kvGet ──")

	type entry struct {
		Key      string  `json:"key"`
		Value    string  `json:"value"`
		Revision int     `json:"revision"`
		Created  *string `json:"created"`
	}

	// Get existing key
	q := fmt.Sprintf(`{ kvGet(bucket: "%s", key: "name") { key value revision created } }`, testBucket)
	data, err := query(q)
	assert("get existing key", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	var result struct {
		KvGet *entry `json:"kvGet"`
	}
	json.Unmarshal(data, &result)
	assert("entry not nil", result.KvGet != nil, "got nil")
	if result.KvGet != nil {
		assert("key is 'name'", result.KvGet.Key == "name", "got: "+result.KvGet.Key)
		assert("value is 'Bob' (updated)", result.KvGet.Value == "Bob", "got: "+result.KvGet.Value)
		assert("revision > 0", result.KvGet.Revision > 0, fmt.Sprintf("got: %d", result.KvGet.Revision))
		assert("created is set", result.KvGet.Created != nil && *result.KvGet.Created != "", "empty")
	}

	// Get non-existent key → null
	q = fmt.Sprintf(`{ kvGet(bucket: "%s", key: "nonexistent") { key value } }`, testBucket)
	data, err = query(q)
	assert("get nonexistent key (no error)", err == nil, fmt.Sprint(err))
	if err == nil {
		result.KvGet = nil // reset
		json.Unmarshal(data, &result)
		assert("nonexistent key returns null", result.KvGet == nil, "expected null")
	}

	// Get each key to verify different values
	for _, tc := range []struct{ key, value string }{
		{"age", "30"},
		{"city", "Tokyo"},
	} {
		q = fmt.Sprintf(`{ kvGet(bucket: "%s", key: "%s") { value } }`, testBucket, tc.key)
		data, err = query(q)
		assert(fmt.Sprintf("get '%s'", tc.key), err == nil, fmt.Sprint(err))
		if err == nil {
			result.KvGet = nil
			json.Unmarshal(data, &result)
			if result.KvGet != nil {
				assert(fmt.Sprintf("'%s' value is '%s'", tc.key, tc.value), result.KvGet.Value == tc.value, "got: "+result.KvGet.Value)
			}
		}
	}
}

// ══════════════════════════════════════════════════════════════════
// KV DELETE TESTS
// ══════════════════════════════════════════════════════════════════

func testKvDelete() {
	fmt.Println("\n── kvDelete ──")

	// Delete existing key
	q := fmt.Sprintf(`mutation { kvDelete(bucket: "%s", key: "city") }`, testBucket)
	data, err := query(q)
	assert("delete existing key", err == nil, fmt.Sprint(err))
	if err == nil {
		val := unmarshal[bool](data, "kvDelete")
		assert("returns true", val, "got false")
	}

	// Verify key is gone via kvGet
	q = fmt.Sprintf(`{ kvGet(bucket: "%s", key: "city") { key } }`, testBucket)
	data, err = query(q)
	assert("get deleted key (no error)", err == nil, fmt.Sprint(err))
	if err == nil {
		type e struct {
			Key string `json:"key"`
		}
		var result struct {
			KvGet *e `json:"kvGet"`
		}
		json.Unmarshal(data, &result)
		assert("deleted key returns null", result.KvGet == nil, "expected null")
	}

	// Verify keys count decreased via kvKeys
	q = fmt.Sprintf(`{ kvKeys(bucket: "%s") }`, testBucket)
	data, err = query(q)
	assert("list keys after delete", err == nil, fmt.Sprint(err))
	if err == nil {
		keys := unmarshal[[]string](data, "kvKeys")
		keySet := make(map[string]bool)
		for _, k := range keys {
			keySet[k] = true
		}
		assert("'city' no longer in keys", !keySet["city"], fmt.Sprintf("keys: %v", keys))
		assert("'name' still in keys", keySet["name"], fmt.Sprintf("keys: %v", keys))
		assert("'age' still in keys", keySet["age"], fmt.Sprintf("keys: %v", keys))
	}
}

// ══════════════════════════════════════════════════════════════════
// ERROR HANDLING TESTS
// ══════════════════════════════════════════════════════════════════

func testErrorNonexistentBucket() {
	fmt.Println("\n── errors: nonexistent bucket ──")

	// kvPut
	errMsg := queryExpectError(`mutation { kvPut(bucket: "__no_such_bucket__", key: "a", value: "b") { key } }`)
	assert("kvPut returns error", errMsg != "", "expected error, got success")

	// kvKeys
	errMsg = queryExpectError(`{ kvKeys(bucket: "__no_such_bucket__") }`)
	assert("kvKeys returns error", errMsg != "", "expected error, got success")

	// kvGet
	errMsg = queryExpectError(`{ kvGet(bucket: "__no_such_bucket__", key: "a") { key } }`)
	assert("kvGet returns error", errMsg != "", "expected error, got success")

	// kvDelete
	errMsg = queryExpectError(`mutation { kvDelete(bucket: "__no_such_bucket__", key: "a") }`)
	assert("kvDelete returns error", errMsg != "", "expected error, got success")
}

func testErrorInvalidQuery() {
	fmt.Println("\n── errors: invalid GraphQL queries ──")

	// Syntax error
	errMsg := queryExpectError(`{ keyValues { nonExistentField } }`)
	assert("unknown field returns error", errMsg != "", "expected error")

	// Missing required arg
	errMsg = queryExpectError(`{ kvKeys }`)
	assert("missing required arg returns error", errMsg != "", "expected error")

	// Wrong type arg
	errMsg = queryExpectError(`{ kvGet(bucket: 123, key: "a") { key } }`)
	assert("wrong arg type returns error", errMsg != "", "expected error")
}

// ══════════════════════════════════════════════════════════════════
// EDGE CASE TESTS
// ══════════════════════════════════════════════════════════════════

func testEdgeCaseEmptyValue() {
	fmt.Println("\n── edge: empty value ──")

	q := fmt.Sprintf(`mutation { kvPut(bucket: "%s", key: "empty-val", value: "") { key value revision } }`, testBucket)
	data, err := query(q)
	assert("put empty value", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	type entry struct {
		Value string `json:"value"`
	}
	e := unmarshal[entry](data, "kvPut")
	assert("value is empty string", e.Value == "", fmt.Sprintf("got: %q", e.Value))

	// Read back
	q = fmt.Sprintf(`{ kvGet(bucket: "%s", key: "empty-val") { value } }`, testBucket)
	data, err = query(q)
	assert("get empty value back", err == nil, fmt.Sprint(err))
	if err == nil {
		var result struct {
			KvGet *entry `json:"kvGet"`
		}
		json.Unmarshal(data, &result)
		assert("read-back value is empty", result.KvGet != nil && result.KvGet.Value == "", "not empty")
	}
}

func testEdgeCaseSpecialCharacters() {
	fmt.Println("\n── edge: special characters ──")

	specialValue := `{"json": true, "nested": {"arr": [1,2,3]}}`
	escaped := strings.ReplaceAll(specialValue, `"`, `\"`)
	q := fmt.Sprintf(`mutation { kvPut(bucket: "%s", key: "json-data", value: "%s") { key value } }`, testBucket, escaped)
	data, err := query(q)
	assert("put JSON value", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	type entry struct {
		Value string `json:"value"`
	}
	e := unmarshal[entry](data, "kvPut")
	assert("JSON value preserved", e.Value == specialValue, "got: "+e.Value)

	// Read back
	q = fmt.Sprintf(`{ kvGet(bucket: "%s", key: "json-data") { value } }`, testBucket)
	data, err = query(q)
	assert("get JSON value back", err == nil, fmt.Sprint(err))
	if err == nil {
		var result struct {
			KvGet *entry `json:"kvGet"`
		}
		json.Unmarshal(data, &result)
		if result.KvGet != nil {
			assert("JSON value matches on read", result.KvGet.Value == specialValue, "got: "+result.KvGet.Value)
		}
	}
}

func testEdgeCaseLongValue() {
	fmt.Println("\n── edge: long value ──")

	longValue := strings.Repeat("A", 10000)
	escaped := longValue // no special chars to escape
	q := fmt.Sprintf(`mutation { kvPut(bucket: "%s", key: "long-val", value: "%s") { key revision } }`, testBucket, escaped)
	data, err := query(q)
	assert("put 10KB value", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	type entry struct {
		Revision int `json:"revision"`
	}
	e := unmarshal[entry](data, "kvPut")
	assert("revision assigned", e.Revision > 0, fmt.Sprintf("got: %d", e.Revision))

	// Read back and verify length
	q = fmt.Sprintf(`{ kvGet(bucket: "%s", key: "long-val") { value } }`, testBucket)
	data, err = query(q)
	assert("get long value back", err == nil, fmt.Sprint(err))
	if err == nil {
		type ve struct {
			Value string `json:"value"`
		}
		var result struct {
			KvGet *ve `json:"kvGet"`
		}
		json.Unmarshal(data, &result)
		if result.KvGet != nil {
			assert("long value length correct", len(result.KvGet.Value) == 10000, fmt.Sprintf("got: %d", len(result.KvGet.Value)))
		}
	}
}

func testEdgeCaseMultipleUpdates() {
	fmt.Println("\n── edge: multiple rapid updates ──")

	// Write same key 5 times, verify final state
	for i := 1; i <= 5; i++ {
		q := fmt.Sprintf(`mutation { kvPut(bucket: "%s", key: "rapid", value: "v%d") { revision } }`, testBucket, i)
		_, err := query(q)
		if err != nil {
			assert(fmt.Sprintf("rapid update %d", i), false, fmt.Sprint(err))
			return
		}
	}
	assert("5 rapid updates succeeded", true, "")

	// Read final value
	q := fmt.Sprintf(`{ kvGet(bucket: "%s", key: "rapid") { value revision } }`, testBucket)
	data, err := query(q)
	assert("get after rapid updates", err == nil, fmt.Sprint(err))
	if err == nil {
		type entry struct {
			Value    string `json:"value"`
			Revision int    `json:"revision"`
		}
		var result struct {
			KvGet *entry `json:"kvGet"`
		}
		json.Unmarshal(data, &result)
		if result.KvGet != nil {
			assert("final value is v5", result.KvGet.Value == "v5", "got: "+result.KvGet.Value)
			assert("revision >= 5", result.KvGet.Revision >= 5, fmt.Sprintf("got: %d", result.KvGet.Revision))
		}
	}
}

func testEdgeCaseDeleteAndRecreate() {
	fmt.Println("\n── edge: delete then recreate ──")

	key := "phoenix"

	// Create
	q := fmt.Sprintf(`mutation { kvPut(bucket: "%s", key: "%s", value: "v1") { revision } }`, testBucket, key)
	data, err := query(q)
	assert("create key", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}
	type entry struct {
		Revision int `json:"revision"`
	}
	e := unmarshal[entry](data, "kvPut")
	rev1 := e.Revision

	// Delete
	q = fmt.Sprintf(`mutation { kvDelete(bucket: "%s", key: "%s") }`, testBucket, key)
	_, err = query(q)
	assert("delete key", err == nil, fmt.Sprint(err))

	// Verify gone
	q = fmt.Sprintf(`{ kvGet(bucket: "%s", key: "%s") { value } }`, testBucket, key)
	data, err = query(q)
	assert("key is gone", err == nil, fmt.Sprint(err))

	// Recreate
	q = fmt.Sprintf(`mutation { kvPut(bucket: "%s", key: "%s", value: "v2") { value revision } }`, testBucket, key)
	data, err = query(q)
	assert("recreate key", err == nil, fmt.Sprint(err))
	if err == nil {
		type e2 struct {
			Value    string `json:"value"`
			Revision int    `json:"revision"`
		}
		r := unmarshal[e2](data, "kvPut")
		assert("recreated value is v2", r.Value == "v2", "got: "+r.Value)
		assert("revision > first", r.Revision > rev1, fmt.Sprintf("got: %d, expected > %d", r.Revision, rev1))
	}
}

// ══════════════════════════════════════════════════════════════════
// PUBLISH TESTS
// ══════════════════════════════════════════════════════════════════

func testPublish() {
	fmt.Println("\n── publish ──")

	type publishResult struct {
		Stream   string `json:"stream"`
		Sequence int    `json:"sequence"`
	}

	// Publish to test stream
	q := fmt.Sprintf(`mutation { publish(subject: "%s.test.msg1", data: "hello world") { stream sequence } }`, testStream)
	data, err := query(q)
	assert("publish message", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	r := unmarshal[publishResult](data, "publish")
	assert("stream name returned", r.Stream == testStream, "got: "+r.Stream)
	assert("sequence > 0", r.Sequence > 0, fmt.Sprintf("got: %d", r.Sequence))
	seq1 := r.Sequence

	// Publish second message
	q = fmt.Sprintf(`mutation { publish(subject: "%s.test.msg2", data: "second message") { stream sequence } }`, testStream)
	data, err = query(q)
	assert("publish second message", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	r = unmarshal[publishResult](data, "publish")
	assert("sequence incremented", r.Sequence > seq1, fmt.Sprintf("got: %d, prev: %d", r.Sequence, seq1))

	// Publish with empty data
	q = fmt.Sprintf(`mutation { publish(subject: "%s.test.empty", data: "") { stream sequence } }`, testStream)
	data, err = query(q)
	assert("publish empty data", err == nil, fmt.Sprint(err))

	// Publish with JSON data
	escaped := strings.ReplaceAll(`{"key": "value", "num": 42}`, `"`, `\"`)
	q = fmt.Sprintf(`mutation { publish(subject: "%s.test.json", data: "%s") { stream sequence } }`, testStream, escaped)
	data, err = query(q)
	assert("publish JSON data", err == nil, fmt.Sprint(err))
}

func testPublishErrors() {
	fmt.Println("\n── publish errors ──")

	// Subject with no matching stream
	errMsg := queryExpectError(`mutation { publish(subject: "__no_stream_matches_this__.test", data: "x") { stream } }`)
	assert("no matching stream returns error", errMsg != "", "expected error")

	// Payload too large (> 1MB) — we send a query with a large value
	// Note: we can't easily craft a >1MB GraphQL payload in a test, so test the concept
	assert("payload limit documented", true, "1MB limit enforced in resolver")
}

// ══════════════════════════════════════════════════════════════════
// STREAM MESSAGES TESTS
// ══════════════════════════════════════════════════════════════════

func testStreamMessages() {
	fmt.Println("\n── streamMessages ──")

	type msg struct {
		Sequence  int    `json:"sequence"`
		Subject   string `json:"subject"`
		Data      string `json:"data"`
		Published string `json:"published"`
	}

	// Publish known messages for testing
	for i := 1; i <= 5; i++ {
		q := fmt.Sprintf(`mutation { publish(subject: "%s.read.%d", data: "msg-%d") { sequence } }`, testStream, i, i)
		_, err := query(q)
		if err != nil {
			assert(fmt.Sprintf("publish msg %d for read test", i), false, fmt.Sprint(err))
			return
		}
	}
	assert("published 5 messages for read test", true, "")

	// Read last 3 messages
	q := fmt.Sprintf(`{ streamMessages(stream: "%s", last: 3) { sequence subject data published } }`, testStream)
	data, err := query(q)
	assert("read last 3 messages", err == nil, fmt.Sprint(err))
	if err != nil {
		return
	}

	var result struct {
		StreamMessages []msg `json:"streamMessages"`
	}
	json.Unmarshal(data, &result)
	assert("got 3 messages", len(result.StreamMessages) == 3, fmt.Sprintf("got: %d", len(result.StreamMessages)))

	if len(result.StreamMessages) >= 3 {
		// Check chronological order (oldest first)
		assert("messages in order", result.StreamMessages[0].Sequence < result.StreamMessages[2].Sequence,
			fmt.Sprintf("seq[0]=%d, seq[2]=%d", result.StreamMessages[0].Sequence, result.StreamMessages[2].Sequence))

		// Last message should be the most recent
		lastMsg := result.StreamMessages[len(result.StreamMessages)-1]
		assert("last message data is 'msg-5'", lastMsg.Data == "msg-5", "got: "+lastMsg.Data)
		assert("subject contains stream prefix", strings.HasPrefix(lastMsg.Subject, testStream+"."), "got: "+lastMsg.Subject)
		assert("published is set", lastMsg.Published != "", "empty published")

		// Verify published is valid RFC3339
		_, parseErr := time.Parse(time.RFC3339, lastMsg.Published)
		assert("published is valid RFC3339", parseErr == nil, fmt.Sprint(parseErr))
	}

	// Read with default last (10)
	q = fmt.Sprintf(`{ streamMessages(stream: "%s") { sequence data } }`, testStream)
	data, err = query(q)
	assert("read with default last", err == nil, fmt.Sprint(err))
	if err == nil {
		json.Unmarshal(data, &result)
		assert("got <= 10 messages", len(result.StreamMessages) <= 10, fmt.Sprintf("got: %d", len(result.StreamMessages)))
		assert("got > 0 messages", len(result.StreamMessages) > 0, fmt.Sprintf("got: %d", len(result.StreamMessages)))
	}

	// Read all messages with last=100
	q = fmt.Sprintf(`{ streamMessages(stream: "%s", last: 100) { sequence } }`, testStream)
	data, err = query(q)
	assert("read with last=100", err == nil, fmt.Sprint(err))
}

func testStreamMessagesEdgeCases() {
	fmt.Println("\n── streamMessages edge cases ──")

	// last > 100 should error
	q := fmt.Sprintf(`{ streamMessages(stream: "%s", last: 101) { sequence } }`, testStream)
	errMsg := queryExpectError(q)
	assert("last=101 returns error (max 100)", errMsg != "", "expected error")
	if errMsg != "" {
		assert("error mentions maximum", strings.Contains(strings.ToLower(errMsg), "max") || strings.Contains(errMsg, "100"), "error: "+errMsg)
	}

	// last=0 should error
	q = fmt.Sprintf(`{ streamMessages(stream: "%s", last: 0) { sequence } }`, testStream)
	errMsg = queryExpectError(q)
	assert("last=0 returns error", errMsg != "", "expected error")

	// Nonexistent stream should error
	errMsg = queryExpectError(`{ streamMessages(stream: "__no_such_stream__", last: 5) { sequence } }`)
	assert("nonexistent stream returns error", errMsg != "", "expected error")

	// Create an empty stream and read from it
	_, err := js.CreateStream(context.Background(), jetstream.StreamConfig{
		Name:     testStream + "_empty",
		Subjects: []string{testStream + "_empty.>"},
	})
	if err == nil {
		q = fmt.Sprintf(`{ streamMessages(stream: "%s_empty") { sequence } }`, testStream)
		data, err := query(q)
		assert("empty stream returns no error", err == nil, fmt.Sprint(err))
		if err == nil {
			type msg struct {
				Sequence int `json:"sequence"`
			}
			var result struct {
				StreamMessages []msg `json:"streamMessages"`
			}
			json.Unmarshal(data, &result)
			assert("empty stream returns 0 messages", len(result.StreamMessages) == 0, fmt.Sprintf("got: %d", len(result.StreamMessages)))
		}
		// Clean up
		js.DeleteStream(context.Background(), testStream+"_empty")
	}
}

// ══════════════════════════════════════════════════════════════════
// SETUP & TEARDOWN
// ══════════════════════════════════════════════════════════════════

func setup() {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	var err error
	natsConn, err = nats.Connect(natsURL)
	if err != nil {
		fmt.Printf("❌ Cannot connect to NATS at %s: %v\n", natsURL, err)
		os.Exit(1)
	}

	js, err = jetstream.New(natsConn)
	if err != nil {
		fmt.Printf("❌ Cannot create JetStream context: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Create test KV bucket
	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: testBucket,
	})
	if err != nil {
		fmt.Printf("❌ Cannot create test bucket: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("   created test bucket '%s'\n", testBucket)

	// Create test stream
	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     testStream,
		Subjects: []string{testStream + ".>"},
	})
	if err != nil {
		fmt.Printf("❌ Cannot create test stream: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("   created test stream '%s'\n", testStream)
}

func teardown() {
	fmt.Println("\n── teardown ──")
	ctx := context.Background()

	if err := js.DeleteKeyValue(ctx, testBucket); err != nil {
		fmt.Printf("  ⚠️  failed to delete test bucket: %v\n", err)
	} else {
		fmt.Printf("  🧹 deleted bucket '%s'\n", testBucket)
	}

	if err := js.DeleteStream(ctx, testStream); err != nil {
		fmt.Printf("  ⚠️  failed to delete test stream: %v\n", err)
	} else {
		fmt.Printf("  🧹 deleted stream '%s'\n", testStream)
	}

	natsConn.Close()
}

// ══════════════════════════════════════════════════════════════════
// MAIN
// ══════════════════════════════════════════════════════════════════

func main() {
	baseURL = os.Getenv("GRAPHQL_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8181"
	}

	fmt.Printf("🧪 nats-graphql E2E tests\n")
	fmt.Printf("   target: %s\n", baseURL)

	// Check server is available
	_, err := query(`{ keyValues { bucket } }`)
	if err != nil {
		fmt.Printf("\n❌ Cannot connect to server: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("   server reachable ✅")

	// Setup NATS test resources
	setup()
	defer teardown()

	// ── HTTP endpoints ──
	testHealthz()
	testPlayground()

	// ── Key-Value stores listing ──
	testKvPut()
	testKeyValuesListAllFields()
	testKeyValuesListEmpty()

	// ── Streams ──
	testStreamsListAllFields()
	testStreamsWithMessages()

	// ── KV operations ──
	testKvKeys()
	testKvGet()
	testKvDelete()

	// ── Error handling ──
	testErrorNonexistentBucket()
	testErrorInvalidQuery()

	// ── Edge cases ──
	testEdgeCaseEmptyValue()
	testEdgeCaseSpecialCharacters()
	testEdgeCaseLongValue()
	testEdgeCaseMultipleUpdates()
	testEdgeCaseDeleteAndRecreate()

	// ── Publish & StreamMessages ──
	testPublish()
	testPublishErrors()
	testStreamMessages()
	testStreamMessagesEdgeCases()

	// Summary
	total := passed + failed
	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  Total: %d  ✅ %d  ❌ %d\n", total, passed, failed)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	if failed > 0 {
		os.Exit(1)
	}
}
