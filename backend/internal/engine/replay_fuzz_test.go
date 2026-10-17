package engine

import "testing"

func FuzzReplayDeterministic(f *testing.F) {
	for _, seed := range []uint64{1, 2, 42, 1<<63 + 17} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, seed uint64) {
		config := testConfig()
		config.Seed = seed
		commands := []Command{
			command("orders-a", CommandArrangeOrders, SeatA, 0, testOrders(SeatA), nil),
			command("orders-b", CommandArrangeOrders, SeatB, 1, reverse(testOrders(SeatB)), nil),
			command("mulligan-a", CommandMulligan, SeatA, 2, nil, nil),
			command("mulligan-b", CommandMulligan, SeatB, 3, nil, nil),
		}
		_, first, err := Replay(config, commands)
		if err != nil {
			t.Fatal(err)
		}
		_, second, err := Replay(config, commands)
		if err != nil {
			t.Fatal(err)
		}
		if first != second {
			t.Fatalf("seed %d replay hashes differ: %s != %s", seed, first, second)
		}
	})
}
