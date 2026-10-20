package httpserver

import "testing"

func FuzzStrictWebSocketDecode(f *testing.F) {
	f.Add([]byte(`{"protocolVersion":"1.2","type":"heartbeat","correlationId":"seed"}`))
	f.Add([]byte(`{"protocolVersion":"1.2","type":"heartbeat","correlationId":"seed","ticket":"leak"}`))
	f.Add([]byte(`[[[[[[[[[[[[[[[[[{}]]]]]]]]]]]]]]]]]`))
	f.Fuzz(func(t *testing.T, content []byte) {
		var heartbeat wsHeartbeat
		_ = decodeStrictJSON(content, &heartbeat)
	})
}
