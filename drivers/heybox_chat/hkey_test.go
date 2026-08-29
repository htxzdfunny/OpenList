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
	if params["hkey"] == "" {
		t.Fatal("hkey must not be empty")
	}
	if params["_time"] == "" {
		t.Fatal("_time must not be empty")
	}
	if params["nonce"] == "" {
		t.Fatal("nonce must not be empty")
	}
	ts, err := strconv.ParseInt(params["_time"], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	want := createHkey(infoAPI, ts, params["nonce"])
	if params["hkey"] != want {
		t.Fatalf("hkey = %q, want %q", params["hkey"], want)
	}
}
