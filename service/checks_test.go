package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

// testCert: ca=nil — самоподписанный; иначе подписан ca.
func testCert(t *testing.T, name string, days int, ca *x509.Certificate, caKey *ecdsa.PrivateKey) (string, *x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Duration(days) * 24 * time.Hour)}
	if name != "" && ca != nil {
		tpl.DNSNames = []string{name}
	}
	if ca == nil {
		tpl.IsCA, tpl.BasicConstraintsValid, tpl.KeyUsage = true, true, x509.KeyUsageCertSign|x509.KeyUsageDigitalSignature
		if name != "Test CA" {
			tpl.DNSNames = []string{name}
		}
		ca, caKey = tpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), c, key
}

func tlsWith(pemCert string, insecure bool) *model.Tls {
	srv, _ := json.Marshal(map[string]interface{}{"enabled": true, "certificate": pemCert, "key": "x"})
	cli, _ := json.Marshal(map[string]interface{}{"insecure": insecure})
	return &model.Tls{Id: 1, Name: "t", Server: srv, Client: cli}
}

func inb(typ string, tls *model.Tls, addrs string) *model.Inbound {
	in := &model.Inbound{Id: 1, Type: typ, Tag: typ + "-1", Addrs: json.RawMessage(addrs), Options: json.RawMessage(`{"listen_port":443}`)}
	if tls != nil {
		in.TlsId, in.Tls = tls.Id, tls
	}
	return in
}

func codes(cs []InboundCheck) map[string]bool {
	m := map[string]bool{}
	for _, c := range cs {
		m[c.Code] = true
	}
	return m
}

func TestInboundChecks(t *testing.T) {
	_, ca, caKey := testCert(t, "Test CA", 3650, nil, nil)
	good, _, _ := testCert(t, "hoop.example.ru", 60, ca, caKey)
	soon, _, _ := testCert(t, "hoop.example.ru", 5, ca, caKey)
	self, _, _ := testCert(t, "hoop.example.ru", 365, nil, nil)

	cases := []struct {
		name string
		in   *model.Inbound
		host string
		want []string
		none bool
	}{
		{"vless без TLS", inb("vless", nil, "[]"), "hoop.example.ru", []string{"vless-plain"}, false},
		{"всё хорошо", inb("hysteria2", tlsWith(good, false), "[]"), "hoop.example.ru", nil, true},
		{"allow insecure", inb("hysteria2", tlsWith(self, true), "[]"), "hoop.example.ru", []string{"insecure"}, false},
		{"самоподписанный без insecure", inb("hysteria2", tlsWith(self, false), "[]"), "hoop.example.ru", []string{"self-signed"}, false},
		{"IP в ссылке", inb("hysteria2", tlsWith(good, false), "[]"), "185.72.147.22", []string{"ip-sni"}, false},
		{"чужое имя", inb("trojan", tlsWith(good, false), "[]"), "other.example.ru", []string{"sni-mismatch"}, false},
		{"Multi Domain с SNI спасает IP", inb("hysteria2", tlsWith(good, false), `[{"server":"185.72.147.22","server_port":443,"tls":{"server_name":"hoop.example.ru"}}]`), "", nil, true},
		{"скоро истекает", inb("hysteria2", tlsWith(soon, false), "[]"), "hoop.example.ru", []string{"cert-expiring"}, false},
		{"нет публичного адреса", inb("hysteria2", tlsWith(good, false), "[]"), "", []string{"no-host"}, false},
		{"hysteria2 без TLS", inb("hysteria2", nil, "[]"), "hoop.example.ru", []string{"no-tls"}, false},
	}
	for _, c := range cases {
		got := codes(checkInbound(c.in, c.host))
		if c.none && len(got) > 0 {
			t.Errorf("%s: лишние замечания %v", c.name, got)
		}
		for _, w := range c.want {
			if !got[w] {
				t.Errorf("%s: нет %q, есть %v", c.name, w, got)
			}
		}
	}
	if ci := certOf(tlsWith(soon, false)); ci.DaysLeft > 5 || ci.SelfSigned || ci.Names[0] != "hoop.example.ru" {
		t.Fatalf("cert info %+v", ci)
	}
	if !certOf(tlsWith(self, false)).SelfSigned {
		t.Fatal("self-signed not detected")
	}
	if !nameMatches("a.example.ru", []string{"*.example.ru"}) || nameMatches("a.b.example.ru", []string{"*.example.ru"}) {
		t.Fatal("wildcard")
	}
}
