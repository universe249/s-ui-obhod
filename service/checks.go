package service

// obhod: проверки, которые ловят проблемы раньше людей.
//   - Совместимость входящих с приложениями: новый Xray (Happ, v2RayTun) отказывается подключаться при
//     «Allow Insecure», VLESS без шифрования, самоподписанном сертификате; сертификат не на то имя.
//   - Сроки сертификатов TLS-шаблонов: истёк или скоро истечёт.

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
)

// InboundCheck — одно замечание к входящему. Level: bad — у части приложений не заработает; warn — стоит поправить.
type InboundCheck struct {
	Level string `json:"level"`
	Code  string `json:"code"`
	Text  string `json:"text"`
}

// CertInfo — сертификат TLS-шаблона.
type CertInfo struct {
	TlsId      uint     `json:"tlsId"`
	Name       string   `json:"name"`
	Names      []string `json:"names"`
	Issuer     string   `json:"issuer"`
	NotAfter   int64    `json:"notAfter"`
	DaysLeft   int      `json:"daysLeft"`
	SelfSigned bool     `json:"selfSigned"`
	Source     string   `json:"source"` // путь к файлу или «в настройках»
	Error      string   `json:"error,omitempty"`
}

const certWarnDays = 14

func tlsMaps(t *model.Tls) (server, client map[string]interface{}) {
	if t == nil {
		return nil, nil
	}
	_ = json.Unmarshal(t.Server, &server)
	_ = json.Unmarshal(t.Client, &client)
	return server, client
}

func certOf(t *model.Tls) CertInfo {
	ci := CertInfo{TlsId: t.Id, Name: t.Name}
	server, _ := tlsMaps(t)
	if server == nil {
		ci.Error = "не удалось прочитать настройки TLS"
		return ci
	}
	if r, ok := server["reality"].(map[string]interface{}); ok && r["enabled"] == true {
		ci.Source = "reality"
		return ci
	}
	if p, _ := server["certificate_path"].(string); p != "" {
		ci.Source = p
	} else {
		ci.Source = "в настройках"
	}
	pemData := util.CertPEMFromTLS(server)
	if pemData == "" {
		if _, acme := server["acme"]; acme {
			ci.Source = "acme"
			return ci
		}
		ci.Error = "сертификат не найден"
		return ci
	}
	c := util.CertLeaf(pemData)
	if c == nil {
		ci.Error = "сертификат не разобрать"
		return ci
	}
	ci.Names = append([]string{}, c.DNSNames...)
	for _, ip := range c.IPAddresses {
		ci.Names = append(ci.Names, ip.String())
	}
	if len(ci.Names) == 0 && c.Subject.CommonName != "" {
		ci.Names = []string{c.Subject.CommonName}
	}
	ci.Issuer = c.Issuer.CommonName
	ci.NotAfter = c.NotAfter.Unix()
	ci.DaysLeft = int(time.Until(c.NotAfter).Hours() / 24)
	if time.Now().After(c.NotAfter) {
		ci.DaysLeft = -1
	}
	ci.SelfSigned = util.CertIsSelfSigned(pemData)
	return ci
}

// Certs — сертификаты всех TLS-шаблонов (кроме Reality).
func (s *InboundService) Certs() ([]CertInfo, error) {
	var tlss []model.Tls
	if err := database.GetDB().Find(&tlss).Error; err != nil {
		return nil, err
	}
	out := make([]CertInfo, 0, len(tlss))
	for i := range tlss {
		if ci := certOf(&tlss[i]); ci.Source != "reality" {
			out = append(out, ci)
		}
	}
	return out, nil
}

func nameMatches(name string, patterns []string) bool {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	for _, p := range patterns {
		p = strings.ToLower(p)
		if p == name {
			return true
		}
		if strings.HasPrefix(p, "*.") {
			if i := strings.IndexByte(name, '.'); i > 0 && name[i+1:] == p[2:] {
				return true
			}
		}
	}
	return false
}

// checkInbound — замечания к одному входящему. host — адрес, который увидят приложения в ссылке.
func checkInbound(in *model.Inbound, publicHost string) []InboundCheck {
	var out []InboundCheck
	add := func(level, code, text string) { out = append(out, InboundCheck{level, code, text}) }
	var opts map[string]interface{}
	if b, err := in.MarshalJSON(); err == nil {
		_ = json.Unmarshal(b, &opts)
	}
	server, client := tlsMaps(in.Tls)
	tlsOn := in.TlsId > 0 && server != nil && server["enabled"] != false
	reality := false
	if r, ok := server["reality"].(map[string]interface{}); ok && r["enabled"] == true {
		reality = true
	}
	_, hasTransport := opts["transport"].(map[string]interface{})

	switch in.Type {
	case "vless":
		if !tlsOn && !hasTransport {
			add("bad", "vless-plain", "VLESS без TLS и без Reality: Happ и v2RayTun (новый Xray) откажутся подключаться к публичному адресу")
		}
	case "hysteria", "hysteria2", "tuic", "naive", "anytls":
		if !tlsOn {
			add("bad", "no-tls", "Для этого протокола нужен TLS: выберите TLS-шаблон")
		}
	}
	if !tlsOn || reality {
		return out
	}

	// адреса, которые попадут в ссылки, и SNI для каждого
	type target struct {
		host, sni string
		insecure  bool
	}
	baseSNI, _ := server["server_name"].(string)
	baseInsecure, _ := client["insecure"].(bool)
	var targets []target
	var addrs []map[string]interface{}
	_ = json.Unmarshal(in.Addrs, &addrs)
	for _, a := range addrs {
		t := target{sni: baseSNI, insecure: baseInsecure}
		t.host, _ = a["server"].(string)
		if at, ok := a["tls"].(map[string]interface{}); ok {
			if v, _ := at["server_name"].(string); v != "" {
				t.sni = v
			}
			if v, ok := at["insecure"].(bool); ok {
				t.insecure = v
			}
		}
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		targets = []target{{host: publicHost, sni: baseSNI, insecure: baseInsecure}}
	}

	ci := certOf(in.Tls)
	for _, t := range targets {
		if t.insecure {
			add("bad", "insecure", "Включено «Allow Insecure»: новый Xray (Happ, v2RayTun) такие ссылки не принимает. Нужен настоящий сертификат и выключенный insecure")
			break
		}
	}
	if ci.Error != "" {
		add("bad", "cert", "Сертификат: "+ci.Error)
		return out
	}
	if ci.NotAfter == 0 { // acme: сроки ведёт sing-box
		return out
	}
	if ci.DaysLeft < 0 {
		add("bad", "cert-expired", "Сертификат истёк — подключиться не получится")
	} else if ci.DaysLeft < certWarnDays {
		add("warn", "cert-expiring", fmt.Sprintf("Сертификат истекает через %d дн.", ci.DaysLeft))
	}
	if ci.SelfSigned {
		allInsecure := true
		for _, t := range targets {
			allInsecure = allInsecure && t.insecure
		}
		if !allInsecure {
			add("bad", "self-signed", "Самоподписанный сертификат: приложения не смогут его проверить. Нужен сертификат Let's Encrypt на домен")
		}
		return out
	}
	for _, t := range targets {
		name := t.sni
		if name == "" {
			name = t.host
		}
		if name == "" {
			add("warn", "no-host", "Не задан публичный адрес сервера: ссылки получат адрес, с которого открыта панель. Укажите его в Настройках → Подписка")
			break
		}
		if net.ParseIP(strings.Trim(name, "[]")) != nil && !nameMatches(name, ci.Names) {
			add("bad", "ip-sni", "В ссылке IP-адрес "+name+", а сертификат выписан на "+strings.Join(ci.Names, ", ")+": задайте домен (публичный адрес или Multi Domain с SNI)")
			break
		}
		if !nameMatches(name, ci.Names) {
			add("bad", "sni-mismatch", "Приложения проверят имя "+name+", а сертификат выписан на "+strings.Join(ci.Names, ", "))
			break
		}
	}
	return out
}

// Checks — замечания ко всем входящим: {id входящего: [...]}.
func (s *InboundService) Checks() (map[uint][]InboundCheck, error) {
	var inbounds []model.Inbound
	if err := database.GetDB().Model(model.Inbound{}).Preload("Tls").Find(&inbounds).Error; err != nil {
		return nil, err
	}
	host := (&SettingService{}).PublicHost(nil)
	out := map[uint][]InboundCheck{}
	for i := range inbounds {
		if c := checkInbound(&inbounds[i], host); len(c) > 0 {
			out[inbounds[i].Id] = c
		}
	}
	return out, nil
}
