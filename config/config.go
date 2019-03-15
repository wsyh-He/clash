package config

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	adapters "github.com/Dreamacro/clash/adapters/outbound"
	"github.com/Dreamacro/clash/common/structure"
	C "github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/dns"
	"github.com/Dreamacro/clash/log"
	R "github.com/Dreamacro/clash/rules"
	T "github.com/Dreamacro/clash/tunnel"

	yaml "gopkg.in/yaml.v2"
)

// General config
type General struct {
	Port               int          `json:"port"`
	SocksPort          int          `json:"socks-port"`
	RedirPort          int          `json:"redir-port"`
	AllowLan           bool         `json:"allow-lan"`
	Mode               T.Mode       `json:"mode"`
	LogLevel           log.LogLevel `json:"log-level"`
	ExternalController string       `json:"-"`
	ExternalUI         string       `json:"-"`
	Secret             string       `json:"-"`
}

// DNS config
type DNS struct {
	Enable       bool             `yaml:"enable"`
	IPv6         bool             `yaml:"ipv6"`
	NameServer   []dns.NameServer `yaml:"nameserver"`
	Fallback     []dns.NameServer `yaml:"fallback"`
	Listen       string           `yaml:"listen"`
	EnhancedMode dns.EnhancedMode `yaml:"enhanced-mode"`
}

// Config is clash config manager
type Config struct {
	General *General
	DNS     *DNS
	Rules   []C.Rule
	Proxies map[string]C.Proxy
}

type rawDNS struct {
	Enable       bool             `yaml:"enable"`
	IPv6         bool             `yaml:"ipv6"`
	NameServer   []string         `yaml:"nameserver"`
	Fallback     []string         `yaml:"fallback"`
	Listen       string           `yaml:"listen"`
	EnhancedMode dns.EnhancedMode `yaml:"enhanced-mode"`
}

type rawConfig struct {
	Port               int          `yaml:"port"`
	SocksPort          int          `yaml:"socks-port"`
	RedirPort          int          `yaml:"redir-port"`
	AllowLan           bool         `yaml:"allow-lan"`
	Mode               T.Mode       `yaml:"mode"`
	LogLevel           log.LogLevel `yaml:"log-level"`
	ExternalController string       `yaml:"external-controller"`
	ExternalUI         string       `yaml:"external-ui"`
	Secret             string       `yaml:"secret"`

	DNS        rawDNS                   `yaml:"dns"`
	Proxy      []map[string]interface{} `yaml:"Proxy"`
	ProxyGroup []map[string]interface{} `yaml:"Proxy Group"`
	Rule       []string                 `yaml:"Rule"`
}

func readConfig(path string) (*rawConfig, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("Configuration file %s is empty", C.Path.Config())
	}

	// config with some default value
	rawConfig := &rawConfig{
		AllowLan:   false,
		Mode:       T.Rule,
		LogLevel:   log.INFO,
		Rule:       []string{},
		Proxy:      []map[string]interface{}{},
		ProxyGroup: []map[string]interface{}{},
		DNS: rawDNS{
			Enable: false,
		},
	}
	err = yaml.Unmarshal([]byte(data), &rawConfig)
	return rawConfig, err
}

// Parse config
func Parse(path string) (*Config, error) {
	config := &Config{}

	rawCfg, err := readConfig(path)
	if err != nil {
		return nil, err
	}

	general, err := parseGeneral(rawCfg)
	if err != nil {
		return nil, err
	}
	config.General = general

	proxies, err := parseProxies(rawCfg)
	if err != nil {
		return nil, err
	}
	config.Proxies = proxies

	rules, err := parseRules(rawCfg)
	if err != nil {
		return nil, err
	}
	config.Rules = rules

	dnsCfg, err := parseDNS(rawCfg.DNS)
	if err != nil {
		return nil, err
	}
	config.DNS = dnsCfg

	return config, nil
}

func parseGeneral(cfg *rawConfig) (*General, error) {
	port := cfg.Port
	socksPort := cfg.SocksPort
	redirPort := cfg.RedirPort
	allowLan := cfg.AllowLan
	externalController := cfg.ExternalController
	externalUI := cfg.ExternalUI
	secret := cfg.Secret
	mode := cfg.Mode
	logLevel := cfg.LogLevel

	if externalUI != "" {
		if !filepath.IsAbs(externalUI) {
			externalUI = filepath.Join(C.Path.HomeDir(), externalUI)
		}

		if _, err := os.Stat(externalUI); os.IsNotExist(err) {
			return nil, fmt.Errorf("external-ui: %s not exist", externalUI)
		}
	}

	general := &General{
		Port:               port,
		SocksPort:          socksPort,
		RedirPort:          redirPort,
		AllowLan:           allowLan,
		Mode:               mode,
		LogLevel:           logLevel,
		ExternalController: externalController,
		ExternalUI:         externalUI,
		Secret:             secret,
	}
	return general, nil
}

func parseProxies(cfg *rawConfig) (map[string]C.Proxy, error) {
	proxies := make(map[string]C.Proxy)
	proxiesConfig := cfg.Proxy
	groupsConfig := cfg.ProxyGroup

	decoder := structure.NewDecoder(structure.Option{TagName: "proxy", WeaklyTypedInput: true})

	proxies["DIRECT"] = adapters.NewDirect()
	proxies["REJECT"] = adapters.NewReject()

	// parse proxy
	for idx, mapping := range proxiesConfig {
		proxyType, existType := mapping["type"].(string)
		if !existType {
			return nil, fmt.Errorf("Proxy %d missing type", idx)
		}

		var proxy C.Proxy
		err := fmt.Errorf("can't parse")
		switch proxyType {
		case "ss":
			ssOption := &adapters.ShadowSocksOption{}
			err = decoder.Decode(mapping, ssOption)
			if err != nil {
				break
			}
			proxy, err = adapters.NewShadowSocks(*ssOption)
		case "socks5":
			socksOption := &adapters.Socks5Option{}
			err = decoder.Decode(mapping, socksOption)
			if err != nil {
				break
			}
			proxy = adapters.NewSocks5(*socksOption)
		case "http":
			httpOption := &adapters.HttpOption{}
			err = decoder.Decode(mapping, httpOption)
			if err != nil {
				break
			}
			proxy = adapters.NewHttp(*httpOption)
		case "vmess":
			vmessOption := &adapters.VmessOption{}
			err = decoder.Decode(mapping, vmessOption)
			if err != nil {
				break
			}
			proxy, err = adapters.NewVmess(*vmessOption)
		default:
			return nil, fmt.Errorf("Unsupport proxy type: %s", proxyType)
		}

		if err != nil {
			return nil, fmt.Errorf("Proxy [%d]: %s", idx, err.Error())
		}

		if _, exist := proxies[proxy.Name()]; exist {
			return nil, fmt.Errorf("Proxy %s is the duplicate name", proxy.Name())
		}
		proxies[proxy.Name()] = proxy
	}

	// parse proxy group
	for idx, mapping := range groupsConfig {
		groupType, existType := mapping["type"].(string)
		groupName, existName := mapping["name"].(string)
		if !existType && existName {
			return nil, fmt.Errorf("ProxyGroup %d: missing type or name", idx)
		}

		if _, exist := proxies[groupName]; exist {
			return nil, fmt.Errorf("ProxyGroup %s: the duplicate name", groupName)
		}
		var group C.Proxy
		ps := []C.Proxy{}

		err := fmt.Errorf("can't parse")
		switch groupType {
		case "url-test":
			urlTestOption := &adapters.URLTestOption{}
			err = decoder.Decode(mapping, urlTestOption)
			if err != nil {
				break
			}

			ps, err = getProxies(proxies, urlTestOption.Proxies)
			if err != nil {
				return nil, fmt.Errorf("ProxyGroup %s: %s", groupName, err.Error())
			}
			group, err = adapters.NewURLTest(*urlTestOption, ps)
		case "select":
			selectorOption := &adapters.SelectorOption{}
			err = decoder.Decode(mapping, selectorOption)
			if err != nil {
				break
			}

			ps, err = getProxies(proxies, selectorOption.Proxies)
			if err != nil {
				return nil, fmt.Errorf("ProxyGroup %s: %s", groupName, err.Error())
			}
			group, err = adapters.NewSelector(selectorOption.Name, ps)
		case "fallback":
			fallbackOption := &adapters.FallbackOption{}
			err = decoder.Decode(mapping, fallbackOption)
			if err != nil {
				break
			}

			ps, err = getProxies(proxies, fallbackOption.Proxies)
			if err != nil {
				return nil, fmt.Errorf("ProxyGroup %s: %s", groupName, err.Error())
			}
			group, err = adapters.NewFallback(*fallbackOption, ps)
		case "load-balance":
			loadBalanceOption := &adapters.LoadBalanceOption{}
			err = decoder.Decode(mapping, loadBalanceOption)
			if err != nil {
				break
			}

			ps, err = getProxies(proxies, loadBalanceOption.Proxies)
			if err != nil {
				return nil, fmt.Errorf("ProxyGroup %s: %s", groupName, err.Error())
			}
			group, err = adapters.NewLoadBalance(loadBalanceOption.Name, ps)
		}
		if err != nil {
			return nil, fmt.Errorf("Proxy %s: %s", groupName, err.Error())
		}
		proxies[groupName] = group
	}

	ps := []C.Proxy{}
	for _, v := range proxies {
		ps = append(ps, v)
	}

	proxies["GLOBAL"], _ = adapters.NewSelector("GLOBAL", ps)
	return proxies, nil
}

func parseRules(cfg *rawConfig) ([]C.Rule, error) {
	rules := []C.Rule{}

	rulesConfig := cfg.Rule
	// parse rules
	for idx, line := range rulesConfig {
		rule := trimArr(strings.Split(line, ","))
		var (
			payload string
			target  string
		)

		switch len(rule) {
		case 2:
			target = rule[1]
		case 3:
			payload = rule[1]
			target = rule[2]
		default:
			return nil, fmt.Errorf("Rules[%d] [- %s] error: format invalid", idx, line)
		}

		rule = trimArr(rule)
		switch rule[0] {
		case "DOMAIN":
			rules = append(rules, R.NewDomain(payload, target))
		case "DOMAIN-SUFFIX":
			rules = append(rules, R.NewDomainSuffix(payload, target))
		case "DOMAIN-KEYWORD":
			rules = append(rules, R.NewDomainKeyword(payload, target))
		case "GEOIP":
			rules = append(rules, R.NewGEOIP(payload, target))
		case "IP-CIDR", "IP-CIDR6":
			rules = append(rules, R.NewIPCIDR(payload, target, false))
		case "SOURCE-IP-CIDR":
			rules = append(rules, R.NewIPCIDR(payload, target, true))
		case "MATCH":
			fallthrough
		case "FINAL":
			rules = append(rules, R.NewFinal(target))
		}
	}

	return rules, nil
}

func hostWithDefaultPort(host string, defPort string) (string, error) {
	if !strings.Contains(host, ":") {
		host += ":"
	}

	hostname, port, err := net.SplitHostPort(host)
	if err != nil {
		return "", err
	}

	if port == "" {
		port = defPort
	}

	return net.JoinHostPort(hostname, port), nil
}

func parseNameServer(servers []string) ([]dns.NameServer, error) {
	nameservers := []dns.NameServer{}

	for idx, server := range servers {
		// parse without scheme .e.g 8.8.8.8:53
		if host, err := hostWithDefaultPort(server, "53"); err == nil {
			nameservers = append(
				nameservers,
				dns.NameServer{Addr: host},
			)
			continue
		}

		u, err := url.Parse(server)
		if err != nil {
			return nil, fmt.Errorf("DNS NameServer[%d] format error: %s", idx, err.Error())
		}

		if u.Scheme != "tls" {
			return nil, fmt.Errorf("DNS NameServer[%d] unsupport scheme: %s", idx, u.Scheme)
		}

		host, err := hostWithDefaultPort(u.Host, "853")
		nameservers = append(
			nameservers,
			dns.NameServer{
				Net:  "tcp-tls",
				Addr: host,
			},
		)
	}

	return nameservers, nil
}

func parseDNS(cfg rawDNS) (*DNS, error) {
	if cfg.Enable && len(cfg.NameServer) == 0 {
		return nil, fmt.Errorf("If DNS configuration is turned on, NameServer cannot be empty")
	}

	dnsCfg := &DNS{
		Enable:       cfg.Enable,
		Listen:       cfg.Listen,
		EnhancedMode: cfg.EnhancedMode,
	}

	if nameserver, err := parseNameServer(cfg.NameServer); err == nil {
		dnsCfg.NameServer = nameserver
	}

	if fallback, err := parseNameServer(cfg.Fallback); err == nil {
		dnsCfg.Fallback = fallback
	}

	return dnsCfg, nil
}
