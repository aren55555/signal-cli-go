package daemon

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aren55555/signal-cli-go/internal/errcode"
)

func testServer(accounts ...string) *Server {
	s := &Server{AutoSubscribe: true, Accounts: func() []Account {
		var out []Account
		for _, a := range accounts {
			out = append(out, Account{Number: a})
		}
		return out
	}}
	s.Methods = SubscriptionMethods(nil)
	s.Methods["version"] = &Method{Class: Both, Run: func(ctx context.Context, c *Call) (string, error) {
		if _, err := NamespaceParams(c); err != nil {
			return "", err
		}
		return `{"version":"0.14.7"}`, nil
	}}
	s.Methods["echoAccount"] = &Method{Class: Single, Run: func(ctx context.Context, c *Call) (string, error) {
		return `"` + c.Account + `"`, nil
	}}
	s.Methods["register"] = &Method{Class: Registration, Run: func(ctx context.Context, c *Call) (string, error) {
		return `"` + c.Account + `"`, nil
	}}
	s.Methods["listAccounts"] = &Method{Class: Multi, Run: func(ctx context.Context, c *Call) (string, error) { return "[]", nil }}
	s.Methods["fail"] = &Method{Class: Both, Run: func(ctx context.Context, c *Call) (string, error) {
		return "", errcode.SendFailed.New().WithData(`{"response":{"timestamp":5}}`)
	}}
	return s
}

// session feeds lines to one stream connection and returns the output
// lines, sorted (requests run concurrently).
func session(t *testing.T, s *Server, input string) []string {
	t.Helper()
	var out strings.Builder
	w := &lockedWriter{w: &out}
	if err := s.ServeStream(context.Background(), strings.NewReader(input), w); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if out.Len() == 0 {
		lines = nil
	}
	sort.Strings(lines)
	return lines
}

type lockedWriter struct {
	w io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) { return l.w.Write(p) }

func errLine(code int, msg, id string) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","error":{"code":%d,"message":%q,"data":null},"id":%s}`, code, msg, id)
}

func TestStreamRequests(t *testing.T) {
	const u = "\n at [Source: UNKNOWN; byte offset: #UNKNOWN]"
	for _, c := range []struct {
		in   string
		want []string
	}{
		// Validation errors still answer a request without an id.
		{`{}`, []string{errLine(-32600, "only jsonrpc version 2.0 is supported", "null")}},
		{`{"jsonrpc":"2.0","method":"version"}`, nil},
		{`{"jsonrpc":"2.0","method":"nope"}`, nil},
		// The id of a request that failed to bind is the raw id member.
		{`{"extra":1,"jsonrpc":"2.0","method":"version","id":13}`, []string{errLine(-32600, `Unrecognized field "extra" (class org.asamk.signal.jsonrpc.JsonRpcRequest), not marked as ignorable (4 known properties: "method", "id", "jsonrpc", "params")`+u+` (through reference chain: org.asamk.signal.jsonrpc.JsonRpcRequest["extra"])`, "13")}},
		// Members bind in order: the first problem wins.
		{`{"params":5,"id":7,"extra":1}`, []string{errLine(-32600, "Problem deserializing property 'params' (expected type: [simple type, class com.fasterxml.jackson.databind.node.ContainerNode<java.lang.Object>]; actual type: `com.fasterxml.jackson.databind.node.IntNode`), problem: Can not set com.fasterxml.jackson.databind.node.ContainerNode field org.asamk.signal.jsonrpc.JsonRpcRequest.params to com.fasterxml.jackson.databind.node.IntNode"+u+` (through reference chain: org.asamk.signal.jsonrpc.JsonRpcRequest["params"])`, "7")}},
		{`{"jsonrpc":"2.0","id":9,"method":{}}`, []string{errLine(-32600, "Cannot deserialize value of type `java.lang.String` from Object value (token `JsonToken.START_OBJECT`)"+u+` (through reference chain: org.asamk.signal.jsonrpc.JsonRpcRequest["method"])`, "9")}},
		{`{"jsonrpc":"2.0","method":true,"id":7}`, []string{errLine(-32601, "Method not implemented", "7")}},
		{`{"jsonrpc":"2.0","method":null,"id":6}`, []string{errLine(-32600, "method field must be set", "6")}},
		{`{"jsonrpc":"2.0","jsonrpc":"1.0","method":"version","id":3}`, []string{errLine(-32600, "only jsonrpc version 2.0 is supported", "3")}},
		{`{"jsonrpc":2.00,"method":"version","id":1}`, []string{`{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}`}},
		{`{"jsonrpc":"2.0","method":"version","id":1,"id":2}`, []string{`{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":2}`}},
		{`{"jsonrpc":"2.0","method":"version","id":1e7}`, []string{`{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1.0E7}`}},
		{`{"jsonrpc":"2.0","method":"version","id":1e400}`, []string{`{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":"Infinity"}`}},
		// A batch with a null element is dropped entirely.
		{`[{"jsonrpc":"2.0","method":"version","id":1},null]`, nil},
		{`[true]`, []string{`[` + errLine(-32600, "Cannot construct instance of `org.asamk.signal.jsonrpc.JsonRpcRequest` (although at least one Creator exists): no boolean/Boolean-argument constructor/factory method to deserialize from boolean value (true)"+u, "null") + `]`}},
		{`[12345678901]`, []string{`[` + errLine(-32600, "Cannot construct instance of `org.asamk.signal.jsonrpc.JsonRpcRequest` (although at least one Creator exists): no long/Long-argument constructor/factory method to deserialize from Number value (12345678901)"+u, "null") + `]`}},
		{`[1e5]`, []string{`[` + errLine(-32600, "Cannot construct instance of `org.asamk.signal.jsonrpc.JsonRpcRequest` (although at least one Creator exists): no double/Double-argument constructor/factory method to deserialize from Number value (100000.0)"+u, "null") + `]`}},
		{`[{}]`, []string{`[` + errLine(-32600, "only jsonrpc version 2.0 is supported", "null") + `]`}},
		{"\x01", []string{errLine(-32700, "Empty input line", "null")}},
		{`true`, []string{errLine(-32600, "unexpected type: BOOLEAN", "null")}},
		// Account selection.
		{`{"jsonrpc":"2.0","method":"version","params":{"account":5},"id":8}`, []string{errLine(-32602, "Specified account does not exist", "8")}},
		{`{"jsonrpc":"2.0","method":"listAccounts","params":{"account":{}},"id":9}`, []string{`{"jsonrpc":"2.0","result":[],"id":9}`}},
		{`{"jsonrpc":"2.0","method":"echoAccount","params":[],"id":14}`, []string{errLine(-32602, "Method requires valid account parameter", "14")}},
		{`{"jsonrpc":"2.0","method":"register","params":[],"id":13}`, []string{errLine(-32602, "Method requires account parameter", "13")}},
		{`{"jsonrpc":"2.0","method":"register","params":{"account":null},"id":11}`, []string{`{"jsonrpc":"2.0","result":"null","id":11}`}},
		// Error data from a method that produced a result first.
		{`{"jsonrpc":"2.0","method":"fail","id":1}`, []string{`{"jsonrpc":"2.0","error":{"code":-1,"message":"Failed to send message","data":{"response":{"timestamp":5}}},"id":1}`}},
	} {
		got := session(t, testServer(), c.in+"\n")
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s:\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestSingleAccountSelection(t *testing.T) {
	s := testServer("+15555550101", "+15555550102")
	got := session(t, s, `{"jsonrpc":"2.0","method":"echoAccount","params":{"account":"+15555550102"},"id":1}`+"\n"+
		`{"jsonrpc":"2.0","method":"echoAccount","id":2}`+"\n")
	want := []string{`{"jsonrpc":"2.0","error":{"code":-32602,"message":"Method requires valid account parameter","data":null},"id":2}`, `{"jsonrpc":"2.0","result":"+15555550102","id":1}`}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q", got)
	}
	s = testServer("+15555550101")
	if got := session(t, s, `{"jsonrpc":"2.0","method":"echoAccount","id":3}`+"\n"); len(got) != 1 || !strings.Contains(got[0], `"result":"+15555550101"`) {
		t.Errorf("one account: %q", got)
	}
	s.SingleAccount = "+15555550101"
	if got := session(t, s, `{"jsonrpc":"2.0","method":"listAccounts","id":4}`+"\n"); len(got) != 1 || !strings.Contains(got[0], "Method not implemented") {
		t.Errorf("multi method in single mode: %q", got)
	}
}

// The unsubscribe edge cases observed in the reference: params values
// convert with JsonNode.asInt, so null and "x" mean id 0. Subscription
// methods run in input order.
func TestUnsubscribe(t *testing.T) {
	s := testServer()
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","method":"unsubscribeReceive","params":{"subscription":"x"},"id":22}`,
		`{"jsonrpc":"2.0","method":"unsubscribeReceive","params":{"subscription":null},"id":23}`,
		`{"jsonrpc":"2.0","method":"unsubscribeReceive","params":{"subscription":[]},"id":24}`,
		`{"jsonrpc":"2.0","method":"unsubscribeReceive","params":[],"id":25}`,
		`{"jsonrpc":"2.0","method":"unsubscribeReceive","params":[1.9],"id":27}`,
	}, "\n") + "\n"
	got := session(t, s, in)
	// "x" is id 0, this connection's implicit subscription; null then
	// misses. (The reference ran these concurrently, so its transcript
	// shows null winning the race instead.)
	alt := []string{
		`{"jsonrpc":"2.0","result":{},"id":22}`,
		errLine(-1, "Unknown subscription id", "23"),
		errLine(-1, "Unknown subscription id", "24"),
		errLine(-32603, `Cannot invoke "com.fasterxml.jackson.databind.JsonNode.asInt()" because the return value of "com.fasterxml.jackson.databind.node.ArrayNode.get(int)" is null (NullPointerException)`, "25"),
		errLine(-1, "Unknown subscription id", "27"),
	}
	sort.Strings(alt)
	if strings.Join(got, "\n") != strings.Join(alt, "\n") {
		t.Errorf("got %q", got)
	}
}

func TestSubscriptionIDsAndNotifications(t *testing.T) {
	s := testServer("+15555550101")
	r, w := io.Pipe()
	out, ow := io.Pipe()
	done := make(chan error)
	go func() { done <- s.ServeStream(context.Background(), r, ow) }()
	br := bufio.NewReader(out)
	fmt.Fprintln(w, `{"jsonrpc":"2.0","method":"subscribeReceive","id":1}`)
	line, _ := br.ReadString('\n')
	if line != `{"jsonrpc":"2.0","result":1,"id":1}`+"\n" {
		t.Fatalf("subscribe: %q", line)
	}
	go s.Publish(false, "+15555550101", `{"envelope":{},"account":"+15555550101"}`)
	got := []string{}
	for i := 0; i < 2; i++ {
		l, _ := br.ReadString('\n')
		got = append(got, strings.TrimSpace(l))
	}
	sort.Strings(got)
	want := []string{
		`{"jsonrpc":"2.0","method":"receive","params":{"envelope":{},"account":"+15555550101"}}`,
		`{"jsonrpc":"2.0","method":"receive","params":{"subscription":1,"result":{"envelope":{},"account":"+15555550101"}}}`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("notifications: %q", got)
	}
	w.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestLineEndings(t *testing.T) {
	got := session(t, testServer(), `{"jsonrpc":"2.0","method":"version","id":"cr"}`+"\r"+`{"jsonrpc":"2.0","method":"version","id":"crlf"}`+"\r\n"+`{"jsonrpc":"2.0","method":"version","id":"eof"}`)
	if len(got) != 3 {
		t.Fatalf("got %q", got)
	}
}

func TestNestingCrash(t *testing.T) {
	var out strings.Builder
	err := testServer().ServeStream(context.Background(), strings.NewReader(strings.Repeat("[", 1001)+"\n"), &out)
	if !errorsIs(err, errcode.JSONNestingDepth) || errcode.ExitCode(err) != 2 {
		t.Fatalf("got %v", err)
	}
	if want := "java.lang.AssertionError: com.fasterxml.jackson.core.exc.StreamConstraintsException: Document nesting depth (1001) exceeds the maximum allowed (1000, from `StreamReadConstraints.getMaxNestingDepth()`)"; errcode.Message(err) != want {
		t.Errorf("message %q", errcode.Message(err))
	}
}

func errorsIs(err error, c errcode.Code) bool {
	e := errcode.As(err)
	return err != nil && e.Code == c
}

func httpDo(t *testing.T, addr, raw string) string {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	io.WriteString(c, raw)
	b, _ := io.ReadAll(c)
	return string(b)
}

func TestHTTPBodies(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	h := &HTTP{S: testServer(), BindHost: "127.0.0.1", Port: port}
	go h.Serve(context.Background(), ln)
	post := func(body string) string {
		raw := fmt.Sprintf("POST /api/v1/rpc HTTP/1.1\r\nHost: 127.0.0.1:%d\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", port, len(body), body)
		resp := httpDo(t, ln.Addr().String(), raw)
		_, b, _ := strings.Cut(resp, "\r\n\r\n")
		return b
	}
	loc := "\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); "
	for _, c := range []struct{ body, want string }{
		// Columns count bytes over HTTP.
		{`["é" x]`, errLine(-32700, "Unexpected character ('x' (code 120)): was expecting comma to separate Array entries"+loc+"line: 1, column: 7]", "null")},
		{"{\r\n\"a\": x}", errLine(-32700, "Unrecognized token 'x': was expecting (JSON String, Number, Array, Object or token 'null', 'true' or 'false')"+loc+"line: 2, column: 6]", "null")},
		{"\x01", errLine(-32700, "Illegal character ((CTRL-CHAR, code 1)): only regular white space (\\r, \\n, \\t) is allowed between tokens"+loc+"line: 1, column: 2]", "null")},
		{"  ", errLine(-32600, "unexpected type: MISSING", "null")},
		{strings.Repeat("[", 1001), errLine(-32603, "An internal server error has occurred.", "null")},
		{`{"jsonrpc":"2.0","method":"subscribeReceive","id":1}`, errLine(-32601, "Method not implemented", "1")},
	} {
		if got := post(c.body); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.body, got, c.want)
		}
	}
	resp := httpDo(t, ln.Addr().String(), "HEAD /api/v1/check HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
	if !strings.HasPrefix(resp, "HTTP/1.1 405 Method Not Allowed\r\nDate: ") || strings.Contains(resp, "Content-length") {
		t.Errorf("HEAD: %q", resp)
	}
}

func TestHostPinning(t *testing.T) {
	h := &HTTP{BindHost: "127.0.0.1", Port: 8080}
	for v, want := range map[string]bool{
		"127.0.0.1:8080": true, "localhost": true, "LOCALHOST:8080": true, "[::1]": true, "[::1]:8080": true,
		"127.0.0.1:8081": false, "evil.example": false, "localhost:x": true,
	} {
		if got := h.hostAllowed(v); got != want {
			t.Errorf("%q: %v", v, got)
		}
	}
	if !(&HTTP{BindHost: "0.0.0.0", Port: 1}).hostAllowed("evil.example") {
		t.Error("wildcard bind must accept any host")
	}
}
