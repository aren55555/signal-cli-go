// SPDX-License-Identifier: GPL-3.0-or-later

package jackson

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/aren55555/signal-cli-go/internal/errcode"
)

// testdata/parse-errors.json pairs malformed single-line inputs with the
// message the reference's JSON-RPC reader returned for each (black-box
// probe of signal-cli 0.14.7 with no accounts).
func TestParseErrors(t *testing.T) {
	data, err := os.ReadFile("testdata/parse-errors.json")
	if err != nil {
		t.Fatal(err)
	}
	var pairs [][2]string
	if err := json.Unmarshal(data, &pairs); err != nil {
		t.Fatal(err)
	}
	if len(pairs) < 100 {
		t.Fatalf("only %d vectors", len(pairs))
	}
	for _, p := range pairs {
		_, err := Parse(p[0], Chars)
		if err == nil {
			t.Errorf("%q: no error, want %q", p[0], p[1])
			continue
		}
		if got := errcode.RPCMessage(err); got != p[1] {
			t.Errorf("%q:\n got %q\nwant %q", p[0], got, p[1])
		}
		if errcode.RPCCode(err) != -32700 {
			t.Errorf("%q: code %d", p[0], errcode.RPCCode(err))
		}
	}
}

func TestNestingDepth(t *testing.T) {
	deep := ""
	for i := 0; i < 1001; i++ {
		deep += "["
	}
	_, err := Parse(deep, Chars)
	if want := "Document nesting depth (1001) exceeds the maximum allowed (1000, from `StreamReadConstraints.getMaxNestingDepth()`) (StreamConstraintsException)"; errcode.RPCMessage(err) != want {
		t.Errorf("got %v", err)
	}
	ok := ""
	for i := 0; i < 1000; i++ {
		ok = "[" + ok + "]"
	}
	if _, err := Parse(ok, Chars); err != nil {
		t.Errorf("depth 1000: %v", err)
	}
}

// Values echoed as request ids (observed): Jackson's node types and
// encoding.
func TestEncode(t *testing.T) {
	for _, c := range []struct{ in, out string }{
		{`1.5`, `1.5`},
		{`1E2`, `100.0`},
		{`-0`, `0`},
		{`-0.0`, `-0.0`},
		{`1e400`, `"Infinity"`},
		{`12345678901234567890`, `12345678901234567890`},
		{`1e7`, `1.0E7`},
		{`1e-3`, `0.001`},
		{`0.0001`, `1.0E-4`},
		{`123456789.123`, `1.23456789123E8`},
		{`12345678901`, `12345678901`},
		{`1.10`, `1.1`},
		{`0.1`, `0.1`},
		{`1e22`, `1.0E22`},
		{`-1.0`, `-1.0`},
		{`"\b\f\n\r\t\u001f\u007f/"`, `"\b\f\n\r\t\u001F` + "\u007f" + `/"`},
		{`"😀 </script>  "`, `"😀 </script>  "`},
		{`"a\u0000b"`, `"a\u0000b"`},
		{`{"a":1,"a":2,"b":[true,null]}`, `{"a":2,"b":[true,null]}`},
		{`[1e5,-1.5e-10]`, `[100000.0,-1.5E-10]`},
	} {
		n, err := Parse(c.in, Chars)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if got := n.Encode(); got != c.out {
			t.Errorf("%s: got %s, want %s", c.in, got, c.out)
		}
	}
}

func TestKinds(t *testing.T) {
	for _, c := range []struct {
		in   string
		kind Kind
	}{
		{`1`, Int}, {`2147483647`, Int}, {`2147483648`, Long}, {`-2147483649`, Long},
		{`9223372036854775807`, Long}, {`9223372036854775808`, BigInt}, {`1.0`, Double},
		{`1e5`, Double}, {`""`, String}, {`null`, Null}, {`true`, Bool}, {`  `, Missing},
	} {
		n, err := Parse(c.in, Chars)
		if err != nil || n.Kind != c.kind {
			t.Errorf("%s: kind %v err %v, want %v", c.in, n, err, c.kind)
		}
	}
}

func TestAsInt(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int32
	}{
		{`5`, 5}, {`"x"`, 0}, {`"12"`, 12}, {`" 7 "`, 7}, {`"1.9"`, 1}, {`1.9`, 1}, {`null`, 0},
		{`[]`, 0}, {`true`, 1}, {`4294967297`, 1}, {`1e20`, math.MaxInt32},
	} {
		n, _ := Parse(c.in, Chars)
		if got := n.AsInt(); got != c.want {
			t.Errorf("%s: AsInt %d, want %d", c.in, got, c.want)
		}
	}
}

func TestText(t *testing.T) {
	n, _ := Parse(`{"s":"x","i":5,"d":2.0,"b":true,"z":null,"a":["p",1],"o":{"k":"v","n":[]}}`, Chars)
	for k, want := range map[string]string{"s": "x", "i": "5", "d": "2.0", "b": "true", "z": "null", "a": "", "o": ""} {
		if got := n.Get(k).AsText(); got != want {
			t.Errorf("AsText(%s) = %q, want %q", k, got, want)
		}
	}
	if got := n.Get("a").JavaString(); got != "[p, 1]" {
		t.Errorf("JavaString array = %q", got)
	}
	if got := n.Get("o").JavaString(); got != "{k=v, n=[]}" {
		t.Errorf("JavaString object = %q", got)
	}
}

func TestLines(t *testing.T) {
	_, err := Parse("{\n  \"a\": x}", Bytes)
	want := "Unrecognized token 'x': was expecting (JSON String, Number, Array, Object or token 'null', 'true' or 'false')\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: 2, column: 8]"
	if got := errcode.RPCMessage(err); got != want {
		t.Errorf("got %q", got)
	}
}
