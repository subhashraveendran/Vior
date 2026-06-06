package protocol

import (
	"encoding/json"
	"testing"
)

// Verifies that the JSON shape sent by mobile-cap/src/js/screens/remote.ts
// unmarshals correctly into InputMessage.
func TestRemoteInputShape(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want InputMessage
	}{
		{
			name: "mouse move",
			raw:  `{"type":"input","data":{"event":"mouse","action":"move","dx":4,"dy":3}}`,
			want: InputMessage{Event: "mouse", Action: "move", DX: 4, DY: 3},
		},
		{
			name: "mouse click",
			raw:  `{"type":"input","data":{"event":"mouse","action":"click"}}`,
			want: InputMessage{Event: "mouse", Action: "click"},
		},
		{
			name: "scroll",
			raw:  `{"type":"input","data":{"event":"scroll","dx":0,"dy":3}}`,
			want: InputMessage{Event: "scroll", DX: 0, DY: 3},
		},
		{
			name: "key chord",
			raw:  `{"type":"input","data":{"event":"key","key":"Cmd+c"}}`,
			want: InputMessage{Event: "key", Key: "Cmd+c"},
		},
		{
			name: "touch from stream",
			raw:  `{"type":"input","data":{"event":"touch","action":"down","x":120,"y":340}}`,
			want: InputMessage{Event: "touch", Action: "down", X: 120, Y: 340},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := Decode([]byte(tc.raw))
			if err != nil {
				t.Fatalf("decode envelope: %v", err)
			}
			if env.Type != MsgInput {
				t.Fatalf("wrong type: %s", env.Type)
			}
			got, err := DecodeData[InputMessage](env)
			if err != nil {
				t.Fatalf("decode input: %v", err)
			}
			if *got != tc.want {
				gj, _ := json.Marshal(got)
				wj, _ := json.Marshal(tc.want)
				t.Fatalf("got %s want %s", gj, wj)
			}
		})
	}
}
