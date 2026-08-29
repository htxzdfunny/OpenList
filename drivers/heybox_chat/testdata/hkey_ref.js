const crypto = require("crypto");

function md5(c) {
  return crypto.createHash("md5").update(c).digest("hex");
}

function av(e, t, n) {
  for (var r = "", i = t.slice(0, n), o = 0; o < e.length; o++)
    r += i[e.charCodeAt(o) % i.length];
  return r;
}

function sv(e, t) {
  for (var n = "", r = 0; r < e.length; r++) n += t[e.charCodeAt(r) % t.length];
  return n;
}

function Vm(e) {
  return 128 & e ? 255 & ((e << 1) ^ 27) : e << 1;
}
function qm(e) {
  return Vm(e) ^ e;
}
function $m(e) {
  return qm(Vm(e));
}
function Ym(e) {
  return $m(qm(Vm(e)));
}
function Gm(e) {
  return Ym(e) ^ $m(e) ^ qm(e);
}
function Km(e) {
  var t = [0, 0, 0, 0];
  t[0] = Gm(e[0]) ^ Ym(e[1]) ^ $m(e[2]) ^ qm(e[3]);
  t[1] = qm(e[0]) ^ Gm(e[1]) ^ Ym(e[2]) ^ $m(e[3]);
  t[2] = $m(e[0]) ^ qm(e[1]) ^ Gm(e[2]) ^ Ym(e[3]);
  t[3] = Ym(e[0]) ^ $m(e[1]) ^ qm(e[2]) ^ Gm(e[3]);
  e[0] = t[0];
  e[1] = t[1];
  e[2] = t[2];
  e[3] = t[3];
  return e;
}

function ov(e, t, n) {
  e = "/" + e.split("/").filter(Boolean).join("/") + "/";
  var r = "AB45STUVWZEFGJ6CH01D237IXYPQRKLMN89";
  var mixed = [av(String(t), r, -2), sv(e, r), sv(n, r)];
  var interleaved = "";
  var max = Math.max.apply(
    Math,
    mixed.map(function (s) {
      return s.length;
    }),
  );
  for (var i = 0; i < max; i++) {
    mixed.forEach(function (s) {
      if (i < s.length) interleaved += s[i];
    });
  }
  interleaved = interleaved.slice(0, 20);
  var o = md5(interleaved);
  var a = String(
    Km(
      o
        .slice(-6)
        .split("")
        .map(function (c) {
          return c.charCodeAt(0);
        }),
    ).reduce(function (x, y) {
      return x + y;
    }, 0) % 100,
  );
  if (a.length < 2) a = "0" + a;
  var s = av(o.substring(0, 5), r, -4);
  return s + a;
}

function createHkey(path, time, nonce) {
  return ov(path, time + 3, nonce);
}

const cases = [
  {
    path: "/bbs/app/api/qcloud/cos/upload/info/v2",
    time: 1700000000,
    nonce: "ABCDEF0123456789ABCDEF0123456789",
  },
  {
    path: "bbs/app/api/qcloud/cos/upload/token/v2",
    time: 1710000000,
    nonce: "1234567890ABCDEF1234567890ABCDEF",
  },
  {
    path: "/bbs/app/api/qcloud/cos/upload/callback/v2/",
    time: 1720000000,
    nonce: "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF",
  },
];

console.log(
  JSON.stringify(
    cases.map(function (c) {
      return {
        path: c.path,
        time: c.time,
        nonce: c.nonce,
        hkey: createHkey(c.path, c.time, c.nonce),
      };
    }),
    null,
    2,
  ),
);
