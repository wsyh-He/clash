package adapters

import (
	"encoding/json"
	"errors"
	"net"
	"sort"

	C "github.com/Dreamacro/clash/constant"
)

type Selector struct {
	*Base
	selected C.Proxy
	proxies  map[string]C.Proxy
}

type SelectorOption struct {
	Name    string   `proxy:"name"`
	Proxies []string `proxy:"proxies"`
}

func (s *Selector) Generator(metadata *C.Metadata) (net.Conn, error) {
	return s.selected.Generator(metadata)
}

func (s *Selector) MarshalJSON() ([]byte, error) {
	var all []string
	for k := range s.proxies {
		all = append(all, k)
	}
	sort.Strings(all)
	return json.Marshal(map[string]interface{}{
		"type": s.Type().String(),
		"now":  s.Now(),
		"all":  all,
	})
}

func (s *Selector) Now() string {
	return s.selected.Name()
}

func (s *Selector) Set(name string) error {
	proxy, exist := s.proxies[name]
	if !exist {
		return errors.New("Proxy does not exist")
	}
	s.selected = proxy
	return nil
}

func NewSelector(name string, proxies []C.Proxy) (*Selector, error) {
	if len(proxies) == 0 {
		return nil, errors.New("Provide at least one proxy")
	}

	mapping := make(map[string]C.Proxy)
	for _, proxy := range proxies {
		mapping[proxy.Name()] = proxy
	}

	s := &Selector{
		Base: &Base{
			name: name,
			tp:   C.Selector,
		},
		proxies:  mapping,
		selected: proxies[0],
	}
	return s, nil
}
