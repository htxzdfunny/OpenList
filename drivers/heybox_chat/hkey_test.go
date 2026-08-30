package heybox_chat

import (
	"strconv"
	"testing"
)

func TestCreateHkey(t *testing.T) {
	tests := []struct {
		path, nonce, want string
		time              int64
	}{
		{"/bbs/app/api/qcloud/cos/upload/info/v2", "ABCDEF0123456789ABCDEF0123456789", "V2V1Z67", 1700000000},
		{"bbs/app/api/qcloud/cos/upload/token/v2", "1234567890ABCDEF1234567890ABCDEF", "SDZ2Z28", 1710000000},
		{"/bbs/app/api/qcloud/cos/upload/callback/v2/", "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", "PZUYD57", 1720000000},
		// official G.g(path, t, nonce) = ov(path, t+1). t+3 collides on 1700000000 but not here.
		{"/bbs/app/api/qcloud/cos/upload/info/v2", "ABCDEF0123456789ABCDEF0123456789", "V2V1Z67", 1700000998},
	}
	for _, tt := range tests {
		got := createHkey(tt.path, tt.time, tt.nonce)
		if got != tt.want {
			t.Fatalf("createHkey(%q, %d, %q) = %q, want %q", tt.path, tt.time, tt.nonce, got, tt.want)
		}
	}
}

func TestQueryParamsIncludeSign(t *testing.T) {
	d := &HeyboxChat{Addition: Addition{HeyboxID: "1", Pkey: "x"}}
	params := d.queryParams(infoAPI)
	for _, key := range []string{"hkey", "_time", "nonce", "device_id", "x_os_type", "device_info", "chat_exe_version"} {
		if params[key] == "" {
			t.Fatalf("%s must not be empty", key)
		}
	}
	if params["x_os_type"] != "Windows" {
		t.Fatalf("x_os_type = %q", params["x_os_type"])
	}
	if params["device_info"] != "Chrome" {
		t.Fatalf("device_info = %q", params["device_info"])
	}
	ts, err := strconv.ParseInt(params["_time"], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	want := createHkey(infoAPI, ts, params["nonce"])
	if params["hkey"] != want {
		t.Fatalf("hkey = %q, want %q", params["hkey"], want)
	}
	again := d.queryParams(infoAPI)
	if again["device_id"] != params["device_id"] {
		t.Fatalf("device_id should be stable, %q vs %q", params["device_id"], again["device_id"])
	}
}
