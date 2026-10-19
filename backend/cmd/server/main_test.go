package main

import "testing"

func TestSecurityLimitsFromEnvironment(t *testing.T) {
	t.Setenv("RESPONSE_CREATE_ROOMS_PER_SESSION", "7")
	t.Setenv("RESPONSE_HTTP_BODY_BYTES", "4096")
	limits, err := securityLimitsFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if limits.CreateRoomsPerSession != 7 || limits.HTTPBodyBytes != 4096 {
		t.Fatalf("unexpected limits: %#v", limits)
	}
}

func TestSecurityLimitsRejectInvalidEnvironment(t *testing.T) {
	t.Setenv("RESPONSE_WS_COMMAND_BURST", "0")
	if _, err := securityLimitsFromEnvironment(); err == nil {
		t.Fatal("zero security limit was accepted")
	}
}
