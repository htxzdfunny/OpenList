package heybox_chat

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
)

const hkeyCharset = "AB45STUVWZEFGJ6CH01D237IXYPQRKLMN89"

func createNonce(ts int64) string {
	var extra [16]byte
	_, _ = rand.Read(extra[:])
	sum := md5.Sum([]byte(strconv.FormatInt(ts, 10) + hex.EncodeToString(extra[:])))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func createHkey(path string, ts int64, nonce string) string {
	return ov(path, ts+1, nonce)
}

func normalizeSignPath(path string) string {
	parts := strings.Split(path, "/")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return "/" + strings.Join(kept, "/") + "/"
}

func charsetPrefix(charset string, n int) string {
	if n < 0 {
		n = len(charset) + n
	}
	if n < 0 {
		n = 0
	}
	if n > len(charset) {
		n = len(charset)
	}
	return charset[:n]
}

func av(e, charset string, n int) string {
	prefix := charsetPrefix(charset, n)
	if prefix == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(e))
	for i := 0; i < len(e); i++ {
		b.WriteByte(prefix[int(e[i])%len(prefix)])
	}
	return b.String()
}

func sv(e, charset string) string {
	if charset == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(e))
	for i := 0; i < len(e); i++ {
		b.WriteByte(charset[int(e[i])%len(charset)])
	}
	return b.String()
}

func interleave(parts []string) string {
	maxLen := 0
	for _, part := range parts {
		if len(part) > maxLen {
			maxLen = len(part)
		}
	}
	var b strings.Builder
	for i := 0; i < maxLen; i++ {
		for _, part := range parts {
			if i < len(part) {
				b.WriteByte(part[i])
			}
		}
	}
	return b.String()
}

func vm(e int) int {
	if e&128 != 0 {
		return ((e << 1) ^ 27) & 255
	}
	return e << 1
}

func qm(e int) int      { return vm(e) ^ e }
func dollarM(e int) int { return qm(vm(e)) }
func ym(e int) int      { return dollarM(qm(vm(e))) }
func gm(e int) int      { return ym(e) ^ dollarM(e) ^ qm(e) }

func km(e []int) []int {
	if len(e) < 4 {
		return e
	}
	t0 := gm(e[0]) ^ ym(e[1]) ^ dollarM(e[2]) ^ qm(e[3])
	t1 := qm(e[0]) ^ gm(e[1]) ^ ym(e[2]) ^ dollarM(e[3])
	t2 := dollarM(e[0]) ^ qm(e[1]) ^ gm(e[2]) ^ ym(e[3])
	t3 := ym(e[0]) ^ dollarM(e[1]) ^ qm(e[2]) ^ gm(e[3])
	e[0], e[1], e[2], e[3] = t0, t1, t2, t3
	return e
}

func ov(path string, ts int64, nonce string) string {
	path = normalizeSignPath(path)
	str1 := av(strconv.FormatInt(ts, 10), hkeyCharset, -2)
	str2 := sv(path, hkeyCharset)
	str3 := sv(nonce, hkeyCharset)
	mixed := interleave([]string{str1, str2, str3})
	if len(mixed) > 20 {
		mixed = mixed[:20]
	}
	sum := md5.Sum([]byte(mixed))
	digest := hex.EncodeToString(sum[:])
	last := digest[len(digest)-6:]
	codes := make([]int, len(last))
	for i := 0; i < len(last); i++ {
		codes[i] = int(last[i])
	}
	mixedCodes := km(codes)
	total := 0
	for _, n := range mixedCodes {
		total += n
	}
	suffix := strconv.Itoa(total % 100)
	if len(suffix) < 2 {
		suffix = "0" + suffix
	}
	prefix := av(digest[:5], hkeyCharset, -4)
	return prefix + suffix
}
